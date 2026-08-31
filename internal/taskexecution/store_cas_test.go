package taskexecution

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestPutCASRejectsStaleRevision(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state"))
	created, err := store.PutCAS(Bundle{TaskID: "task-a", WorkspaceKey: "workspace", Provider: ProviderLocalWorktree, State: StateReady}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if created.Revision != 1 {
		t.Fatalf("new bundle revision=%d want 1", created.Revision)
	}

	updated := created.Clone()
	updated.State = StateRunning
	updated, err = store.PutCAS(updated, created.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 || updated.State != StateRunning {
		t.Fatalf("unexpected CAS update: %+v", updated)
	}

	stale := created.Clone()
	stale.State = StateCancelled
	if _, err := store.PutCAS(stale, created.Revision); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale revision overwrote newer state: %v", err)
	}
	loaded, _, err := store.Get("workspace", "task-a")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != StateRunning || loaded.Revision != 2 {
		t.Fatalf("stale update changed durable state: %+v", loaded)
	}
}

func TestLeaseGenerationFencesExpiredOwner(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state"))
	bundle, err := store.Put(Bundle{TaskID: "task-a", WorkspaceKey: "workspace", Provider: ProviderLocalWorktree, State: StateReady})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Claim(bundle.WorkspaceKey, bundle.TaskID, "agent-a", time.Nanosecond)
	if err != nil {
		t.Fatal(err)
	}
	if first.Lease.Generation == 0 {
		t.Fatalf("claim did not receive fencing generation: %+v", first.Lease)
	}
	time.Sleep(time.Millisecond)
	second, err := store.Claim(bundle.WorkspaceKey, bundle.TaskID, "agent-b", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if second.Lease.Generation <= first.Lease.Generation {
		t.Fatalf("takeover did not advance generation: first=%+v second=%+v", first.Lease, second.Lease)
	}
	if _, err := store.ReleaseLease(bundle.WorkspaceKey, bundle.TaskID, "agent-a", first.Lease.Generation); !errors.Is(err, ErrLeaseHeld) {
		t.Fatalf("expired owner released replacement lease: %v", err)
	}
	if _, err := store.ReleaseLease(bundle.WorkspaceKey, bundle.TaskID, "agent-b", first.Lease.Generation); !errors.Is(err, ErrLeaseGeneration) {
		t.Fatalf("stale generation released current lease: %v", err)
	}
	released, err := store.ReleaseLease(bundle.WorkspaceKey, bundle.TaskID, "agent-b", second.Lease.Generation)
	if err != nil {
		t.Fatal(err)
	}
	if released.Lease.OwnerID != "" || released.Lease.Generation != second.Lease.Generation {
		t.Fatalf("release must preserve fencing generation: %+v", released.Lease)
	}
}

func TestLegacyPutStillAdvancesRevision(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state"))
	first, err := store.Put(Bundle{TaskID: "task-a", WorkspaceKey: "workspace", Provider: ProviderLocalWorktree, State: StateReady})
	if err != nil {
		t.Fatal(err)
	}
	second := first.Clone()
	second.State = StateReview
	second, err = store.Put(second)
	if err != nil {
		t.Fatal(err)
	}
	if second.Revision != first.Revision+1 {
		t.Fatalf("legacy put did not advance revision: first=%d second=%d", first.Revision, second.Revision)
	}
}
