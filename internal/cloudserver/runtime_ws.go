package cloudserver

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/coder/websocket"
)

var runtimeWorkspaceIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,80}$`)

type runtimeRealtimeEnvelope struct {
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

func sanitizeRuntimeWorkspaceIDs(values []string) []string {
	if len(values) > 500 {
		values = values[:500]
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if !runtimeWorkspaceIDPattern.MatchString(value) {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func writeRuntimeRealtime(parent context.Context, conn *websocket.Conn, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, raw)
}

func (s *Server) RegisterRuntimeRealtime() {
	s.Mux.HandleFunc("GET /api/client/runtime/ws", s.runtimeRealtime)
}

func (s *Server) runtimeRealtime(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionContextTakeover})
	if err != nil {
		return
	}
	conn.SetReadLimit(1 << 20)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	defer conn.Close(websocket.StatusNormalClosure, "runtime realtime closed")

	registerCtx, registerCancel := context.WithTimeout(ctx, 10*time.Second)
	_, raw, err := conn.Read(registerCtx)
	registerCancel()
	if err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "runtime registration timeout")
		return
	}
	var register runtimeRealtimeEnvelope
	if json.Unmarshal(raw, &register) != nil || register.Type != "runtime_register" || register.CredentialID == "" || register.Secret == "" {
		_ = conn.Close(websocket.StatusPolicyViolation, "runtime_register first")
		return
	}
	device, err := s.Store.AuthenticateDevice(ctx, register.CredentialID, cloud.HashSecret(register.Secret))
	if err != nil || device == nil || device.UserID == "" {
		_ = conn.Close(websocket.StatusCode(4403), "AUTH_FAILED")
		return
	}
	if err := s.verifySignedDeviceRequest(r, device); err != nil {
		_ = conn.Close(websocket.StatusCode(4403), "DEVICE_SIGNATURE_INVALID")
		return
	}
	if register.DeviceID != "" && register.DeviceID != device.DeviceID {
		_ = conn.Close(websocket.StatusCode(4403), "DEVICE_ID_MISMATCH")
		return
	}
	workspaceIDs := sanitizeRuntimeWorkspaceIDs(register.WorkspaceIDs)
	if err := s.Activation.Heartbeat(ctx, device.UserID, device.DeviceID, workspaceIDs, 45*time.Second); err != nil {
		_ = conn.Close(websocket.StatusInternalError, "runtime presence failed")
		return
	}
	if err := writeRuntimeRealtime(ctx, conn, runtimeRealtimeEnvelope{Type: "runtime_registered", Now: time.Now().UnixMilli()}); err != nil {
		return
	}

	readErr := make(chan error, 1)
	go func() {
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				readErr <- err
				return
			}
			var envelope runtimeRealtimeEnvelope
			if json.Unmarshal(data, &envelope) != nil {
				continue
			}
			switch envelope.Type {
			case "runtime_sync", "runtime_pong":
				if envelope.Type == "runtime_sync" {
					workspaceIDs = sanitizeRuntimeWorkspaceIDs(envelope.WorkspaceIDs)
				}
				if heartbeatErr := s.Activation.Heartbeat(ctx, device.UserID, device.DeviceID, workspaceIDs, 45*time.Second); heartbeatErr != nil {
					readErr <- heartbeatErr
					return
				}
			case "runtime_activation_result":
				if envelope.ActivationResult == nil || envelope.ActivationResult.RequestID == "" || envelope.ActivationResult.WorkspaceID == "" {
					continue
				}
				if ackErr := s.Activation.AcknowledgeActivation(ctx, *envelope.ActivationResult); ackErr != nil {
					readErr <- ackErr
					return
				}
			case "runtime_revocation_ack":
				if envelope.RequestID == "" || envelope.WorkspaceID == "" {
					continue
				}
				if _, ackErr := s.Activation.AcknowledgeRevocation(ctx, device.UserID, device.DeviceID, envelope.WorkspaceID, envelope.RequestID); ackErr != nil {
					readErr <- ackErr
					return
				}
			}
		}
	}()

	type nextRuntimeEvent struct {
		activation *cloud.WorkspaceActivation
		revocation *cloud.WorkspaceRevocation
		err        error
	}
	events := make(chan nextRuntimeEvent, 1)
	go func() {
		for {
			activation, revocation, waitErr := s.Activation.WaitForNext(ctx, device.UserID, device.DeviceID, 30*time.Second)
			if waitErr != nil {
				if ctx.Err() == nil {
					events <- nextRuntimeEvent{err: waitErr}
				}
				return
			}
			if activation == nil && revocation == nil {
				continue
			}
			if activation != nil {
				installSystemApp := activation.Action == "install-system-app"
				if installSystemApp && activation.WorkspaceID != cloud.OpenMontageWorkspaceID {
					s.Store.Audit(cloud.AuditEvent{UserID: device.UserID, Event: "workspace.activation_rejected", DeviceID: device.DeviceID, WorkspaceID: activation.WorkspaceID, Detail: map[string]any{"requestId": activation.RequestID, "reason": "unsupported-system-app"}})
					continue
				}
				if !installSystemApp {
					authorized, authzErr := s.Activation.IsAuthorized(ctx, device.UserID, device.DeviceID, activation.WorkspaceID)
					if authzErr != nil {
						events <- nextRuntimeEvent{err: authzErr}
						return
					}
					if !authorized {
						s.Store.Audit(cloud.AuditEvent{UserID: device.UserID, Event: "workspace.activation_rejected", DeviceID: device.DeviceID, WorkspaceID: activation.WorkspaceID, Detail: map[string]any{"requestId": activation.RequestID, "reason": "not-authorized"}})
						continue
					}
				}
			}
			select {
			case events <- nextRuntimeEvent{activation: activation, revocation: revocation}:
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-readErr:
			cancel()
			return
		case event := <-events:
			if event.err != nil {
				_ = conn.Close(websocket.StatusInternalError, "runtime event failed")
				cancel()
				return
			}
			message := runtimeRealtimeEnvelope{Type: "runtime_event", Activation: event.activation, Revocation: event.revocation, Now: time.Now().UnixMilli()}
			if err := writeRuntimeRealtime(ctx, conn, message); err != nil {
				requeueCtx, requeueCancel := context.WithTimeout(context.Background(), 2*time.Second)
				if event.revocation != nil {
					_ = s.Activation.RequestRevocation(requeueCtx, device.UserID, device.DeviceID, *event.revocation, 120*time.Second)
				} else if event.activation != nil {
					_ = s.Activation.Request(requeueCtx, device.UserID, device.DeviceID, *event.activation, 60*time.Second)
				}
				requeueCancel()
				cancel()
				return
			}
		}
	}
}
