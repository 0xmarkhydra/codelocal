package opensandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	metadataOwnerKey     = "codelocal.owner"
	metadataWorkspaceKey = "codelocal.workspace"
	metadataProfileKey   = "codelocal.profile"
	metadataImageKey     = "codelocal.image"
)

type Lifecycle interface {
	CreateSandbox(context.Context, CreateSandboxRequest) (*SandboxInfo, error)
	GetSandbox(context.Context, string) (*SandboxInfo, error)
	ListSandboxes(context.Context, ListOptions) (*ListSandboxesResponse, error)
	ResumeSandbox(context.Context, string) error
	RenewExpiration(context.Context, string, time.Time) (*RenewExpirationResponse, error)
}

type AcquireSpec struct {
	OwnerID        string
	WorkspaceKey   string
	WorkspaceID    string
	Profile        string
	Image          string
	Entrypoint     []string
	Platform       *PlatformSpec
	ResourceLimits map[string]string
	NetworkPolicy  *NetworkPolicy
	TTL            time.Duration
	WaitTimeout    time.Duration
	PollInterval   time.Duration
	Env            map[string]string
}

type Manager struct {
	Lifecycle Lifecycle
	Now       func() time.Time
}

func NewManager(lifecycle Lifecycle) *Manager {
	return &Manager{Lifecycle: lifecycle, Now: time.Now}
}

// Acquire reuses a matching Running/Paused/in-flight sandbox or provisions a
// new one. The caller MUST hold the distributed workspace/session lease before
// calling this method in a multi-replica deployment; metadata lookup alone is
// not a concurrency primitive.
func (m *Manager) Acquire(ctx context.Context, spec AcquireSpec) (*SandboxInfo, error) {
	if err := validateAcquireSpec(spec); err != nil {
		return nil, err
	}
	if m == nil || m.Lifecycle == nil {
		return nil, errors.New("opensandbox lifecycle unavailable")
	}
	metadata := workspaceMetadata(spec)
	listed, err := m.Lifecycle.ListSandboxes(ctx, ListOptions{
		States:   []string{"Running", "Paused", "Creating", "Resuming"},
		Metadata: metadata,
		PageSize: 20,
	})
	if err != nil {
		return nil, err
	}
	if existing := bestReusableSandbox(listed.Items); existing != nil {
		return m.ensureReady(ctx, *existing, spec)
	}

	timeoutSeconds := int(spec.TTL.Seconds())
	request := CreateSandboxRequest{
		Image:          &ImageSpec{URI: spec.Image},
		Entrypoint:     append([]string(nil), spec.Entrypoint...),
		Platform:       spec.Platform,
		Timeout:        &timeoutSeconds,
		ResourceLimits: cloneStringMap(spec.ResourceLimits),
		Env:            cloneStringMap(spec.Env),
		Metadata:       metadata,
		NetworkPolicy:  spec.NetworkPolicy,
	}
	created, err := m.Lifecycle.CreateSandbox(ctx, request)
	if err != nil {
		return nil, err
	}
	return m.ensureReady(ctx, *created, spec)
}

func (m *Manager) ensureReady(ctx context.Context, sandbox SandboxInfo, spec AcquireSpec) (*SandboxInfo, error) {
	switch strings.ToLower(sandbox.Status.State) {
	case "running":
		return m.renew(ctx, sandbox, spec.TTL)
	case "paused":
		if err := m.Lifecycle.ResumeSandbox(ctx, sandbox.ID); err != nil {
			return nil, err
		}
	case "creating", "resuming":
	default:
		return nil, fmt.Errorf("opensandbox %s is not reusable in state %q", sandbox.ID, sandbox.Status.State)
	}

	waitTimeout := spec.WaitTimeout
	if waitTimeout <= 0 {
		waitTimeout = 90 * time.Second
	}
	pollInterval := spec.PollInterval
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	waitCtx, cancel := context.WithTimeout(ctx, waitTimeout)
	defer cancel()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		current, err := m.Lifecycle.GetSandbox(waitCtx, sandbox.ID)
		if err != nil {
			return nil, err
		}
		switch strings.ToLower(current.Status.State) {
		case "running":
			return m.renew(waitCtx, *current, spec.TTL)
		case "error", "deleted", "deleting":
			return nil, fmt.Errorf("opensandbox %s entered state %q: %s", current.ID, current.Status.State, firstNonEmpty(current.Status.Message, current.Status.Reason))
		}
		select {
		case <-waitCtx.Done():
			return nil, fmt.Errorf("wait for opensandbox %s: %w", sandbox.ID, waitCtx.Err())
		case <-ticker.C:
		}
	}
}

func (m *Manager) renew(ctx context.Context, sandbox SandboxInfo, ttl time.Duration) (*SandboxInfo, error) {
	if ttl <= 0 {
		return &sandbox, nil
	}
	now := time.Now
	if m != nil && m.Now != nil {
		now = m.Now
	}
	desired := now().UTC().Add(ttl)
	if sandbox.ExpiresAt != nil && sandbox.ExpiresAt.After(desired.Add(-ttl/4)) {
		return &sandbox, nil
	}
	response, err := m.Lifecycle.RenewExpiration(ctx, sandbox.ID, desired)
	if err != nil {
		return nil, err
	}
	if response != nil && response.ExpiresAt != nil {
		sandbox.ExpiresAt = response.ExpiresAt
	} else {
		sandbox.ExpiresAt = &desired
	}
	return &sandbox, nil
}

func validateAcquireSpec(spec AcquireSpec) error {
	if strings.TrimSpace(spec.OwnerID) == "" {
		return errors.New("opensandbox owner ID required")
	}
	if strings.TrimSpace(spec.WorkspaceKey) == "" {
		return errors.New("opensandbox workspace key required")
	}
	if strings.TrimSpace(spec.WorkspaceID) == "" {
		return errors.New("opensandbox workspace ID required")
	}
	if strings.TrimSpace(spec.Profile) == "" {
		return errors.New("opensandbox runtime profile required")
	}
	if strings.TrimSpace(spec.Image) == "" {
		return errors.New("opensandbox runtime image required")
	}
	if len(spec.ResourceLimits) == 0 {
		return errors.New("opensandbox resource limits required")
	}
	if spec.TTL <= 0 {
		return errors.New("opensandbox TTL must be positive")
	}
	return nil
}

func workspaceMetadata(spec AcquireSpec) map[string]string {
	return map[string]string{
		metadataOwnerKey:     stableScopeHash(spec.OwnerID),
		metadataWorkspaceKey: stableScopeHash(spec.WorkspaceKey),
		metadataProfileKey:   spec.Profile,
		metadataImageKey:     spec.Image,
	}
}

func stableScopeHash(value string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(digest[:16])
}

func bestReusableSandbox(items []SandboxInfo) *SandboxInfo {
	if len(items) == 0 {
		return nil
	}
	priority := map[string]int{"running": 0, "paused": 1, "resuming": 2, "creating": 3}
	copyItems := append([]SandboxInfo(nil), items...)
	sort.SliceStable(copyItems, func(i, j int) bool {
		pi, iok := priority[strings.ToLower(copyItems[i].Status.State)]
		pj, jok := priority[strings.ToLower(copyItems[j].Status.State)]
		if !iok {
			pi = 99
		}
		if !jok {
			pj = 99
		}
		if pi != pj {
			return pi < pj
		}
		return copyItems[i].CreatedAt.After(copyItems[j].CreatedAt)
	})
	switch strings.ToLower(copyItems[0].Status.State) {
	case "running", "paused", "resuming", "creating":
		item := copyItems[0]
		return &item
	default:
		return nil
	}
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
