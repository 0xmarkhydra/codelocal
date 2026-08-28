package opensandbox

import (
	"context"
	"errors"
	"testing"
	"time"
)

type snapshotLifecycleFake struct {
	*fakeLifecycle
	snapshots       []SnapshotInfo
	createdSnapshot *SnapshotInfo
	snapshotStates  []SnapshotInfo
	deletedSandbox  []string
	deletedSnapshot []string
	snapshotGets    int
}

func (f *snapshotLifecycleFake) CreateSnapshot(context.Context, string, string) (*SnapshotInfo, error) {
	if f.createdSnapshot == nil {
		return nil, errors.New("snapshot create not configured")
	}
	copy := *f.createdSnapshot
	return &copy, nil
}

func (f *snapshotLifecycleFake) GetSnapshot(context.Context, string) (*SnapshotInfo, error) {
	if len(f.snapshotStates) == 0 {
		return nil, errors.New("snapshot state not configured")
	}
	index := f.snapshotGets
	if index >= len(f.snapshotStates) {
		index = len(f.snapshotStates) - 1
	}
	f.snapshotGets++
	copy := f.snapshotStates[index]
	return &copy, nil
}

func (f *snapshotLifecycleFake) ListSnapshots(context.Context, SnapshotListOptions) (*ListSnapshotsResponse, error) {
	return &ListSnapshotsResponse{Items: append([]SnapshotInfo(nil), f.snapshots...)}, nil
}

func (f *snapshotLifecycleFake) DeleteSnapshot(_ context.Context, id string) error {
	f.deletedSnapshot = append(f.deletedSnapshot, id)
	return nil
}

func (f *snapshotLifecycleFake) DeleteSandbox(_ context.Context, id string) error {
	f.deletedSandbox = append(f.deletedSandbox, id)
	return nil
}

func TestManagerAcquireRestoresNewestReadySnapshot(t *testing.T) {
	now := time.Date(2026, 8, 29, 1, 0, 0, 0, time.UTC)
	base := &fakeLifecycle{
		created:     &SandboxInfo{ID: "sb-restored", Status: SandboxStatus{State: "Pending"}},
		getSequence: []SandboxInfo{{ID: "sb-restored", Status: SandboxStatus{State: "Running"}}},
	}
	fake := &snapshotLifecycleFake{
		fakeLifecycle: base,
		snapshots: []SnapshotInfo{
			{ID: "snap-old", Name: "workspace-snapshot", Status: SnapshotStatus{State: "Ready"}, CreatedAt: now.Add(-time.Hour)},
			{ID: "snap-new", Name: "workspace-snapshot", Status: SnapshotStatus{State: "Ready"}, CreatedAt: now},
		},
	}
	manager := NewManager(fake)
	manager.Now = func() time.Time { return now }
	spec := testAcquireSpec()
	spec.SnapshotName = "workspace-snapshot"
	result, err := manager.Acquire(context.Background(), spec)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if result.ID != "sb-restored" {
		t.Fatalf("sandbox = %q", result.ID)
	}
	if fake.createRequest.SnapshotID != "snap-new" {
		t.Fatalf("snapshotId = %q, want snap-new", fake.createRequest.SnapshotID)
	}
	if fake.createRequest.Image != nil {
		t.Fatalf("snapshot restore must not also send image: %#v", fake.createRequest.Image)
	}
}

func TestManagerSnapshotAndDeleteDeletesOnlyAfterReady(t *testing.T) {
	base := &fakeLifecycle{}
	fake := &snapshotLifecycleFake{
		fakeLifecycle: base,
		createdSnapshot: &SnapshotInfo{ID: "snap-1", SandboxID: "sb-1", Status: SnapshotStatus{State: "Creating"}},
		snapshotStates: []SnapshotInfo{{ID: "snap-1", SandboxID: "sb-1", Status: SnapshotStatus{State: "Ready"}}},
	}
	manager := NewManager(fake)
	result, err := manager.SnapshotAndDelete(context.Background(), "sb-1", "workspace-snapshot", 2*time.Second)
	if err != nil {
		t.Fatalf("SnapshotAndDelete() error = %v", err)
	}
	if result.Status.State != "Ready" || len(fake.deletedSandbox) != 1 || fake.deletedSandbox[0] != "sb-1" {
		t.Fatalf("result=%#v deleted=%#v", result, fake.deletedSandbox)
	}
}

func TestManagerSnapshotFailureRetainsSandbox(t *testing.T) {
	base := &fakeLifecycle{}
	fake := &snapshotLifecycleFake{
		fakeLifecycle: base,
		createdSnapshot: &SnapshotInfo{ID: "snap-fail", SandboxID: "sb-safe", Status: SnapshotStatus{State: "Creating"}},
		snapshotStates: []SnapshotInfo{{ID: "snap-fail", SandboxID: "sb-safe", Status: SnapshotStatus{State: "Failed"}, Message: "registry unavailable"}},
	}
	manager := NewManager(fake)
	if _, err := manager.SnapshotAndDelete(context.Background(), "sb-safe", "workspace-snapshot", 2*time.Second); err == nil {
		t.Fatal("SnapshotAndDelete() error = nil, want snapshot failure")
	}
	if len(fake.deletedSandbox) != 0 {
		t.Fatalf("sandbox deleted after failed snapshot: %#v", fake.deletedSandbox)
	}
}

func TestManagerAcceptsCurrentPendingState(t *testing.T) {
	fake := &fakeLifecycle{
		created:     &SandboxInfo{ID: "sb-pending", Status: SandboxStatus{State: "Pending"}},
		getSequence: []SandboxInfo{{ID: "sb-pending", Status: SandboxStatus{State: "Running"}}},
	}
	manager := NewManager(fake)
	if _, err := manager.Acquire(context.Background(), testAcquireSpec()); err != nil {
		t.Fatalf("Acquire(Pending) error = %v", err)
	}
}
