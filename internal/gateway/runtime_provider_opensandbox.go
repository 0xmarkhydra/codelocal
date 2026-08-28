package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/gateway/opensandbox"
)

type OpenSandboxProvisioner interface {
	Acquire(context.Context, opensandbox.AcquireSpec) (*opensandbox.SandboxInfo, error)
}

type OpenSandboxRuntimeProvider struct {
	Provisioner    OpenSandboxProvisioner
	Workspaces     *WorkspaceService
	Hub            *Hub
	Coordinator    *Coordinator
	Leases         *RuntimeLeaseCoordinator
	Bootstrap      *RuntimeBootstrapStore
	ServerURL      string
	Image          string
	Profile        string
	ResourceLimits map[string]string
	NetworkPolicy  *opensandbox.NetworkPolicy
	SandboxTTL     time.Duration
	ReadyTimeout   time.Duration
	AliasTTL       time.Duration
}

func (p *OpenSandboxRuntimeProvider) Kind() RuntimeProviderKind { return RuntimeProviderCloud }

func (p *OpenSandboxRuntimeProvider) Acquire(ctx context.Context, request RuntimeAcquireRequest) (*WorkspaceView, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	source, err := p.sourceWorkspace(ctx, request.UserID, request.WorkspaceKey)
	if err != nil {
		return nil, err
	}
	runtimeSessionID, deviceID := managedRuntimeIdentity(request.UserID, request.WorkspaceKey, p.Profile)
	targetKey := ClientKey(request.UserID, deviceID, source.WorkspaceID)

	// Fast path for a live managed runtime, including a runtime owned by another
	// Railway replica. The alias is refreshed before returning the binding.
	if workspace, ok := p.activeCloudWorkspace(ctx, request.UserID, request.WorkspaceKey, targetKey); ok {
		return workspace, nil
	}

	lease, err := p.Leases.Acquire(ctx, RuntimeLeaseScope{
		UserID:       request.UserID,
		WorkspaceKey: request.WorkspaceKey,
		Provider:     RuntimeProviderCloud,
		Profile:      p.Profile,
	})
	if errors.Is(err, ErrRuntimeLeaseHeld) {
		return p.waitForPeerProvision(ctx, request.UserID, request.WorkspaceKey)
	}
	if err != nil {
		return nil, fmt.Errorf("acquire cloud runtime lease: %w", err)
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = lease.Release(releaseCtx)
	}()

	// Re-check after acquiring the lease. A previous owner may have completed
	// provisioning just before its lease expired.
	if workspace, ok := p.activeCloudWorkspace(ctx, request.UserID, request.WorkspaceKey, targetKey); ok {
		return workspace, nil
	}

	bootstrapToken, err := p.Bootstrap.Issue(ctx, RuntimeBootstrapState{
		UserID:           request.UserID,
		WorkspaceKey:     request.WorkspaceKey,
		WorkspaceID:      source.WorkspaceID,
		Profile:          p.Profile,
		RuntimeSessionID: runtimeSessionID,
		DeviceID:         deviceID,
		DeviceName:       "CodeLocal Cloud",
	})
	if err != nil {
		return nil, fmt.Errorf("issue cloud runtime bootstrap: %w", err)
	}

	_, err = p.Provisioner.Acquire(ctx, opensandbox.AcquireSpec{
		OwnerID:        request.UserID,
		WorkspaceKey:   request.WorkspaceKey,
		WorkspaceID:    source.WorkspaceID,
		Profile:        p.Profile,
		Image:          p.Image,
		Entrypoint:     []string{"/usr/local/bin/codelocal-cloud-runtime"},
		ResourceLimits: cloneRuntimeStrings(p.ResourceLimits),
		NetworkPolicy:  p.NetworkPolicy,
		TTL:            p.SandboxTTL,
		Env: map[string]string{
			"CODELOCAL_CLOUD_SERVER":              p.ServerURL,
			"CODELOCAL_RUNTIME_BOOTSTRAP_TOKEN":   bootstrapToken,
			"CODELOCAL_RUNTIME_SESSION_ID":        runtimeSessionID,
			"CODELOCAL_RUNTIME_DEVICE_ID":          deviceID,
			"CODELOCAL_RUNTIME_DEVICE_NAME":        "CodeLocal Cloud",
			"CODELOCAL_RUNTIME_WORKSPACE_NAME":     source.WorkspaceName,
			"CODELOCAL_WORKSPACE_PATH":             "/workspace",
		},
	})
	if err != nil {
		if opensandbox.IsUnavailable(err) {
			return nil, fmt.Errorf("%w: OpenSandbox: %v", ErrRuntimeProviderUnavailable, err)
		}
		return nil, fmt.Errorf("provision OpenSandbox runtime: %w", err)
	}

	workspace, err := p.waitCloudWorkspace(ctx, request.UserID, request.WorkspaceKey, targetKey)
	if err != nil {
		return nil, err
	}
	return workspace, nil
}

func (p *OpenSandboxRuntimeProvider) Call(ctx context.Context, request RuntimeCallRequest) (RoutedResult, error) {
	if p == nil || p.Hub == nil {
		return RoutedResult{}, errors.New("cloud runtime gateway unavailable")
	}
	return p.Hub.Call(ctx, request.UserID, request.WorkspaceKey, request.SessionID, request.Tool, request.Args, request.SideEffect, request.RequestID)
}

func (p *OpenSandboxRuntimeProvider) validate() error {
	if p == nil || p.Provisioner == nil || p.Workspaces == nil || p.Hub == nil || p.Coordinator == nil || p.Leases == nil || p.Bootstrap == nil {
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
	// Missing/unauthorized workspaces are hard failures. Auto must not provision
	// a fresh cloud copy when the product workspace no longer belongs to user.
	return nil, fmt.Errorf("workspace is not available or no longer authorized: %s", key)
}

func (p *OpenSandboxRuntimeProvider) activeCloudWorkspace(ctx context.Context, userID, sourceKey, targetKey string) (*WorkspaceView, bool) {
	owner, err := p.Coordinator.Owner(ctx, targetKey)
	if err != nil || owner == "" {
		return nil, false
	}
	catalog, err := p.Workspaces.Catalog(ctx, userID)
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

func (p *OpenSandboxRuntimeProvider) waitForPeerProvision(ctx context.Context, userID, sourceKey string) (*WorkspaceView, error) {
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
			if workspace, ok := p.activeCloudWorkspace(waitCtx, userID, sourceKey, targetKey); ok {
				return workspace, nil
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
		catalog, err := p.Workspaces.Catalog(waitCtx, userID)
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

func cloneRuntimeStrings(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
