package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGrantManagedPreservesControlPlaneWorkspaceID(t *testing.T) {
	stateDir := t.TempDir()
	workspaceDir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODELOCAL_STATE_DIR", stateDir)

	registry := New()
	granted, err := registry.GrantManaged("project-deadbeef01", workspaceDir, "Cloud Project")
	if err != nil {
		t.Fatalf("GrantManaged() error = %v", err)
	}
	if granted.WorkspaceID != "project-deadbeef01" {
		t.Fatalf("workspaceId = %q", granted.WorkspaceID)
	}
	if granted.WorkspaceName != "Cloud Project" {
		t.Fatalf("workspaceName = %q", granted.WorkspaceName)
	}

	items, err := registry.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].WorkspaceID != "project-deadbeef01" {
		t.Fatalf("items = %#v", items)
	}
}

func TestGrantManagedRejectsUnsafeIDAndRoot(t *testing.T) {
	t.Setenv("CODELOCAL_STATE_DIR", t.TempDir())
	registry := New()
	if _, err := registry.GrantManaged("bad/id", t.TempDir(), "bad"); err == nil {
		t.Fatal("invalid managed workspace ID accepted")
	}
	if _, err := registry.GrantManaged("root-workspace", string(filepath.Separator), "root"); err == nil {
		t.Fatal("filesystem root accepted")
	}
}
