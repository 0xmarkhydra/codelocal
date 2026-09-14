package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/deviceauth"
	"github.com/0xmarkhydra/codelocal/internal/version"
	"github.com/0xmarkhydra/codelocal/internal/workspace"
	"github.com/coder/websocket"
)

const (
	runtimeControlHeartbeat = 15 * time.Second
	runtimeRegistryCheck    = 3 * time.Second
	runtimeControlWriteWait = 10 * time.Second
)

type runtimeControlEnvelope struct {
	Type             string                           `json:"type"`
	ClientVersion    string                           `json:"clientVersion,omitempty"`
	CredentialID     string                           `json:"credentialId,omitempty"`
	Secret           string                           `json:"credentialSecret,omitempty"`
	DeviceID         string                           `json:"deviceId,omitempty"`
	WorkspaceIDs     []string                         `json:"workspaceIds,omitempty"`
	WorkspaceID      string                           `json:"workspaceId,omitempty"`
	RequestID        string                           `json:"requestId,omitempty"`
	Activation       *cloud.WorkspaceActivation       `json:"activation,omitempty"`
	ActivationResult *cloud.WorkspaceActivationResult `json:"activationResult,omitempty"`
	Revocation       *cloud.WorkspaceRevocation       `json:"revocation,omitempty"`
	Now              int64                            `json:"now,omitempty"`
}

func runtimeControlURL(base string) string {
	u, err := url.Parse(normalizeBase(base))
	if err != nil {
		return normalizeBase(base) + "/api/client/runtime/ws"
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = "/api/client/runtime/ws"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func runtimeWorkspaceIDs(items []workspace.Workspace) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if item.WorkspaceID != "" {
			ids = append(ids, item.WorkspaceID)
		}
	}
	return ids
}

func writeRuntimeControl(parent context.Context, conn *websocket.Conn, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, runtimeControlWriteWait)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, raw)
}

func runtimeControlError(err error) error {
	if err == nil {
		return nil
	}
	if websocket.CloseStatus(err) == websocket.StatusCode(4403) {
		return ErrDeviceAuthorizationRevoked
	}
	return err
}

func workspaceActivationReason(err error) string {
	switch WorkspaceActivationPhase(err) {
	case "registry_lookup":
		return "workspace_registry_unavailable"
	case "registry_authorization":
		return "workspace_registry_entry_missing"
	case "engine_init":
		return "workspace_engine_initialization_failed"
	case "worker_register":
		return "workspace_worker_registration_failed"
	default:
		return "workspace_activation_failed"
	}
}

func (r *Runtime) dialRuntimeControl(ctx context.Context, items []workspace.Workspace) (*websocket.Conn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	target := runtimeControlURL(r.Options.BaseURL)
	headers := http.Header{}
	if r.Options.Credential.DevicePrivateKey != "" {
		signed, err := deviceauth.SignatureHeaders(http.MethodGet, target, nil, r.Options.Credential.DevicePrivateKey, time.Now())
		if err != nil {
			return nil, err
		}
		headers = signed
	}
	conn, _, err := websocket.Dial(dialCtx, target, &websocket.DialOptions{
		HTTPHeader:      headers,
		CompressionMode: websocket.CompressionContextTakeover,
	})
	if err != nil {
		return nil, runtimeControlError(err)
	}
	conn.SetReadLimit(1 << 20)
	register := runtimeControlEnvelope{
		Type:          "runtime_register",
		ClientVersion: version.Version,
		CredentialID:  r.Options.Credential.CredentialID,
		Secret:        r.Options.Credential.CredentialSecret,
		DeviceID:      r.Options.Credential.DeviceID,
		WorkspaceIDs:  runtimeWorkspaceIDs(items),
	}
	if err := writeRuntimeControl(ctx, conn, register); err != nil {
		_ = conn.Close(websocket.StatusInternalError, "runtime register failed")
		return nil, runtimeControlError(err)
	}
	ackCtx, ackCancel := context.WithTimeout(ctx, 10*time.Second)
	_, raw, err := conn.Read(ackCtx)
	ackCancel()
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, "runtime register timeout")
		return nil, runtimeControlError(err)
	}
	var ack runtimeControlEnvelope
	if json.Unmarshal(raw, &ack) != nil || ack.Type != "runtime_registered" {
		_ = conn.Close(websocket.StatusPolicyViolation, "runtime register rejected")
		return nil, errors.New("CodeLocal Cloud rejected realtime runtime registration")
	}
	return conn, nil
}

