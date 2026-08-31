package editing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/localfs"
)

func TestSavepointRewindPlanPreservesDisjointUserChanges(t *testing.T) {
	workspace := t.TempDir()
	stateRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "auth.go"), []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := localfs.New(workspace)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewSavepointStore(stateRoot, fs)
	if err != nil {
		t.Fatal(err)
	}
	captured, err := store.Capture(SavepointCaptureRequest{
		ID: "sp-1", WorkspaceKey: "workspace", Paths: []string{"generated.go", "auth.go"},
		Provenance: SavepointProvenance{TaskID: "task", AgentID: "agent", ActivationID: "activation:agent:1", TraceID: "trace"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if captured.State != SavepointCaptured || captured.Revision != 1 || len(captured.Entries) != 2 {
		t.Fatalf("captured = %#v", captured)
	}
	authAfter := []byte("alpha\nbeta\ngamma-agent\n")
	generatedAfter := []byte("package generated\n")
	if err := os.WriteFile(filepath.Join(workspace, "auth.go"), authAfter, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "generated.go"), generatedAfter, 0o644); err != nil {
		t.Fatal(err)
	}
	sealed, err := store.Seal("workspace", captured.ID, captured.Revision, map[string]string{
		"auth.go":      localfs.Hash(authAfter),
		"generated.go": localfs.Hash(generatedAfter),
	})
	if err != nil {
		t.Fatal(err)
	}
	if sealed.State != SavepointSealed || sealed.Revision != 2 {
		t.Fatalf("sealed = %#v", sealed)
	}
	// User keeps working after the agent finished; this edit must survive rewind.
	if err := os.WriteFile(filepath.Join(workspace, "auth.go"), []byte("alpha-user\nbeta\ngamma-agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := store.PlanRewind(context.Background(), "workspace", sealed.ID, sealed.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.CanApply || len(plan.Conflicts) != 0 || len(plan.Patch.Files) != 1 || len(plan.Deletes) != 1 {
		t.Fatalf("rewind plan = %#v", plan)
	}
	if plan.Deletes[0].Path != "generated.go" || plan.Deletes[0].ExpectedHash == "" {
		t.Fatalf("guarded delete = %#v", plan.Deletes)
	}
	if plan.Patch.Files[0].Content == nil {
		t.Fatal("rewind patch content missing")
	}
	content := *plan.Patch.Files[0].Content
	if !strings.Contains(content, "alpha-user") || !strings.Contains(content, "gamma\n") || strings.Contains(content, "gamma-agent") {
		t.Fatalf("rewind did not preserve user edit: %q", content)
	}
	engine := New(fs)
	if _, err := engine.ValidatePatchSet(plan.Patch); err != nil {
		t.Fatalf("fresh rewind patch should validate: %v", err)
	}
	// A later user edit after planning must stale the rewind patch.
	if err := os.WriteFile(filepath.Join(workspace, "auth.go"), []byte("alpha-user-2\nbeta\ngamma-agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.ValidatePatchSet(plan.Patch); !errors.Is(err, ErrPatchStaleBase) {
		t.Fatalf("expected stale rewind patch, got %v", err)
	}
}

func TestSavepointSealRejectsUserRaceBeforeCheckpoint(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "file.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, _ := localfs.New(workspace)
	store, _ := NewSavepointStore(t.TempDir(), fs)
	captured, err := store.Capture(SavepointCaptureRequest{ID: "race", WorkspaceKey: "w", Paths: []string{"file.txt"}, Provenance: SavepointProvenance{TaskID: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	agentAfter := []byte("agent-after\n")
	if err := os.WriteFile(filepath.Join(workspace, "file.txt"), agentAfter, 0o644); err != nil {
		t.Fatal(err)
	}
	// User changes the file before Seal can record the agent outcome.
	if err := os.WriteFile(filepath.Join(workspace, "file.txt"), []byte("user-raced\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Seal("w", "race", captured.Revision, map[string]string{"file.txt": localfs.Hash(agentAfter)}); !errors.Is(err, ErrSavepointStale) {
		t.Fatalf("expected stale seal, got %v", err)
	}
	loaded, found, err := store.Load("w", "race")
	if err != nil || !found || loaded.State != SavepointCaptured || loaded.Revision != captured.Revision {
		t.Fatalf("failed seal mutated manifest: loaded=%#v found=%v err=%v", loaded, found, err)
	}
}

func TestSavepointRewindDetectsOverlappingUserEdit(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "file.txt"), []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, _ := localfs.New(workspace)
	store, _ := NewSavepointStore(t.TempDir(), fs)
	captured, err := store.Capture(SavepointCaptureRequest{ID: "sp", WorkspaceKey: "w", Paths: []string{"file.txt"}, Provenance: SavepointProvenance{TaskID: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	agentAfter := []byte("alpha\nbeta-agent\ngamma\n")
	_ = os.WriteFile(filepath.Join(workspace, "file.txt"), agentAfter, 0o644)
	sealed, err := store.Seal("w", "sp", captured.Revision, map[string]string{"file.txt": localfs.Hash(agentAfter)})
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(workspace, "file.txt"), []byte("alpha\nbeta-user\ngamma\n"), 0o644)
	plan, err := store.PlanRewind(context.Background(), "w", "sp", sealed.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if plan.CanApply || len(plan.Conflicts) != 1 || plan.Conflicts[0].Reason != "overlapping_user_change" || len(plan.Patch.Files) != 0 {
		t.Fatalf("overlap should conflict: %#v", plan)
	}
}

func TestSavepointRewindRestoresAgentDeletedFileAsExclusiveCreate(t *testing.T) {
	workspace := t.TempDir()
	original := []byte("important user code\n")
	if err := os.WriteFile(filepath.Join(workspace, "keep.go"), original, 0o640); err != nil {
		t.Fatal(err)
	}
	fs, _ := localfs.New(workspace)
	store, _ := NewSavepointStore(t.TempDir(), fs)
	captured, err := store.Capture(SavepointCaptureRequest{ID: "delete", WorkspaceKey: "w", Paths: []string{"keep.go"}, Provenance: SavepointProvenance{TaskID: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(workspace, "keep.go")); err != nil {
		t.Fatal(err)
	}
	sealed, err := store.Seal("w", "delete", captured.Revision, map[string]string{"keep.go": ""})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := store.PlanRewind(context.Background(), "w", "delete", sealed.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.CanApply || len(plan.Patch.Files) != 1 || !plan.Patch.Files[0].ExpectedAbsent || plan.Patch.Files[0].Content == nil || *plan.Patch.Files[0].Content != string(original) {
		t.Fatalf("deleted-file rewind = %#v", plan)
	}
}

func TestSavepointAgentCreatedFileChangedByUserBecomesConflict(t *testing.T) {
	workspace := t.TempDir()
	fs, _ := localfs.New(workspace)
	store, _ := NewSavepointStore(t.TempDir(), fs)
	captured, err := store.Capture(SavepointCaptureRequest{ID: "created", WorkspaceKey: "w", Paths: []string{"new.go"}, Provenance: SavepointProvenance{TaskID: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	agentAfter := []byte("agent content\n")
	_ = os.WriteFile(filepath.Join(workspace, "new.go"), agentAfter, 0o644)
	sealed, err := store.Seal("w", "created", captured.Revision, map[string]string{"new.go": localfs.Hash(agentAfter)})
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(workspace, "new.go"), []byte("user changed agent-created file\n"), 0o644)
	plan, err := store.PlanRewind(context.Background(), "w", "created", sealed.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if plan.CanApply || len(plan.Deletes) != 0 || len(plan.Conflicts) != 1 || plan.Conflicts[0].Reason != "agent_created_file_changed_by_user" {
		t.Fatalf("created-file user edit was not fenced: %#v", plan)
	}
}

func TestSavepointManifestDoesNotContainRawSourceAndBlobCorruptionFails(t *testing.T) {
	workspace := t.TempDir()
	secretText := "ordinary source payload unique-marker-123\n"
	if err := os.WriteFile(filepath.Join(workspace, "source.go"), []byte(secretText), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, _ := localfs.New(workspace)
	store, _ := NewSavepointStore(t.TempDir(), fs)
	captured, err := store.Capture(SavepointCaptureRequest{ID: "private", WorkspaceKey: "w", Paths: []string{"source.go"}, Provenance: SavepointProvenance{TaskID: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(store.manifestPath("w", captured.ID))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(manifest), "unique-marker-123") {
		t.Fatalf("manifest leaked raw source: %s", manifest)
	}
	blobHash := captured.Entries[0].BeforeBlob
	blobPath := filepath.Join(store.workspaceDir("w"), "blobs", blobHash+".blob")
	if err := os.WriteFile(blobPath, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.loadBlob("w", blobHash); !errors.Is(err, ErrSavepointBlob) {
		t.Fatalf("expected corrupt blob rejection, got %v", err)
	}
}

func TestSavepointBlocksSensitivePath(t *testing.T) {
	workspace := t.TempDir()
	fs, _ := localfs.New(workspace)
	store, _ := NewSavepointStore(t.TempDir(), fs)
	_, err := store.Capture(SavepointCaptureRequest{ID: "secret", WorkspaceKey: "w", Paths: []string{".env"}, Provenance: SavepointProvenance{TaskID: "task"}})
	if !errors.Is(err, ErrInvalidSavepoint) {
		t.Fatalf("expected sensitive path rejection, got %v", err)
	}
}
