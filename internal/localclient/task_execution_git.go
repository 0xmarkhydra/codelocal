package localclient

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/repository"
	"github.com/0xmarkhydra/codelocal/internal/taskexecution"
)

type taskGitTarget struct {
	Repository repository.Checkout
	Root       string
	RepoPath   string
	Binding    taskexecution.RepositoryBinding
	Active     bool
}

func (e *Engine) taskGitTarget(ctx context.Context, args map[string]any, opts HandleOptions, selector, path string, prepare bool) (taskGitTarget, error) {
	repo, repoPath, err := e.gitTarget(selector, path)
	if err != nil {
		return taskGitTarget{}, err
	}
	taskID := strings.TrimSpace(asString(args[privateTaskExecutionID]))
	if taskID == "" || e.TaskExecutions == nil {
		return taskGitTarget{Repository: repo, Root: repo.Root, RepoPath: repoPath}, nil
	}
	bundle, ok, err := e.TaskExecutions.Store.Get(e.WorkspaceKey, taskID)
	if err != nil {
		return taskGitTarget{}, err
	}
	if ok {
		if binding, bound := taskBindingForRepository(bundle, repo); bound {
			return taskGitTarget{Repository: repo, Root: binding.LocalPath, RepoPath: repoPath, Binding: binding, Active: true}, nil
		}
	}
	if !prepare {
		return taskGitTarget{Repository: repo, Root: repo.Root, RepoPath: repoPath}, nil
	}
	workspacePath := repo.RelativePath
	if path != "" {
		workspacePath = path
	}
	target, err := e.taskExecutionTargetForPath(ctx, args, opts, workspacePath, true)
	if err != nil {
		return taskGitTarget{}, err
	}
	if !target.Active {
		return taskGitTarget{}, errors.New("task Git mutation requires an isolated repository binding")
	}
	return taskGitTarget{Repository: repo, Root: target.Binding.LocalPath, RepoPath: repoPath, Binding: target.Binding, Active: true}, nil
}

func taskGitMetadata(args map[string]any, repo repository.Checkout) map[string]any {
	return map[string]any{
		"taskId":         strings.TrimSpace(asString(args[privateTaskExecutionID])),
		"provider":       string(taskexecution.ProviderLocalWorktree),
		"repositoryId":   repo.ID,
		"repositoryPath": repo.RelativePath,
	}
}

func attachTaskGitMetadata(result map[string]any, args map[string]any, repo repository.Checkout, active bool) map[string]any {
	if result == nil {
		result = map[string]any{}
	}
	if active {
		result["taskExecution"] = taskGitMetadata(args, repo)
	}
	result["repositoryId"] = repo.ID
	result["repositoryPath"] = repo.RelativePath
	return result
}

func taskGitStatusItem(args map[string]any, repo repository.Checkout, root string) (map[string]any, error) {
	result, err := runGit(root, "status", "--short", "--branch")
	if err != nil {
		return nil, err
	}
	output := asString(result["output"])
	return map[string]any{
		"repositoryId": repo.ID, "repositoryPath": repo.RelativePath,
		"dirty": gitStatusDirty(output), "output": output,
		"taskExecution": taskGitMetadata(args, repo),
	}, nil
}

func (e *Engine) taskGitStatus(args map[string]any) (map[string]any, error) {
	bundle, ok, err := e.taskExecutionBundle(args)
	if err != nil {
		return nil, err
	}
	selector := asString(args["repository"])
	if !ok {
		return e.gitStatus(selector)
	}
	if selector != "" {
		repo, err := e.Repositories.ResolveSelector(selector)
		if err != nil {
			return nil, err
		}
		binding, bound := taskBindingForRepository(bundle, repo)
		if !bound {
			return e.gitStatus(selector)
		}
		result, err := runGit(binding.LocalPath, "status", "--short", "--branch")
		return attachTaskGitMetadata(result, args, repo, true), err
	}
	items := []map[string]any{}
	var combined strings.Builder
	bindings := append([]taskexecution.RepositoryBinding(nil), bundle.RepositoryBindings...)
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].RepositoryPath < bindings[j].RepositoryPath })
	for _, binding := range bindings {
		repo, err := e.Repositories.ResolveSelector(binding.RepositoryID)
		if err != nil {
			continue
		}
		item, err := taskGitStatusItem(args, repo, binding.LocalPath)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
		combined.WriteString("[" + repo.RelativePath + "]\n")
		combined.WriteString(asString(item["output"]))
		if !strings.HasSuffix(asString(item["output"]), "\n") {
			combined.WriteByte('\n')
		}
	}
	return map[string]any{
		"repositoryCount": len(items), "repositories": items,
		"stdout": combined.String(), "stderr": "", "output": combined.String(), "exitCode": 0,
		"taskExecution": map[string]any{"taskId": bundle.TaskID, "provider": string(bundle.Provider), "repositoryCount": len(items)},
	}, nil
}