func (r *Runtime) handleRuntimeRevocation(ctx context.Context, conn *websocket.Conn, rev *cloud.WorkspaceRevocation, lastItems *[]workspace.Workspace, lastSignature *string) error {
	removed, revokeErr := r.Registry.Revoke(rev.WorkspaceID)
	if revokeErr != nil {
		slog.Warn("workspace revoke failed", "error", revokeErr)
	}
	r.mu.Lock()
	worker := r.workers[rev.WorkspaceID]
	r.mu.Unlock()
	if worker != nil {
		worker.Stop("workspace revoked")
	}
	if removed {
		items, syncErr := r.SyncRegistry(ctx, true)
		if errors.Is(syncErr, ErrDeviceAuthorizationRevoked) {
			return syncErr
		}
		if syncErr != nil {
			slog.Debug("workspace sync after realtime revoke failed", "error", syncErr)
		} else {
			*lastItems = items
			*lastSignature = registrySignature(items)
			if err := writeRuntimeControl(ctx, conn, runtimeControlEnvelope{Type: "runtime_sync", ClientVersion: version.Version, WorkspaceIDs: runtimeWorkspaceIDs(items)}); err != nil {
				return runtimeControlError(err)
			}
		}
	}
	if err := writeRuntimeControl(ctx, conn, runtimeControlEnvelope{Type: "runtime_revocation_ack", WorkspaceID: rev.WorkspaceID, RequestID: rev.RequestID}); err != nil {
		return runtimeControlError(err)
	}
	return nil
}

func (r *Runtime) serveRuntimeControl(parent context.Context, conn *websocket.Conn, initialItems []workspace.Workspace) error {
	ctx, cancel := context.WithCancel(parent)
	r.mu.Lock()
	r.pollCancel = cancel
	r.mu.Unlock()
	defer func() {
		cancel()
		r.mu.Lock()
		r.pollCancel = nil
		r.mu.Unlock()
		_ = conn.Close(websocket.StatusNormalClosure, "runtime control reconnect")
	}()

	events := make(chan runtimeControlEnvelope, 8)
	readErr := make(chan error, 1)
	go func() {
		for {
			_, raw, err := conn.Read(ctx)
			if err != nil {
				readErr <- runtimeControlError(err)
				return
			}
			var envelope runtimeControlEnvelope
			if json.Unmarshal(raw, &envelope) != nil {
				continue
			}
			select {
			case events <- envelope:
			case <-ctx.Done():
				return
			}
		}
	}()

	lastItems := initialItems
	lastSignature := registrySignature(initialItems)
	registryTicker := time.NewTicker(runtimeRegistryCheck)
	heartbeatTicker := time.NewTicker(runtimeControlHeartbeat)
	defer registryTicker.Stop()
	defer heartbeatTicker.Stop()

	sendSync := func(items []workspace.Workspace) error {
		return runtimeControlError(writeRuntimeControl(ctx, conn, runtimeControlEnvelope{
			Type:          "runtime_sync",
			ClientVersion: version.Version,
			WorkspaceIDs:  runtimeWorkspaceIDs(items),
		}))
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-readErr:
			return err
		case <-registryTicker.C:
			items, err := r.SyncRegistry(ctx, false)
			if errors.Is(err, ErrDeviceAuthorizationRevoked) {
				return err
			}
			if err != nil {
				slog.Debug("workspace realtime sync check failed", "error", err)
				continue
			}
			signature := registrySignature(items)
			lastItems = items
			if signature != lastSignature {
				if err := sendSync(items); err != nil {
					return err
				}
				lastSignature = signature
			}
		case <-heartbeatTicker.C:
			if err := sendSync(lastItems); err != nil {
				return err
			}
		case envelope := <-events:
			switch envelope.Type {
			case "runtime_event":
				if envelope.Revocation != nil {
					if err := r.handleRuntimeRevocation(ctx, conn, envelope.Revocation, &lastItems, &lastSignature); err != nil {
						return err
					}
					continue
				}
				if envelope.Activation != nil {
					activation := envelope.Activation
					var activationErr error
					if activation.Action == "install-system-app" {
						if activation.WorkspaceID != cloud.OpenMontageWorkspaceID {
							activationErr = fmt.Errorf("unsupported system app workspace: %s", activation.WorkspaceID)
						} else {
							installCtx, installCancel := context.WithTimeout(ctx, 2*time.Minute)
							_, activationErr = r.InstallSystemApp(installCtx, cloud.OpenMontageSystemProjectID)
							installCancel()
							if activationErr == nil {
								items, syncErr := r.SyncRegistry(ctx, true)
								if syncErr != nil {
									activationErr = syncErr
								} else {
									lastItems = items
									lastSignature = registrySignature(items)
									if err := sendSync(items); err != nil {
										return err
									}
								}
							}
						}
					} else {
						_, activationErr = r.Activate(ctx, activation.WorkspaceID)
					}
					result := &cloud.WorkspaceActivationResult{
						WorkspaceID:    activation.WorkspaceID,
						RequestID:      activation.RequestID,
						OK:             activationErr == nil,
						Phase:          "ready",
						AcknowledgedAt: time.Now().UnixMilli(),
					}
					if activationErr != nil {
						result.Phase = WorkspaceActivationPhase(activationErr)
						result.Reason = workspaceActivationReason(activationErr)
						slog.Warn("workspace activation failed", "workspaceId", activation.WorkspaceID, "requestId", activation.RequestID, "action", activation.Action, "phase", result.Phase, "reason", result.Reason, "error", activationErr)
					}
					if err := writeRuntimeControl(ctx, conn, runtimeControlEnvelope{Type: "runtime_activation_result", ActivationResult: result, Now: time.Now().UnixMilli()}); err != nil {
						return runtimeControlError(err)
					}
				}
			case "runtime_ping":
				if err := writeRuntimeControl(ctx, conn, runtimeControlEnvelope{Type: "runtime_pong", Now: time.Now().UnixMilli()}); err != nil {
					return runtimeControlError(err)
				}
			}
		}
	}
}


