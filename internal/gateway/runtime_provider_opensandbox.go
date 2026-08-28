package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/gateway/opensandbox"
)

type OpenSandboxProvisioner interface {
	Acquire(context.Context, opensandbox.AcquireSpec) (*opensandbox.SandboxInfo, error)
	EnsureExisting(context.Context, opensandbox.AcquireSpec) (*opensandbox.SandboxInfo, bool, error)
	SnapshotAndDelete(context.Context, string, string, time.Duration) (*opensandbox.SnapshotInfo, error)
}

type OpenSandboxRuntimeProvider struct {
	Provisioner    OpenSandboxProvisioner
	Workspaces     *WorkspaceService
	Hub            *Hub
	Coordinator    *Coordinator
	Leases         *RuntimeLeaseCoordinator
	Bootstrap      *RuntimeBootstrapStore
	Sessions       *CloudRuntimeSessionStore
	ServerURL      string
	Image          string
	Profile        string
	ResourceLimits map[string]string
	NetworkPolicy  *opensandbox.NetworkPolicy
	SandboxTTL     time.Duration
	ReadyTimeout   time.Duration
	AliasTTL       time.Duration
	IdleTimeout    time.Duration
	ReaperOwner    string
}

func (p *OpenSandboxRuntimeProvider) Kind() RuntimeProviderKind { return RuntimeProviderCloud }

func (p *OpenSandboxRuntimeProvider) Acquire(ctx context.Context, request RuntimeAcquireRequest) (*WorkspaceView, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if err := p.Sessions.WaitReapClear(ctx, request.WorkspaceKey, p.Profile); err != nil {
		return nil, fmt.Errorf("wait for cloud workspace checkpoint: %w", err)
	}
	source, err := p.sourceWorkspace(ctx, request.UserID, request.WorkspaceKey)
	if err != nil {
		return nil, err
	}
	runtimeSessionID, deviceID := managedRuntimeIdentity(request.UserID, request.WorkspaceKey, p.Profile)
	targetKey := ClientKey(request.UserID, deviceID, source.WorkspaceID)
	snapshotName := managedSnapshotName(request.UserID, request.WorkspaceKey, p.Profile, p.Image)

	// Existing compute is renewed/resumed before consulting its runtime owner.
	// EnsureExisting is create-free, so a stale Redis session can never spawn a
	// sandbox with incomplete bootstrap data.
	baseSpec := p.acquireSpec(request, source, snapshotName, nil)
	if sandbox, found, existingErr := p.Provisioner.EnsureExisting(ctx, baseSpec); existingErr != nil {
		if opensandbox.IsUnavailable(existingErr) {
			return nil, fmt.Errorf("%w: OpenSandbox: %v", ErrRuntimeProviderUnavailable, existingErr)
		}
		return nil, existingErr
	} else if found {
		state := p.sessionState(request, targetKey, sandbox.ID, snapshotName)
		if err := p.Sessions.Touch(ctx, state); err != nil {
			return nil, err
		}
		if runtimeWorkspace, ok := p.activeCloudWorkspace(ctx, request.UserID, request.WorkspaceKey, targetKey); ok {
			return projectCloudWorkspace(source, runtimeWorkspace), nil
		}
		runtimeWorkspace, waitErr := p.waitCloudWorkspace(ctx, request.UserID, request.WorkspaceKey, targetKey)
		if waitErr == nil {
			_ = p.Sessions.Touch(ctx, state)
			return projectCloudWorkspace(source, runtimeWorkspace), nil
		}
		return nil, waitErr
	}

	repositories, err := p.Workspaces.Store.WorkspaceRepositorySources(ctx, request.UserID, source.DeviceID, source.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("load cloud workspace sources: %w", err)
	}
	if len(repositories) == 0 {
		return nil, fmt.Errorf("%w: workspace has no credential-free durable Git source for cloud hydration", ErrRuntimeProviderUnavailable)
	}
	repositoriesJSON, err := json.Marshal(repositories)
	if err != nil {
		return nil, err
	}

	lease, err := p.Leases.Acquire(ctx, RuntimeLeaseScope{UserID: request.UserID, WorkspaceKey: request.WorkspaceKey, Provider: RuntimeProviderCloud, Profile: p.Profile})
	if errors.Is(err, ErrRuntimeLeaseHeld) {
		return p.waitForPeerProvision(ctx, request.UserID, request.WorkspaceKey, source)
	}
	if err != nil {
		return nil, fmt.Errorf("acquire cloud runtime lease: %w", err)
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = lease.Release(releaseCtx)
	}()

	// Another replica may have completed while this replica waited for lease.
	if sandbox, found, existingErr := p.Provisioner.EnsureExisting(ctx, baseSpec); existingErr == nil && found {
		state := p.sessionState(request, targetKey, sandbox.ID, snapshotName)
		_ = p.Sessions.Touch(ctx, state)
		runtimeWorkspace, waitErr := p.waitCloudWorkspace(ctx, request.UserID, request.WorkspaceKey, targetKey)
		if waitErr != nil {
			return nil, waitErr
		}
		return projectCloudWorkspace(source, runtimeWorkspace), nil
	}

	bootstrapToken, err := p.Bootstrap.Issue(ctx, RuntimeBootstrapState{UserID: request.UserID, WorkspaceKey: request.WorkspaceKey, WorkspaceID: source.WorkspaceID, Profile: p.Profile, RuntimeSessionID: runtimeSessionID, DeviceID: deviceID, DeviceName: "CodeLocal Cloud"})
	if err != nil {
		return nil, fmt.Errorf("issue cloud runtime bootstrap: %w", err)
	}
	env := map[string]string{
		"CODELOCAL_CLOUD_SERVER":              p.ServerURL,
		"CODELOCAL_RUNTIME_BOOTSTRAP_TOKEN":   bootstrapToken,
		"CODELOCAL_RUNTIME_SESSION_ID":        runtimeSessionID,
		"CODELOCAL_RUNTIME_DEVICE_ID":         deviceID,
		"CODELOCAL_RUNTIME_DEVICE_NAME":       "CodeLocal Cloud",
		"CODELOCAL_RUNTIME_WORKSPACE_ID":      source.WorkspaceID,
		"CODELOCAL_RUNTIME_WORKSPACE_NAME":    source.WorkspaceName,
		"CODELOCAL_RUNTIME_REPOSITORIES_JSON": string(repositoriesJSON),
		"CODELOCAL_WORKSPACE_PATH":            "/workspace",
	}
	sandbox, err := p.Provisioner.Acquire(ctx, p.acquireSpec(request, source, snapshotName, env))
	if err != nil {
		if opensandbox.IsUnavailable(err) {
			return nil, fmt.Errorf("%w: OpenSandbox: %v", ErrRuntimeProviderUnavailable, err)
		}
		return nil, fmt.Errorf("provision OpenSandbox runtime: %w", err)
	}
	state := p.sessionState(request, targetKey, sandbox.ID, snapshotName)
	if err := p.Sessions.Touch(ctx, state); err != nil {
		return nil, err
	}
	runtimeWorkspace, err := p.waitCloudWorkspace(ctx, request.UserID, request.WorkspaceKey, targetKey)
	if err != nil {
		return nil, err
	}
	return projectCloudWorkspace(source, runtimeWorkspace), nil
}

