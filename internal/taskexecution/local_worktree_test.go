package taskexecution

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/repository"
)

func gitTaskTest(t *testing.T, dir string, args ...string) string {
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

func makeTaskRepo(t *testing.T) repository.Checkout {
	t.Helper()
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	gitTaskTest(t, root, "init", "-q")
	if err := os.WriteFile(filepath.Join(root, "app.txt"), []byte("main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTaskTest(t, root, "add", "app.txt")
	gitTaskTest(t, root, "commit", "-qm", "initial")
	return repository.Checkout{ID: "repo-a", RelativePath: ".", Root: root, IdentitySource: "lineage"}
}

func TestLocalWorktreeProviderIsolatesTwoTasksOnSameRepository(t *testing.T) {
	ctx := context.Background()
	repo := makeTaskRepo(t)
	provider := NewLocalWorktreeProvider(filepath.Join(t.TempDir(), "worktrees"))

	first, err := provider.Prepare(ctx, PrepareRequest{TaskID: "task-a", WorkspaceKey: "workspace", WorkspaceID: "ws", ProjectID: "project", Repositories: []repository.Checkout{repo}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := provider.Prepare(ctx, PrepareRequest{TaskID: "task-b", WorkspaceKey: "workspace", WorkspaceID: "ws", ProjectID: "project", Repositories: []repository.Checkout{repo}})
	if err != nil {
		t.Fatal(err)
	}
	firstPath := first.RepositoryBindings[0].LocalPath
	secondPath := second.RepositoryBindings[0].LocalPath
	if firstPath == secondPath {
		t.Fatal("different tasks must not share a writable worktree")
	}
	if err := os.WriteFile(filepath.Join(firstPath, "app.txt"), []byte("task-a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secondPath, "app.txt"), []byte("task-b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainData, err := os.ReadFile(filepath.Join(repo.Root, "app.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(mainData) != "main\n" {
		t.Fatalf("authoritative checkout was modified: %q", mainData)
	}
	firstData, _ := os.ReadFile(filepath.Join(firstPath, "app.txt"))
	secondData, _ := os.ReadFile(filepath.Join(secondPath, "app.txt"))
	if string(firstData) != "task-a\n" || string(secondData) != "task-b\n" {
		t.Fatalf("task worktrees were not isolated: first=%q second=%q", firstData, secondData)
	}
}

func TestLocalWorktreeProviderReusesTaskBinding(t *testing.T) {
	ctx := context.Background()
	repo := makeTaskRepo(t)
	provider := NewLocalWorktreeProvider(filepath.Join(t.TempDir(), "worktrees"))
	req := PrepareRequest{TaskID: "task-a", WorkspaceKey: "workspace", Repositories: []repository.Checkout{repo}}
	first, err := provider.Prepare(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := provider.Prepare(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if first.RepositoryBindings[0].LocalPath != second.RepositoryBindings[0].LocalPath {
		t.Fatal("task resume should reuse the existing local worktree")
	}
}

func TestLocalWorktreeProviderSnapshotsDirtyAuthoritativeRepository(t *testing.T) {
	ctx := context.Background()
	repo := makeTaskRepo(t)

	if err := os.WriteFile(filepath.Join(repo.Root, "app.txt"), []byte("staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTaskTest(t, repo.Root, "add", "app.txt")
	if err := os.WriteFile(filepath.Join(repo.Root, "app.txt"), []byte("dirty working\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo.Root, "notes.txt"), []byte("untracked\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	provider := NewLocalWorktreeProvider(filepath.Join(t.TempDir(), "worktrees"))
	bundle, err := provider.Prepare(ctx, PrepareRequest{TaskID: "task-dirty", WorkspaceKey: "workspace", Repositories: []repository.Checkout{repo}})
	if err != nil {
		t.Fatalf("dirty authoritative checkout should be snapshotted, got %v", err)
	}
	binding := bundle.RepositoryBindings[0]

	taskData, err := os.ReadFile(filepath.Join(binding.LocalPath, "app.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(taskData) != "dirty working\n" {
		t.Fatalf("tracked dirty state was not snapshotted: %q", taskData)
	}
	untrackedData, err := os.ReadFile(filepath.Join(binding.LocalPath, "notes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(untrackedData) != "untracked\n" {
		t.Fatalf("untracked state was not snapshotted: %q", untrackedData)
	}

	if staged := strings.TrimSpace(gitTaskTest(t, binding.LocalPath, "diff", "--cached", "--name-only")); staged != "" {
		t.Fatalf("source index state must not leak into isolated task index: %q", staged)
	}
	if dirty := gitTaskTest(t, binding.LocalPath, "status", "--porcelain"); !strings.Contains(dirty, "app.txt") || !strings.Contains(dirty, "notes.txt") {
		t.Fatalf("isolated task should start from the source working snapshot, status=%q", dirty)
	}

	if err := os.WriteFile(filepath.Join(binding.LocalPath, "app.txt"), []byte("task edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourceData, err := os.ReadFile(filepath.Join(repo.Root, "app.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(sourceData) != "dirty working\n" {
		t.Fatalf("task mutation leaked into authoritative checkout: %q", sourceData)
	}
}

func TestCleanupRemovesMergedCleanTaskWorktree(t *testing.T) {
	ctx := context.Background()
	repo := makeTaskRepo(t)
	provider := NewLocalWorktreeProvider(filepath.Join(t.TempDir(), "worktrees"))
	bundle, err := provider.Prepare(ctx, PrepareRequest{TaskID: "task-clean", WorkspaceKey: "workspace", Repositories: []repository.Checkout{repo}})
	if err != nil {
		t.Fatal(err)
	}
	binding := bundle.RepositoryBindings[0]
	if err := provider.Cleanup(ctx, repo, binding); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(binding.LocalPath); !os.IsNotExist(err) {
		t.Fatalf("clean merged worktree should be removed, stat err=%v", err)
	}
	if gitCommandOK(ctx, repo.Root, "show-ref", "--verify", "--quiet", "refs/heads/"+binding.BranchName) {
		t.Fatal("clean merged task branch should be deleted")
	}
}

func TestCleanupRefusesUncommittedTaskWorktree(t *testing.T) {
	ctx := context.Background()
	repo := makeTaskRepo(t)
	provider := NewLocalWorktreeProvider(filepath.Join(t.TempDir(), "worktrees"))
	bundle, err := provider.Prepare(ctx, PrepareRequest{TaskID: "task-a", WorkspaceKey: "workspace", Repositories: []repository.Checkout{repo}})
	if err != nil {
		t.Fatal(err)
	}
	binding := bundle.RepositoryBindings[0]
	if err := os.WriteFile(filepath.Join(binding.LocalPath, "app.txt"), []byte("dirty task\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := provider.Cleanup(ctx, repo, binding); err == nil {
		t.Fatal("cleanup must not delete a dirty task worktree")
	}
	if _, err := os.Stat(binding.LocalPath); err != nil {
		t.Fatalf("unsafe cleanup removed task worktree: %v", err)
	}
}
