package projectbrain

import (
	"path/filepath"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/projectidentity"
)

func TestSyncStateSurvivesRuntimeRestartAndKeepsTombstoneBase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project-brain", "sync-state.json")
	first := NewSyncStateStoreAt(path)
	source := deltaSource("AGENTS.md", "hash-a")
	manifest := NewManifest([]Source{source})
	key := SourceIdentityKey(source)
	want := WorkspaceSyncState{
		WorkspaceID: "workspace-a",
		ProjectIdentity: projectidentity.Snapshot{
			SuggestedName: "Project A",
			Repositories:  []projectidentity.Repository{{ID: "repo-a", RelativePath: ".", IdentitySource: "remote"}},
		},
		Manifest:      manifest,
		BaseRevisions: map[string]string{key: "rev-tombstone"},
		SyncedRoot:    manifest.RootHash,
		UpdatedAt:     123,
	}
	if err := first.Put(want); err != nil {
		t.Fatal(err)
	}

	// A new store instance simulates a fresh CodeLocal runtime process.
	second := NewSyncStateStoreAt(path)
	got, ok, err := second.Workspace("workspace-a")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got.SyncedRoot != want.SyncedRoot || got.BaseRevisions[key] != "rev-tombstone" || got.Manifest.RootHash != manifest.RootHash || len(got.ProjectIdentity.Repositories) != 1 || got.ProjectIdentity.Repositories[0].ID != "repo-a" {
		t.Fatalf("durable sync state was not restored: %#v", got)
	}
}

func TestSyncStatePrunesRevokedWorkspaceWithoutTouchingAuthorizedState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project-brain", "sync-state.json")
	store := NewSyncStateStoreAt(path)
	if err := store.Put(WorkspaceSyncState{WorkspaceID: "keep", BaseRevisions: map[string]string{"a": "1"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(WorkspaceSyncState{WorkspaceID: "remove", BaseRevisions: map[string]string{"b": "2"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Prune([]string{"keep"}); err != nil {
		t.Fatal(err)
	}
	keep, ok, err := store.Workspace("keep")
	if err != nil || !ok || keep.BaseRevisions["a"] != "1" {
		t.Fatalf("authorized state changed: %#v ok=%v err=%v", keep, ok, err)
	}
	if _, ok, err := store.Workspace("remove"); err != nil || ok {
		t.Fatalf("revoked workspace state was retained: ok=%v err=%v", ok, err)
	}
}