func (p *OpenSandboxRuntimeProvider) Call(ctx context.Context, request RuntimeCallRequest) (RoutedResult, error) {
	if p == nil || p.Hub == nil || p.Sessions == nil {
		return RoutedResult{}, errors.New("cloud runtime gateway unavailable")
	}
	finish, err := p.Sessions.BeginCall(ctx, request.WorkspaceKey, p.Profile)
	if err != nil {
		return RoutedResult{}, fmt.Errorf("mark cloud runtime call active: %w", err)
	}
	defer finish()
	return p.Hub.Call(ctx, request.UserID, request.WorkspaceKey, request.SessionID, request.Tool, request.Args, request.SideEffect, request.RequestID)
}

func (p *OpenSandboxRuntimeProvider) acquireSpec(request RuntimeAcquireRequest, source *WorkspaceView, snapshotName string, env map[string]string) opensandbox.AcquireSpec {
	return opensandbox.AcquireSpec{OwnerID: request.UserID, WorkspaceKey: request.WorkspaceKey, WorkspaceID: source.WorkspaceID, Profile: p.Profile, Image: p.Image, SnapshotName: snapshotName, Entrypoint: []string{"/usr/local/bin/codelocal-cloud-runtime"}, ResourceLimits: cloneRuntimeStrings(p.ResourceLimits), NetworkPolicy: p.NetworkPolicy, TTL: p.SandboxTTL, Env: env}
}

