package gateway

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

type WorkspaceView struct {
	Key             string         `json:"key"`
	DeviceID        string         `json:"deviceId"`
	DeviceName      string         `json:"deviceName"`
	WorkspaceID     string         `json:"workspaceId"`
	WorkspaceName   string         `json:"workspaceName"`
	Status          string         `json:"status"`
	RuntimeOnline   bool           `json:"runtimeOnline"`
	Authorized      any            `json:"authorized"`
	ClientVersion   string         `json:"clientVersion,omitempty"`
	ProtocolVersion int            `json:"protocolVersion"`
	ProjectRoot     any            `json:"projectRoot"`
	Capabilities    map[string]any `json:"capabilities"`
	LastSeenAt      int64          `json:"lastSeenAt"`
}

type WorkspaceService struct {
	Store       *cloud.Store
	Activation  *cloud.ActivationStore
	Hub         *Hub
	Coordinator *Coordinator
}

func (s *WorkspaceService) Catalog(ctx context.Context, userID string) ([]WorkspaceView, error) {
	records, err := s.Store.ListWorkspaceRecords(ctx, userID)
	if err != nil {
		return nil, err
	}
	devices, err := s.Store.ListDevices(ctx, userID)
	if err != nil {
		return nil, err
	}
	deviceNames := map[string]string{}
	for _, d := range devices {
		deviceNames[d.DeviceID] = d.DeviceName
	}
	deviceIDs := []string{}
	seen := map[string]struct{}{}
	for _, w := range records {
		if _, ok := seen[w.DeviceID]; !ok {
			seen[w.DeviceID] = struct{}{}
			deviceIDs = append(deviceIDs, w.DeviceID)
		}
	}
	type runtimeState struct {
		online     bool
		authorized map[string]struct{}
	}
	states := map[string]runtimeState{}
	for _, deviceID := range deviceIDs {
		online, onlineErr := s.Activation.IsOnline(ctx, userID, deviceID)
		if onlineErr != nil {
			online = false
		}
		authorized := map[string]struct{}{}
		if online {
			ids, _ := s.Activation.AuthorizedIDs(ctx, userID, deviceID)
			for _, id := range ids {
				authorized[id] = struct{}{}
			}
		}
		states[deviceID] = runtimeState{online: online, authorized: authorized}
	}
	out := []WorkspaceView{}
	for _, w := range records {
		key := ClientKey(userID, w.DeviceID, w.WorkspaceID)
		owner, _ := s.Coordinator.Owner(ctx, key)
		local := s.Hub.localClient(key)
		active := owner != "" || local != nil
		state := states[w.DeviceID]
		_, authorizedNow := state.authorized[w.WorkspaceID]
		if !active && state.online && !authorizedNow {
			continue
		}
		status := "device_offline"
		if active {
			status = "active"
		} else if state.online {
			status = "sleeping"
		}
		var authorized any = nil
		if active {
			authorized = true
		} else if state.online {
			authorized = authorizedNow
		}
		clientVersion := ""
		if v, ok := w.Capabilities["clientVersion"].(string); ok {
			clientVersion = v
		}
		if local != nil && local.ClientVersion != "" {
			clientVersion = local.ClientVersion
		}
		caps := w.Capabilities
		var projectRoot any
		deviceName := deviceNames[w.DeviceID]
		if deviceName == "" {
			deviceName = w.DeviceID
		}
		if local != nil {
			caps = map[string]any{"filesystem": local.Capabilities.Filesystem, "git": local.Capabilities.Git, "shell": local.Capabilities.Shell, "pty": local.Capabilities.PTY, "sandbox": local.Capabilities.Sandbox, "semanticProviders": local.Capabilities.SemanticProviders, "idempotency": local.Capabilities.Idempotency, "cancellation": local.Capabilities.Cancellation, "approvals": local.Capabilities.Approvals, "approvalMemory": local.Capabilities.ApprovalMemory, "hostPolicyExecution": local.Capabilities.HostPolicyExecution, "mcpHub": local.Capabilities.MCPHub, "terminalChatApproval": local.Capabilities.TerminalChatApproval, "terminalHistory": local.Capabilities.TerminalHistory}
			projectRoot = local.ProjectRoot
			deviceName = local.DeviceName
		}
		out = append(out, WorkspaceView{Key: key, DeviceID: w.DeviceID, DeviceName: deviceName, WorkspaceID: w.WorkspaceID, WorkspaceName: w.WorkspaceName, Status: status, RuntimeOnline: state.online, Authorized: authorized, ClientVersion: clientVersion, ProtocolVersion: w.ProtocolVersion, ProjectRoot: projectRoot, Capabilities: caps, LastSeenAt: w.LastSeenAt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeenAt > out[j].LastSeenAt })
	return out, nil
}

func (s *WorkspaceService) Activate(ctx context.Context, userID, key string) (*WorkspaceView, error) {
	if owner, _ := s.Coordinator.Owner(ctx, key); owner != "" {
		catalog, err := s.Catalog(ctx, userID)
		if err != nil {
			return nil, err
		}
		for i := range catalog {
			if catalog[i].Key == key {
				return &catalog[i], nil
			}
		}
	}
	catalog, err := s.Catalog(ctx, userID)
	if err != nil {
		return nil, err
	}
	var workspace *WorkspaceView
	for i := range catalog {
		if catalog[i].Key == key {
			copy := catalog[i]
			workspace = &copy
			break
		}
	}
	if workspace == nil {
		return nil, fmt.Errorf("workspace is not available or no longer authorized: %s", key)
	}
	if !workspace.RuntimeOnline {
		return nil, fmt.Errorf("the device for %s is offline; run `codelocal` on that device", workspace.WorkspaceName)
	}
	if workspace.Authorized != true {
		return nil, fmt.Errorf("workspace is not authorized by the local CodeLocal runtime: %s", workspace.WorkspaceName)
	}
	requestID := cloud.RandomHex(16)
	if err := s.Activation.Request(ctx, userID, workspace.DeviceID, cloud.WorkspaceActivation{WorkspaceID: workspace.WorkspaceID, RequestID: requestID, RequestedAt: time.Now().UnixMilli()}, 60*time.Second); err != nil {
		return nil, err
	}
	s.Store.Audit(cloud.AuditEvent{UserID: userID, Event: "workspace.activation_requested", DeviceID: workspace.DeviceID, WorkspaceID: workspace.WorkspaceID, Detail: map[string]any{"requestId": requestID}})
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := s.Coordinator.WaitOwner(waitCtx, key); err != nil {
		return nil, errors.New("workspace activation timed out; keep `codelocal` running and try again")
	}
	s.Store.Audit(cloud.AuditEvent{UserID: userID, Event: "workspace.activated", DeviceID: workspace.DeviceID, WorkspaceID: workspace.WorkspaceID, Detail: map[string]any{"requestId": requestID}})
	// Refresh after activation so protocol/capability gating sees the capabilities
	// advertised by the newly connected client rather than the sleeping catalog
	// placeholder created during workspace sync.
	if refreshed, refreshErr := s.Catalog(ctx, userID); refreshErr == nil {
		for i := range refreshed {
			if refreshed[i].Key == key {
				return &refreshed[i], nil
			}
		}
	}
	workspace.Status = "active"
	workspace.Authorized = true
	return workspace, nil
}

func (s *WorkspaceService) Revoke(ctx context.Context, userID, deviceID, workspaceID string) error {
	online, err := s.Activation.IsOnline(ctx, userID, deviceID)
	if err != nil {
		return err
	}
	if !online {
		return errors.New("that CodeLocal machine runtime is offline")
	}
	requestID := cloud.RandomHex(16)
	if err := s.Activation.RequestRevocation(ctx, userID, deviceID, cloud.WorkspaceRevocation{WorkspaceID: workspaceID, RequestID: requestID, RequestedAt: time.Now().UnixMilli()}, 120*time.Second); err != nil {
		return err
	}
	if err := s.Activation.WaitForRevocationAck(ctx, requestID, 8*time.Second); err != nil {
		return err
	}
	s.Store.Audit(cloud.AuditEvent{UserID: userID, Event: "workspace.revoked", DeviceID: deviceID, WorkspaceID: workspaceID, Detail: map[string]any{"requestId": requestID}})
	return nil
}