func (e *Engine) taskGitDiff(ctx context.Context, args map[string]any, opts HandleOptions) (map[string]any, error) {
	selector, path := asString(args["repository"]), asString(args["path"])
	cached := asBool(args["cached"], false)
	bundle, ok, err := e.taskExecutionBundle(args)
	if err != nil {
		return nil, err
	}
	if !ok || path != "" || selector != "" {
		target, err := e.taskGitTarget(ctx, args, opts, selector, path, false)
		if err != nil {
			return nil, err
		}
		gitArgs := []string{"diff", "--no-ext-diff", "--unified=3"}
		if cached {
			gitArgs = append(gitArgs, "--cached")
		}
		if target.RepoPath != "" {
			gitArgs = append(gitArgs, "--", target.RepoPath)
		}
		result, err := runGit(target.Root, gitArgs...)
		if result != nil {
			result["diff"] = result["output"]
		}
		return attachTaskGitMetadata(result, args, target.Repository, target.Active), err
	}
	items := []map[string]any{}
	var combined strings.Builder
	for _, binding := range bundle.RepositoryBindings {
		repo, resolveErr := e.Repositories.ResolveSelector(binding.RepositoryID)
		if resolveErr != nil {
			continue
		}
		gitArgs := []string{"diff", "--no-ext-diff", "--unified=3"}
		if cached {
			gitArgs = append(gitArgs, "--cached")
		}
		result, runErr := runGit(binding.LocalPath, gitArgs...)
		if runErr != nil {
			return nil, runErr
		}
		diff := asString(result["output"])
		if strings.TrimSpace(diff) == "" {
			continue
		}
		items = append(items, map[string]any{"repositoryId": repo.ID, "repositoryPath": repo.RelativePath, "diff": diff, "taskExecution": taskGitMetadata(args, repo)})
		combined.WriteString("[" + repo.RelativePath + "]\n")
		combined.WriteString(diff)
		if !strings.HasSuffix(diff, "\n") {
			combined.WriteByte('\n')
		}
	}
	return map[string]any{
		"repositoryCount": len(bundle.RepositoryBindings), "changedRepositories": len(items), "repositories": items,
		"diff": combined.String(), "stdout": combined.String(), "stderr": "", "output": combined.String(), "exitCode": 0,
		"taskExecution": map[string]any{"taskId": bundle.TaskID, "provider": string(bundle.Provider), "repositoryCount": len(bundle.RepositoryBindings)},
	}, nil
}

func (e *Engine) taskGitRead(ctx context.Context, operation string, args map[string]any, opts HandleOptions) (map[string]any, error) {
	selector, path := asString(args["repository"]), asString(args["path"])
	target, err := e.taskGitTarget(ctx, args, opts, selector, path, false)
	if err != nil {
		return nil, err
	}
	var gitArgs []string
	switch operation {
	case "log":
		n := asInt(args["limit"], 20)
		if n < 1 {
			n = 1
		}
		if n > 100 {
			n = 100
		}
		gitArgs = []string{"log", "-" + strconv.Itoa(n), "--date=iso", "--pretty=format:%h%x09%ad%x09%an%x09%s"}
		if target.RepoPath != "" {
			gitArgs = append(gitArgs, "--", target.RepoPath)
		}
	case "show":
		gitArgs = []string{"show", "--stat", "--oneline", "--decorate", defaultString(asString(args["ref"]), "HEAD")}
	case "blame":
		if path == "" {
			return nil, errors.New("git blame requires path")
		}
		gitArgs = []string{"blame", "--line-porcelain"}
		if start := asInt(args["startLine"], 0); start > 0 {
			end := asInt(args["endLine"], start)
			gitArgs = append(gitArgs, "-L", fmt.Sprintf("%d,%d", start, end))
		}
		gitArgs = append(gitArgs, "--", target.RepoPath)
	case "file_history":
		if path == "" {
			return nil, errors.New("git file_history requires path")
		}
		n := asInt(args["limit"], 30)
		if n < 1 {
			n = 1
		}
		if n > 100 {
			n = 100
		}
		gitArgs = []string{"log", "--follow", "-" + strconv.Itoa(n), "--date=iso", "--pretty=format:%h%x09%ad%x09%an%x09%s", "--", target.RepoPath}
	default:
		return nil, fmt.Errorf("unsupported task Git read operation: %s", operation)
	}
	result, err := runGit(target.Root, gitArgs...)
	return attachTaskGitMetadata(result, args, target.Repository, target.Active), err
}

