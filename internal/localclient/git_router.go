package localclient

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/repository"
)

func withRepository(result map[string]any, repo repository.Checkout) map[string]any {
	if result == nil {
		result = map[string]any{}
	}
	result["repositoryId"] = repo.ID
	result["repositoryPath"] = repo.RelativePath
	return result
}

func (e *Engine) gitTarget(selector, path string) (repository.Checkout, string, error) {
	selector = strings.TrimSpace(selector)
	if path == "" {
		repo, err := e.Repositories.ResolveSelector(selector)
		return repo, "", err
	}
	if err := safeGitPath(path); err != nil {
		return repository.Checkout{}, "", err
	}
	repo, repoPath, err := e.Repositories.ResolvePath(path)
	if err != nil {
		return repository.Checkout{}, "", err
	}
	if selector != "" && selector != repo.ID && filepath.ToSlash(selector) != repo.RelativePath {
		return repository.Checkout{}, "", errors.New("Git path does not belong to the selected repository")
	}
	return repo, repoPath, nil
}

func (e *Engine) gitPathsTarget(selector string, paths []string) (repository.Checkout, []string, error) {
	if len(paths) == 0 {
		return repository.Checkout{}, nil, errors.New("at least one Git path is required")
	}
	var target repository.Checkout
	repoPaths := make([]string, 0, len(paths))
	for index, path := range paths {
		repo, repoPath, err := e.gitTarget(selector, path)
		if err != nil {
			return repository.Checkout{}, nil, err
		}
		if index == 0 {
			target = repo
		} else if repo.ID != target.ID || repo.RelativePath != target.RelativePath {
			return repository.Checkout{}, nil, errors.New("Git paths span multiple repositories; operate on one repository at a time")
		}
		repoPaths = append(repoPaths, repoPath)
	}
	return target, repoPaths, nil
}

func gitStatusDirty(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "##") {
			return true
		}
	}
	return false
}

func (e *Engine) gitStatus(selector string) (map[string]any, error) {
	if selector != "" || !e.Repositories.IsMulti() {
		repo, err := e.Repositories.ResolveSelector(selector)
		if err != nil {
			return nil, err
		}
		result, err := runGit(repo.Root, "status", "--short", "--branch")
		return withRepository(result, repo), err
	}
	items := []map[string]any{}
	var combined strings.Builder
	for _, repo := range e.Repositories.All() {
		result, err := runGit(repo.Root, "status", "--short", "--branch")
		if err != nil {
			return nil, err
		}
		output := asString(result["output"])
		items = append(items, map[string]any{
			"repositoryId": repo.ID, "repositoryPath": repo.RelativePath,
			"dirty": gitStatusDirty(output), "output": output,
		})
		combined.WriteString("[" + repo.RelativePath + "]\n")
		combined.WriteString(output)
		if !strings.HasSuffix(output, "\n") {
			combined.WriteByte('\n')
		}
	}
	return map[string]any{"repositoryCount": len(items), "repositories": items, "stdout": combined.String(), "stderr": "", "output": combined.String(), "exitCode": 0}, nil
}

func repositoryBranch(root string) string {
	result, err := runGit(root, "branch", "--show-current")
	if err == nil {
		if branch := strings.TrimSpace(asString(result["stdout"])); branch != "" {
			return branch
		}
	}
	if result, err := runGit(root, "rev-parse", "--short", "HEAD"); err == nil {
		if revision := strings.TrimSpace(asString(result["stdout"])); revision != "" {
			return "detached@" + revision
		}
	}
	return "unknown"
}

func (e *Engine) repositoryBranchState() (string, string, []map[string]any) {
	items := []map[string]any{}
	fingerprintParts := []string{}
	common, same := "", true
	for _, repo := range e.Repositories.All() {
		branch := repositoryBranch(repo.Root)
		items = append(items, map[string]any{"repositoryId": repo.ID, "repositoryPath": repo.RelativePath, "branch": branch})
		fingerprintParts = append(fingerprintParts, repo.RelativePath+"="+branch)
		if common == "" {
			common = branch
		} else if common != branch {
			same = false
		}
	}
	if len(items) == 0 {
		return "", "", items
	}
	display := common
	if !same {
		display = "multiple"
	}
	return display, strings.Join(fingerprintParts, "|"), items
}

func (e *Engine) gitDiff(selector, path string, cached bool) (map[string]any, error) {
	gitArgs := []string{"diff", "--no-ext-diff", "--unified=3"}
	if cached {
		gitArgs = append(gitArgs, "--cached")
	}
	if path != "" {
		repo, repoPath, err := e.gitTarget(selector, path)
		if err != nil {
			return nil, err
		}
		gitArgs = append(gitArgs, "--", repoPath)
		result, err := runGit(repo.Root, gitArgs...)
		if result != nil {
			result["diff"] = result["output"]
		}
		return withRepository(result, repo), err
	}
	if selector != "" || !e.Repositories.IsMulti() {
		repo, _, err := e.gitTarget(selector, "")
		if err != nil {
			return nil, err
		}
		result, err := runGit(repo.Root, gitArgs...)
		if result != nil {
			result["diff"] = result["output"]
		}
		return withRepository(result, repo), err
	}
	items := []map[string]any{}
	var combined strings.Builder
	for _, repo := range e.Repositories.All() {
		result, err := runGit(repo.Root, gitArgs...)
		if err != nil {
			return nil, err
		}
		diff := asString(result["output"])
		if strings.TrimSpace(diff) == "" {
			continue
		}
		items = append(items, map[string]any{"repositoryId": repo.ID, "repositoryPath": repo.RelativePath, "diff": diff})
		combined.WriteString("[" + repo.RelativePath + "]\n")
		combined.WriteString(diff)
		if !strings.HasSuffix(diff, "\n") {
			combined.WriteByte('\n')
		}
	}
	return map[string]any{"repositoryCount": len(e.Repositories.All()), "changedRepositories": len(items), "repositories": items, "diff": combined.String(), "stdout": combined.String(), "stderr": "", "output": combined.String(), "exitCode": 0}, nil
}