func (p *OpenSandboxRuntimeProvider) sessionState(request RuntimeAcquireRequest, targetKey, sandboxID, snapshotName string) CloudRuntimeSession {
	return CloudRuntimeSession{ID: cloudRuntimeSessionID(request.WorkspaceKey, p.Profile), UserID: request.UserID, WorkspaceKey: request.WorkspaceKey, TargetKey: targetKey, SandboxID: sandboxID, SnapshotName: snapshotName, Profile: p.Profile}
}

func (p *OpenSandboxRuntimeProvider) validate() error {
	if p == nil || p.Provisioner == nil || p.Workspaces == nil || p.Workspaces.Store == nil || p.Hub == nil || p.Coordinator == nil || p.Leases == nil || p.Bootstrap == nil || p.Sessions == nil {
		return errors.New("OpenSandbox runtime provider is not fully configured")
	}
	if strings.TrimSpace(p.ServerURL) == "" || strings.TrimSpace(p.Image) == "" || strings.TrimSpace(p.Profile) == "" {
		return errors.New("OpenSandbox runtime server, image and profile are required")
	}
	if len(p.ResourceLimits) == 0 {
		return errors.New("OpenSandbox runtime resource limits are required")
	}
	if p.SandboxTTL <= 0 {
		return errors.New("OpenSandbox runtime TTL must be positive")
	}
	return nil
}

func (p *OpenSandboxRuntimeProvider) sourceWorkspace(ctx context.Context, userID, key string) (*WorkspaceView, error) {
	catalog, err := p.Workspaces.Catalog(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range catalog {
		if catalog[i].Key == key {
			copy := catalog[i]
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("workspace is not available or no longer authorized: %s", key)
}

func (p *OpenSandboxRuntimeProvider) activeCloudWorkspace(ctx context.Context, userID, sourceKey, targetKey string) (*WorkspaceView, bool) {
	owner, err := p.Coordinator.Owner(ctx, targetKey)
	if err != nil || owner == "" {
		return nil, false
	}
	catalog, err := p.Workspaces.catalogAll(ctx, userID)
	if err != nil {
		return nil, false
	}
	for i := range catalog {
		if catalog[i].Key != targetKey {
			continue
		}
		copy := catalog[i]
		copy.Status = "active"
		copy.RuntimeOnline = true
		copy.Authorized = true
		if p.Coordinator.BindRuntimeAlias(ctx, sourceKey, targetKey, p.aliasTTL()) != nil {
			return nil, false
		}
		return &copy, true
	}
	return nil, false
}

func (p *OpenSandboxRuntimeProvider) waitForPeerProvision(ctx context.Context, userID, sourceKey string, source *WorkspaceView) (*WorkspaceView, error) {
	waitCtx, cancel := context.WithTimeout(ctx, p.readyTimeout())
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		targetKey, err := p.Coordinator.ResolveRuntimeAlias(waitCtx, sourceKey)
		if err != nil {
			return nil, err
		}
		if targetKey != sourceKey {
			if runtimeWorkspace, ok := p.activeCloudWorkspace(waitCtx, userID, sourceKey, targetKey); ok {
				return projectCloudWorkspace(source, runtimeWorkspace), nil
			}
		}
		select {
		case <-waitCtx.Done():
			return nil, fmt.Errorf("%w: another replica is still provisioning the cloud runtime", ErrRuntimeProviderUnavailable)
		case <-ticker.C:
		}
	}
}

func (p *OpenSandboxRuntimeProvider) waitCloudWorkspace(ctx context.Context, userID, sourceKey, targetKey string) (*WorkspaceView, error) {
	waitCtx, cancel := context.WithTimeout(ctx, p.readyTimeout())
	defer cancel()
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		catalog, err := p.Workspaces.catalogAll(waitCtx, userID)
		if err == nil {
			for i := range catalog {
				candidate := catalog[i]
				if candidate.Key != targetKey || !candidate.RuntimeOnline {
					continue
				}
				activated, activateErr := p.Workspaces.ActivateLocal(waitCtx, userID, targetKey)
				if activateErr == nil {
					if err := p.Coordinator.BindRuntimeAlias(waitCtx, sourceKey, targetKey, p.aliasTTL()); err != nil {
						return nil, err
					}
					return activated, nil
				}
				if !localRuntimeUnavailable(activateErr) {
					return nil, activateErr
				}
			}
		}
		select {
		case <-waitCtx.Done():
			return nil, fmt.Errorf("%w: cloud runtime did not register before timeout", ErrRuntimeProviderUnavailable)
		case <-ticker.C:
		}
	}
}

func (p *OpenSandboxRuntimeProvider) StartIdleReaper(ctx context.Context, interval time.Duration) {
	if p == nil || p.Sessions == nil || p.Provisioner == nil {
		return
	}
	if interval <= 0 {
		interval = time.Minute
	}
	if p.IdleTimeout <= 0 {
		p.IdleTimeout = 20 * time.Minute
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.reapIdle(ctx)
			}
		}
	}()
}