func isGatewayNotReady(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connect: connection refused")
}

func (r *Runtime) runRealtime(ctx context.Context) error {
	items, err := r.SyncRegistry(ctx, true)
	if err != nil {
		return err
	}
	slog.Info("CodeLocal runtime connected", "device", r.Options.Credential.DeviceName, "workspaces", len(items), "transport", "websocket")
	if r.Options.OnReady != nil {
		r.Options.OnReady()
	}

	reconnectDelay := time.Second
	var outageSince time.Time
	warned := false
	for {
		r.mu.Lock()
		stopped := r.stopped
		r.mu.Unlock()
		if stopped || ctx.Err() != nil {
			return nil
		}

		items, syncErr := r.SyncRegistry(ctx, false)
		if errors.Is(syncErr, ErrDeviceAuthorizationRevoked) {
			return syncErr
		}
		if syncErr != nil {
			slog.Debug("workspace sync before realtime connect failed", "error", syncErr)
			items, _ = r.Registry.List()
		}
		conn, dialErr := r.dialRuntimeControl(ctx, items)
		if dialErr == nil {
			reconnectDelay = time.Second
			outageSince = time.Time{}
			warned = false
			dialErr = r.serveRuntimeControl(ctx, conn, items)
		}
		if errors.Is(dialErr, ErrDeviceAuthorizationRevoked) {
			return dialErr
		}
		if ctx.Err() != nil {
			return nil
		}
		r.mu.Lock()
		stopped = r.stopped
		r.mu.Unlock()
		if stopped {
			return nil
		}
		if outageSince.IsZero() {
			outageSince = time.Now()
		}
		if !warned && time.Since(outageSince) >= 30*time.Second {
			slog.Warn("runtime realtime connection unavailable; reconnecting", "error", fmt.Sprint(dialErr))
			warned = true
		} else {
			slog.Debug("runtime realtime connection interrupted; reconnecting", "error", fmt.Sprint(dialErr), "retryIn", reconnectDelay)
		}
		timer := time.NewTimer(reconnectDelay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil
		case <-timer.C:
		}
		if !isGatewayNotReady(dialErr) && reconnectDelay < 30*time.Second {
			reconnectDelay *= 2
			if reconnectDelay > 30*time.Second {
				reconnectDelay = 30 * time.Second
			}
		}
	}
}
