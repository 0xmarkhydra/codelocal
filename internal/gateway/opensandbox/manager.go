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

type SnapshotLifecycle interface {
	CreateSnapshot(context.Context, string, string) (*SnapshotInfo, error)
	GetSnapshot(context.Context, string) (*SnapshotInfo, error)
	ListSnapshots(context.Context, SnapshotListOptions) (*ListSnapshotsResponse, error)
	DeleteSnapshot(context.Context, string) error
	DeleteSandbox(context.Context, string) error
}

type AcquireSpec struct {
	OwnerID        string
	WorkspaceKey   string
	WorkspaceID    string
	Profile        string
	Image          string
	SnapshotName   string
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

// Acquire reuses a matching Running/Paused/in-flight sandbox, restores the
// newest Ready named snapshot when available, or provisions a new image-backed
// sandbox. The caller MUST hold the distributed workspace/session lease before
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
		States:   []string{"Running", "Paused", "Pending", "Creating", "Resuming"},
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
		Entrypoint:     append([]string(nil), spec.Entrypoint...),
		Platform:       spec.Platform,
		Timeout:        &timeoutSeconds,
		ResourceLimits: cloneStringMap(spec.ResourceLimits),
		Env:            cloneStringMap(spec.Env),
		Metadata:       metadata,
		NetworkPolicy:  spec.NetworkPolicy,
	}
	if snapshot := m.newestReadySnapshot(ctx, strings.TrimSpace(spec.SnapshotName)); snapshot != nil {
		request.SnapshotID = snapshot.ID
	} else {
		request.Image = &ImageSpec{URI: spec.Image}
	}
	created, err := m.Lifecycle.CreateSandbox(ctx, request)
	if err != nil {
		return nil, err
	}
	return m.ensureReady(ctx, *created, spec)
}

// EnsureExisting renews/resumes a reusable matching sandbox without ever
// creating one. It is safe for request hot paths where a stale session record
// must not accidentally provision a sandbox with incomplete bootstrap env.
func (m *Manager) EnsureExisting(ctx context.Context, spec AcquireSpec) (*SandboxInfo, bool, error) {
	if err := validateAcquireSpec(spec); err != nil {
		return nil, false, err
	}
	if m == nil || m.Lifecycle == nil {
		return nil, false, errors.New("opensandbox lifecycle unavailable")
	}
	listed, err := m.Lifecycle.ListSandboxes(ctx, ListOptions{
		States:   []string{"Running", "Paused", "Pending", "Creating", "Resuming"},
		Metadata: workspaceMetadata(spec),
		PageSize: 20,
	})
	if err != nil {
		return nil, false, err
	}
	existing := bestReusableSandbox(listed.Items)
	if existing == nil {
		return nil, false, nil
	}
	ready, err := m.ensureReady(ctx, *existing, spec)
	return ready, err == nil, err
}

func (m *Manager) ensureReady(ctx context.Context, sandbox SandboxInfo, spec AcquireSpec) (*SandboxInfo, error) {
	switch strings.ToLower(sandbox.Status.State) {
	case "running":
		return m.renew(ctx, sandbox, spec.TTL)
	case "paused":
		if err := m.Lifecycle.ResumeSandbox(ctx, sandbox.ID); err != nil {
			return nil, err
		}
	case "pending", "creating", "resuming":
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
		case "failed", "error", "terminated", "stopping", "deleted", "deleting":
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

func (m *Manager) newestReadySnapshot(ctx context.Context, name string) *SnapshotInfo {
	if name == "" {
		return nil
	}
	lifecycle, ok := m.Lifecycle.(SnapshotLifecycle)
	if !ok {
		return nil
	}
	listed, err := lifecycle.ListSnapshots(ctx, SnapshotListOptions{Name: name, States: []string{"Ready"}, PageSize: 20})
	if err != nil || listed == nil || len(listed.Items) == 0 {
		return nil
	}
	items := append([]SnapshotInfo(nil), listed.Items...)
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	item := items[0]
	return &item
}

// SnapshotAndDelete persists a Running sandbox, waits for a Ready snapshot,
// then removes the compute sandbox. The persistent snapshot is retained for a
// later Acquire restore. Data is never discarded when snapshot creation fails.
func (m *Manager) SnapshotAndDelete(ctx context.Context, sandboxID, snapshotName string, waitTimeout time.Duration) (*SnapshotInfo, error) {
	lifecycle, ok := m.Lifecycle.(SnapshotLifecycle)
	if !ok {
		return nil, errors.New("opensandbox snapshot lifecycle unavailable")
	}
	if strings.TrimSpace(sandboxID) == "" || strings.TrimSpace(snapshotName) == "" {
		return nil, errors.New("opensandbox sandbox ID and snapshot name required")
	}
	created, err := lifecycle.CreateSnapshot(ctx, sandboxID, snapshotName)
	if err != nil {
		return nil, err
	}
	if waitTimeout <= 0 {
		waitTimeout = 2 * time.Minute
	}
	waitCtx, cancel := context.WithTimeout(ctx, waitTimeout)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	current := created
	for {
		switch strings.ToLower(current.Status.State) {
		case "ready":
			if err := lifecycle.DeleteSandbox(waitCtx, sandboxID); err != nil {
				return nil, err
			}
			return current, nil
		case "failed":
			return nil, fmt.Errorf("opensandbox snapshot %s failed: %s", current.ID, firstNonEmpty(current.Status.Message, current.Status.Reason))
		}
		select {
		case <-waitCtx.Done():
			return nil, fmt.Errorf("wait for opensandbox snapshot %s: %w", current.ID, waitCtx.Err())
		case <-ticker.C:
		}
		current, err = lifecycle.GetSnapshot(waitCtx, current.ID)
		if err != nil {
			return nil, err
		}
	}
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
	priority := map[string]int{"running": 0, "paused": 1, "resuming": 2, "pending": 3, "creating": 4}
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
	case "running", "paused", "resuming", "pending", "creating":
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
