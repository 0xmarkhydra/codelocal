package repository

import (
	"path/filepath"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/projectidentity"
)

func TestResolvePathChoosesDeepestRepository(t *testing.T) {
	root := t.TempDir()
	registry := FromSnapshot(root, projectidentity.Snapshot{Repositories: []projectidentity.Repository{
		{ID: "root", RelativePath: ".", IdentitySource: "remote"},
		{ID: "auth", RelativePath: "backend/auth", IdentitySource: "remote"},
	}})

	repo, path, err := registry.ResolvePath("backend/auth/internal/login.go")
	if err != nil {
		t.Fatal(err)
	}
	if repo.ID != "auth" || path != "internal/login.go" {
		t.Fatalf("unexpected resolution: repo=%#v path=%q", repo, path)
	}
	if repo.Root != filepath.Join(root, "backend", "auth") {
		t.Fatalf("unexpected repo root: %q", repo.Root)
	}
}

func TestResolveSelectorRequiresRepositoryForMultiRepo(t *testing.T) {
	registry := FromSnapshot(t.TempDir(), projectidentity.Snapshot{Repositories: []projectidentity.Repository{
		{ID: "web-id", RelativePath: "web", IdentitySource: "remote"},
		{ID: "api-id", RelativePath: "backend/api", IdentitySource: "remote"},
	}})
	if _, err := registry.ResolveSelector(""); err == nil {
		t.Fatal("multi-repo selector must be explicit")
	}
	if repo, err := registry.ResolveSelector("web"); err != nil || repo.ID != "web-id" {
		t.Fatalf("relative repository selector failed: repo=%#v err=%v", repo, err)
	}
	if repo, err := registry.ResolveSelector("api-id"); err != nil || repo.RelativePath != "backend/api" {
		t.Fatalf("id repository selector failed: repo=%#v err=%v", repo, err)
	}
}

func TestResolvePathRejectsWorkspaceEscape(t *testing.T) {
	registry := FromSnapshot(t.TempDir(), projectidentity.Snapshot{Repositories: []projectidentity.Repository{
		{ID: "repo", RelativePath: ".", IdentitySource: "remote"},
	}})
	if _, _, err := registry.ResolvePath("../outside"); err == nil {
		t.Fatal("workspace escape must be rejected")
	}
}
