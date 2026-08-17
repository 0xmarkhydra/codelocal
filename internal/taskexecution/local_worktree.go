package taskexecution

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/repository"
	codelocalstate "github.com/0xmarkhydra/codelocal/internal/state"
)

var ErrDirtyRepository = errors.New("repository has uncommitted changes; local task worktree currently requires a clean source checkout")

type PrepareRequest struct {
	TaskID       string
	ProjectID    string
	WorkspaceID  string
	WorkspaceKey string
	Repositories []repository.Checkout
}

type LocalWorktreeProvider struct{ baseDir string }

type createdWorktree struct {
	repo repository.Checkout
	path string
}

func NewLocalWorktreeProvider(baseDir string) *LocalWorktreeProvider {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		baseDir = filepath.Join(codelocalstate.Dir(), "worktrees")
	}
	return &LocalWorktreeProvider{baseDir: baseDir}
}

func gitCommand(ctx context.Context, root string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(), "PAGER=cat", "GIT_PAGER=cat", "CI=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func gitCommandOK(ctx context.Context, root string, args ...string) bool {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(), "PAGER=cat", "GIT_PAGER=cat", "CI=1")
	return cmd.Run() == nil
}

func taskBranch(taskID string) string { return "codelocal/task/" + digestKey("branch", taskID)[:12] }

func validatePrepareRequest(req PrepareRequest) error {
	if strings.TrimSpace(req.TaskID) == "" || strings.TrimSpace(req.WorkspaceKey) == "" {
		return errors.New("taskId and workspaceKey are required")
	}
	if len(req.Repositories) == 0 {
		return errors.New("at least one repository is required")
	}
	return nil
}

func newBundle(req PrepareRequest) Bundle {
	return Bundle{SchemaVersion: 1, ID: "exec_" + digestKey(req.WorkspaceKey, req.TaskID), TaskID: req.TaskID,
		ProjectID: req.ProjectID, WorkspaceID: req.WorkspaceID, WorkspaceKey: req.WorkspaceKey,
		Provider: ProviderLocalWorktree, State: StatePreparing}
}

func (p *LocalWorktreeProvider) worktreePath(workspaceKey, taskID string, repo repository.Checkout) string {
	return filepath.Join(p.baseDir, digestKey("workspace", workspaceKey), digestKey("task", taskID),
		digestKey("repository", repo.ID, repo.RelativePath))
}

func sourceRevision(ctx context.Context, repo repository.Checkout) (string, error) {
	status, err := gitCommand(ctx, repo.Root, "status", "--porcelain")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(status) != "" {
		return "", fmt.Errorf("%w: %s", ErrDirtyRepository, repo.RelativePath)
	}
	return gitCommand(ctx, repo.Root, "rev-parse", "HEAD")
}

func validExistingWorktree(ctx context.Context, path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() || !gitCommandOK(ctx, path, "rev-parse", "--is-inside-work-tree") {
		return false, fmt.Errorf("existing task execution path is not a valid Git worktree: %s", path)
	}
	return true, nil
}

func worktreeAddArgs(ctx context.Context, repo repository.Checkout, path, branch, head string) []string {
	if gitCommandOK(ctx, repo.Root, "show-ref", "--verify", "--quiet", "refs/heads/"+branch) {
		return []string{"worktree", "add", path, branch}
	}
	return []string{"worktree", "add", "-b", branch, path, head}
}

func (p *LocalWorktreeProvider) ensureWorktree(ctx context.Context, req PrepareRequest, repo repository.Checkout, branch, head string) (string, bool, error) {
	path := p.worktreePath(req.WorkspaceKey, req.TaskID, repo)
	if err := codelocalstate.EnsurePrivateDir(filepath.Dir(path)); err != nil {
		return "", false, err
	}
	if reused, err := validExistingWorktree(ctx, path); err != nil || reused {
		return path, false, err
	}
	_, err := gitCommand(ctx, repo.Root, worktreeAddArgs(ctx, repo, path, branch, head)...)
	return path, err == nil, err
}

func repositoryBinding(req PrepareRequest, repo repository.Checkout, branch, head, path string) RepositoryBinding {
	return RepositoryBinding{RepositoryID: repo.ID, RepositoryPath: repo.RelativePath, SourceRevision: head,
		BranchName: branch, BindingID: "binding_" + digestKey(req.TaskID, repo.ID, repo.RelativePath), LocalPath: path}
}

func rollbackWorktrees(created []createdWorktree) {
	for i := len(created) - 1; i >= 0; i-- {
		_, _ = gitCommand(context.Background(), created[i].repo.Root, "worktree", "remove", "--force", created[i].path)
	}
}

func (p *LocalWorktreeProvider) prepareRepository(ctx context.Context, req PrepareRequest, repo repository.Checkout, branch string) (RepositoryBinding, *createdWorktree, error) {
	head, err := sourceRevision(ctx, repo)
	if err != nil {
		return RepositoryBinding{}, nil, err
	}
	path, created, err := p.ensureWorktree(ctx, req, repo, branch, head)
	if err != nil {
		return RepositoryBinding{}, nil, err
	}
	if created {
		return repositoryBinding(req, repo, branch, head, path), &createdWorktree{repo: repo, path: path}, nil
	}
	return repositoryBinding(req, repo, branch, head, path), nil, nil
}

func (p *LocalWorktreeProvider) Prepare(ctx context.Context, req PrepareRequest) (Bundle, error) {
	if err := validatePrepareRequest(req); err != nil {
		return Bundle{}, err
	}
	if err := codelocalstate.EnsurePrivateDir(p.baseDir); err != nil {
		return Bundle{}, err
	}
	return p.prepareRepositories(ctx, req)
}

func (p *LocalWorktreeProvider) prepareRepositories(ctx context.Context, req PrepareRequest) (Bundle, error) {
	bundle, branch, created := newBundle(req), taskBranch(req.TaskID), []createdWorktree{}
	for _, repo := range req.Repositories {
		binding, added, err := p.prepareRepository(ctx, req, repo, branch)
		if err != nil {
			rollbackWorktrees(created)
			return Bundle{}, err
		}
		bundle.RepositoryBindings = append(bundle.RepositoryBindings, binding)
		if added != nil {
			created = append(created, *added)
		}
	}
	bundle.State = StateReady
	return bundle, nil
}

func (p *LocalWorktreeProvider) CanCleanup(ctx context.Context, repo repository.Checkout, binding RepositoryBinding) (bool, string, error) {
	status, err := gitCommand(ctx, binding.LocalPath, "status", "--porcelain")
	if err != nil {
		return false, "", err
	}
	if strings.TrimSpace(status) != "" {
		return false, "worktree has uncommitted changes", nil
	}
	if binding.BranchName == "" {
		return false, "worktree branch is unknown", nil
	}
	if !gitCommandOK(ctx, repo.Root, "merge-base", "--is-ancestor", binding.BranchName, "HEAD") {
		return false, "task branch contains commits not merged into the authoritative checkout", nil
	}
	return true, "safe to remove", nil
}

func (p *LocalWorktreeProvider) Cleanup(ctx context.Context, repo repository.Checkout, binding RepositoryBinding) error {
	ok, reason, err := p.CanCleanup(ctx, repo, binding)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New(reason)
	}
	if _, err := gitCommand(ctx, repo.Root, "worktree", "remove", binding.LocalPath); err != nil {
		return err
	}
	if binding.BranchName == "" {
		return nil
	}
	_, err = gitCommand(ctx, repo.Root, "branch", "-d", binding.BranchName)
	return err
}
