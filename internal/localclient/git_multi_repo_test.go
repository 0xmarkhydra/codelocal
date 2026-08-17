package localclient

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitTestRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=CodeLocal Test", "GIT_AUTHOR_EMAIL=test@codelocal.invalid",
		"GIT_COMMITTER_NAME=CodeLocal Test", "GIT_COMMITTER_EMAIL=test@codelocal.invalid",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

func makeNestedGitRepo(t *testing.T, root, relative, filename string) string {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitTestRun(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, filename), []byte("initial\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTestRun(t, dir, "add", filename)
	gitTestRun(t, dir, "commit", "-qm", "initial")
	return dir
}

func newMultiRepoEngine(t *testing.T) (*Engine, string, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "BIDDI")
	web := makeNestedGitRepo(t, root, "web", "app.ts")
	auth := makeNestedGitRepo(t, root, "backend/auth", "login.go")
	t.Setenv("CODELOCAL_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	engine, err := New(root, "biddi-test", "BIDDI", "device::biddi-test", "device")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engine.Close)
	return engine, web, auth
}

func TestGitStatusAggregatesLogicalMultiRepoWorkspace(t *testing.T) {
	engine, web, _ := newMultiRepoEngine(t)
	if err := os.WriteFile(filepath.Join(web, "app.ts"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := engine.gitStatus("")
	if err != nil {
		t.Fatal(err)
	}
	if result["repositoryCount"] != 2 {
		t.Fatalf("expected two repositories, got %#v", result)
	}
	items, ok := result["repositories"].([]map[string]any)
	if !ok || len(items) != 2 {
		t.Fatalf("unexpected repository status payload: %#v", result)
	}
	dirty := 0
	for _, item := range items {
		if item["dirty"] == true {
			dirty++
		}
	}
	if dirty != 1 {
		t.Fatalf("expected exactly one dirty repository: %#v", items)
	}
}

func TestGitDiffRoutesWorkspacePathToOwningRepository(t *testing.T) {
	engine, _, auth := newMultiRepoEngine(t)
	if err := os.WriteFile(filepath.Join(auth, "login.go"), []byte("changed auth\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := engine.gitDiff("", "backend/auth/login.go", false)
	if err != nil {
		t.Fatal(err)
	}
	if result["repositoryPath"] != "backend/auth" {
		t.Fatalf("wrong repository routing: %#v", result)
	}
	if !strings.Contains(asString(result["diff"]), "changed auth") {
		t.Fatalf("expected auth diff, got %#v", result)
	}
}

func TestGitRepositorySelectorRequiredForAmbiguousRepoWideRead(t *testing.T) {
	engine, _, _ := newMultiRepoEngine(t)
	if _, _, err := engine.gitTarget("", ""); err == nil {
		t.Fatal("repo-wide operation in multi-repo workspace must require repository selector")
	}
	if repo, _, err := engine.gitTarget("web", ""); err != nil || repo.RelativePath != "web" {
		t.Fatalf("repository selector did not resolve: repo=%#v err=%v", repo, err)
	}
}