func (e *Engine) taskGitStage(ctx context.Context, args map[string]any, opts HandleOptions, unstage bool) (map[string]any, error) {
	paths := stringSlice(args["paths"])
	if len(paths) == 0 {
		return nil, errors.New("Git stage/unstage requires at least one path")
	}
	repo, repoPaths, err := e.gitPathsTarget(asString(args["repository"]), paths)
	if err != nil {
		return nil, err
	}
	target, err := e.taskGitTarget(ctx, args, opts, repo.ID, paths[0], true)
	if err != nil {
		return nil, err
	}
	gitArgs := []string{"add", "--"}
	if unstage {
		gitArgs = []string{"restore", "--staged", "--"}
	}
	gitArgs = append(gitArgs, repoPaths...)
	result, err := e.guardedGitAt(target.Root, gitArgs, asString(args["approvalToken"]), opts.SessionID)
	return attachTaskGitMetadata(result, args, repo, true), err
}

func (e *Engine) taskGitCommit(ctx context.Context, args map[string]any, opts HandleOptions) (map[string]any, error) {
	target, err := e.taskGitTarget(ctx, args, opts, asString(args["repository"]), "", true)
	if err != nil {
		return nil, err
	}
	staged, err := runGit(target.Root, "diff", "--cached", "--name-only")
	if err != nil {
		return nil, err
	}
	stagedPaths := strings.Fields(asString(staged["stdout"]))
	if len(stagedPaths) == 0 {
		return nil, errors.New("no staged changes to commit")
	}
	expected := stringSlice(args["expectedPaths"])
	if len(expected) > 0 {
		allowed := map[string]struct{}{}
		for _, path := range expected {
			repo, repoPath, resolveErr := e.gitTarget(asString(args["repository"]), path)
			if resolveErr != nil {
				return nil, resolveErr
			}
			if repo.ID != target.Repository.ID || repo.RelativePath != target.Repository.RelativePath {
				return nil, errors.New("expected commit paths span repositories")
			}
			allowed[repoPath] = struct{}{}
		}
		unexpected := []string{}
		for _, path := range stagedPaths {
			if _, ok := allowed[filepath.ToSlash(filepath.Clean(path))]; !ok {
				unexpected = append(unexpected, path)
			}
		}
		if len(unexpected) > 0 {
			return nil, fmt.Errorf("unexpected staged changes: %s", strings.Join(unexpected, ", "))
		}
	}
	result, err := e.guardedGitAt(target.Root, []string{"commit", "-m", asString(args["message"])}, asString(args["approvalToken"]), opts.SessionID)
	return attachTaskGitMetadata(result, args, target.Repository, true), err
}

func (e *Engine) taskGitPush(ctx context.Context, args map[string]any, opts HandleOptions) (map[string]any, error) {
	if asBool(args["force"], false) {
		return map[string]any{"status": "blocked", "riskLevel": "BLOCKED", "reason": "force push is blocked by CodeLocal"}, nil
	}
	target, err := e.taskGitTarget(ctx, args, opts, asString(args["repository"]), "", true)
	if err != nil {
		return nil, err
	}
	gitArgs := []string{"push"}
	if remote := asString(args["remote"]); remote != "" {
		gitArgs = append(gitArgs, remote)
	}
	if branch := asString(args["branch"]); branch != "" {
		gitArgs = append(gitArgs, branch)
	}
	result, err := e.guardedGitAt(target.Root, gitArgs, asString(args["approvalToken"]), opts.SessionID)
	return attachTaskGitMetadata(result, args, target.Repository, true), err
}
