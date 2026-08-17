package taskexecution

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestStorePersistsBundleAndEnforcesLease(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state"))
	bundle, err := store.Put(Bundle{
		TaskID: "task-a", WorkspaceKey: "device::workspace", WorkspaceID: "workspace",
		ProjectID: "project", Provider: ProviderLocalWorktree, State: StateReady,
	})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.ID == "" || bundle.SchemaVersion != 1 {
		t.Fatalf("bundle identity was not initialized: %#v", bundle)
	}
	loaded, ok, err := store.Get(bundle.WorkspaceKey, bundle.TaskID)
	if err != nil || !ok || loaded.ID != bundle.ID {
		t.Fatalf("bundle did not persist: loaded=%#v ok=%v err=%v", loaded, ok, err)
	}
	claimed, err := store.Claim(bundle.WorkspaceKey, bundle.TaskID, "agent-a", time.Minute)
	if err != nil || claimed.Lease.OwnerID != "agent-a" {
		t.Fatalf("claim failed: %#v err=%v", claimed, err)
	}
	if _, err := store.Claim(bundle.WorkspaceKey, bundle.TaskID, "agent-b", time.Minute); !errors.Is(err, ErrLeaseHeld) {
		t.Fatalf("second owner must be rejected, got %v", err)
	}
	if _, err := store.Release(bundle.WorkspaceKey, bundle.TaskID, "agent-b"); !errors.Is(err, ErrLeaseHeld) {
		t.Fatalf("wrong owner release must be rejected, got %v", err)
	}
	if released, err := store.Release(bundle.WorkspaceKey, bundle.TaskID, "agent-a"); err != nil || released.Lease.OwnerID != "" {
		t.Fatalf("owner release failed: %#v err=%v", released, err)
	}
}

func TestExpiredLeaseCanBeTakenOver(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "state"))
	bundle, err := store.Put(Bundle{TaskID: "task-a", WorkspaceKey: "workspace", Provider: ProviderLocalWorktree, State: StateReady})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(bundle.WorkspaceKey, bundle.TaskID, "agent-a", time.Nanosecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	if claimed, err := store.Claim(bundle.WorkspaceKey, bundle.TaskID, "agent-b", time.Minute); err != nil || claimed.Lease.OwnerID != "agent-b" {
		t.Fatalf("expired lease was not recoverable: %#v err=%v", claimed, err)
	}
}