func (p *OpenSandboxRuntimeProvider) reapIdle(ctx context.Context) {
	before := time.Now().Add(-p.IdleTimeout)
	due, err := p.Sessions.Due(ctx, before, 64)
	if err != nil {
		slog.Warn("cloud runtime idle scan failed", "error", err)
		return
	}
	for _, candidate := range due {
		claimed, claimErr := p.Sessions.ClaimReap(ctx, candidate.ID, p.ReaperOwner, 4*time.Minute)
		if claimErr != nil || !claimed {
			continue
		}
		func(state CloudRuntimeSession) {
			defer func() {
				releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_ = p.Sessions.ReleaseReap(releaseCtx, state.ID, p.ReaperOwner)
			}()
			fresh, readErr := p.Sessions.Get(ctx, state.WorkspaceKey, state.Profile)
			if readErr != nil || fresh == nil || fresh.SandboxID == "" || fresh.LastActiveAt > before.UnixMilli() {
				return
			}
			busy, busyErr := p.Sessions.InFlight(ctx, fresh.ID)
			if busyErr != nil || busy {
				return
			}
			snapshot, snapshotErr := p.Provisioner.SnapshotAndDelete(ctx, fresh.SandboxID, fresh.SnapshotName, 3*time.Minute)
			if snapshotErr != nil {
				slog.Warn("cloud runtime checkpoint failed; compute retained", "sandboxId", fresh.SandboxID, "error", snapshotErr)
				return
			}
			_ = snapshot
			_ = p.Coordinator.ReleaseRuntimeAlias(ctx, fresh.WorkspaceKey)
			if err := p.Sessions.MarkSnapshotted(ctx, *fresh); err != nil {
				slog.Warn("cloud runtime checkpoint metadata update failed", "sandboxId", fresh.SandboxID, "error", err)
			}
		}(candidate)
	}
}

func (p *OpenSandboxRuntimeProvider) readyTimeout() time.Duration {
	if p.ReadyTimeout > 0 {
		return p.ReadyTimeout
	}
	return 60 * time.Second
}

func (p *OpenSandboxRuntimeProvider) aliasTTL() time.Duration {
	if p.AliasTTL > 0 {
		return p.AliasTTL
	}
	if p.SandboxTTL > 0 {
		return p.SandboxTTL + 5*time.Minute
	}
	return defaultRuntimeAliasTTL
}

func managedRuntimeIdentity(userID, workspaceKey, profile string) (sessionID, deviceID string) {
	digest := sha256.Sum256([]byte(strings.Join([]string{strings.TrimSpace(userID), strings.TrimSpace(workspaceKey), strings.TrimSpace(profile)}, "\x00")))
	hexID := hex.EncodeToString(digest[:])
	return "crs_" + hexID[:32], "cloud-" + hexID[:24]
}

func managedSnapshotName(userID, workspaceKey, profile, image string) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{strings.TrimSpace(userID), strings.TrimSpace(workspaceKey), strings.TrimSpace(profile), strings.TrimSpace(image)}, "\x00")))
	return "codelocal-" + hex.EncodeToString(digest[:16])
}

func managedRuntimeDeviceID(value string) bool {
	value = strings.TrimSpace(value)
	const prefix = "cloud-"
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+24 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, prefix))
	return err == nil
}

func projectCloudWorkspace(source, runtimeWorkspace *WorkspaceView) *WorkspaceView {
	if source == nil || runtimeWorkspace == nil {
		return nil
	}
	view := *source
	view.Status = "active"
	view.RuntimeOnline = true
	view.Authorized = true
	view.ClientVersion = runtimeWorkspace.ClientVersion
	view.ProtocolVersion = runtimeWorkspace.ProtocolVersion
	view.Capabilities = runtimeWorkspace.Capabilities
	view.LastSeenAt = runtimeWorkspace.LastSeenAt
	return &view
}

func cloneRuntimeStrings(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
