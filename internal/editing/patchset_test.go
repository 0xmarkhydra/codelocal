package editing

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/localfs"
)

func patchEngineForTest(t *testing.T) (*Engine, string) {
	t.Helper()
	root := t.TempDir()
	fs, err := localfs.New(root)
	if err != nil {
		t.Fatal(err)
	}
	return New(fs), root
}

func TestPatchSetRejectsStaleExistingFileWithoutOverwrite(t *testing.T) {
	engine, root := patchEngineForTest(t)
	path := filepath.Join(root, "app.txt")
	if err := os.WriteFile(path, []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hash := localfs.Hash([]byte("base\n"))
	replacement := "agent\n"
	set := PatchSet{ID: "patch-1", Provenance: PatchProvenance{TaskID: "task-1"}, Files: []PatchFile{{Path: "app.txt", ExpectedHash: hash, Content: &replacement}}}
	if err := os.WriteFile(path, []byte("user\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, _, err := engine.ApplyPatchSet(set)
	if !errors.Is(err, ErrPatchStaleBase) || result.State != PatchStale {
		t.Fatalf("expected stale base, state=%s err=%v", result.State, err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "user\n" {
		t.Fatalf("stale patch overwrote user work: %q", data)
	}
}

func TestPatchSetRejectsNewPathThatAppeared(t *testing.T) {
	engine, root := patchEngineForTest(t)
	content := "agent\n"
	set := PatchSet{ID: "patch-2", Provenance: PatchProvenance{TaskID: "task-2"}, Files: []PatchFile{{Path: "new.txt", ExpectedAbsent: true, Content: &content}}}
	if err := os.WriteFile(filepath.Join(root, "new.txt"), []byte("user\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, _, err := engine.ApplyPatchSet(set)
	if !errors.Is(err, ErrPatchConflict) || result.State != PatchConflicted {
		t.Fatalf("expected path conflict, state=%s err=%v", result.State, err)
	}
	data, _ := os.ReadFile(filepath.Join(root, "new.txt"))
	if string(data) != "user\n" {
		t.Fatalf("conflicting create overwrote user work: %q", data)
	}
}

func TestPatchSetAppliesGuardedExistingAndExclusiveCreate(t *testing.T) {
	engine, root := patchEngineForTest(t)
	if err := os.WriteFile(filepath.Join(root, "app.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	replacement, created := "agent\n", "created\n"
	set := PatchSet{ID: "patch-3", Provenance: PatchProvenance{TaskID: "task-3", AgentID: "agent-1"}, Files: []PatchFile{{Path: "app.txt", ExpectedHash: localfs.Hash([]byte("base\n")), Content: &replacement}, {Path: "new.txt", ExpectedAbsent: true, Content: &created}}}
	result, meta, err := engine.ApplyPatchSet(set)
	if err != nil || result.State != PatchApplied {
		t.Fatalf("apply failed state=%s meta=%+v err=%v", result.State, meta, err)
	}
	app, _ := os.ReadFile(filepath.Join(root, "app.txt"))
	newFile, _ := os.ReadFile(filepath.Join(root, "new.txt"))
	if string(app) != replacement || string(newFile) != created {
		t.Fatalf("unexpected files app=%q new=%q", app, newFile)
	}
}
