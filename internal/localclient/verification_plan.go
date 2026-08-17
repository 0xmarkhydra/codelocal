package localclient

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/orchestration"
	"github.com/0xmarkhydra/codelocal/internal/repository"
)

func projectStringSlice(project map[string]any, key string) []string {
	value := project[key]
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, strings.TrimSpace(text))
			}
		}
		return out
	default:
		return nil
	}
}

func verificationRepositoryProfiles(value any) []orchestration.RepositoryProfile {
	out := []orchestration.RepositoryProfile{}
	appendProfile := func(item map[string]any) {
		if item == nil {
			return
		}
		out = append(out, orchestration.RepositoryProfile{
			ID: strings.TrimSpace(asString(item["id"])), Path: strings.TrimSpace(asString(item["path"])),
			BuildCommands: stringSlice(item["buildCommands"]), TestCommands: stringSlice(item["testCommands"]),
			TypecheckCommands: stringSlice(item["typecheckCommands"]), LintCommands: stringSlice(item["lintCommands"]),
		})
	}
	switch typed := value.(type) {
	case []map[string]any:
		for _, item := range typed {
			appendProfile(item)
		}
	case []any:
		for _, raw := range typed {
			if item, ok := raw.(map[string]any); ok {
				appendProfile(item)
			}
		}
	}
	return out
}

func verificationProjectProfile(project map[string]any) orchestration.ProjectProfile {
	return orchestration.ProjectProfile{
		Languages:         projectStringSlice(project, "languages"),
		Frameworks:        projectStringSlice(project, "frameworks"),
		BuildCommands:     projectStringSlice(project, "buildCommands"),
		TestCommands:      projectStringSlice(project, "testCommands"),
		TypecheckCommands: projectStringSlice(project, "typecheckCommands"),
		LintCommands:      projectStringSlice(project, "lintCommands"),
		Repositories:      verificationRepositoryProfiles(project["repositories"]),
	}
}

func verificationPlanForChanges(project map[string]any, paths []string) orchestration.VerificationPlan {
	return orchestration.BuildVerificationPlan(orchestration.PlanInput{
		Project:      verificationProjectProfile(project),
		TouchedFiles: append([]string(nil), paths...),
	})
}

func recommendedChecksForChanges(project map[string]any, paths []string) []string {
	plan := verificationPlanForChanges(project, paths)
	checks := make([]string, 0, len(plan.Checks))
	for _, check := range plan.Checks {
		if command := strings.TrimSpace(check.Command); command != "" {
			checks = append(checks, command)
		}
	}
	return checks
}

func changedRepositoryPaths(root string) []string {
	status, err := runGit(root, "status", "--porcelain=v1")
	if err != nil {
		return nil
	}
	seen, paths := map[string]struct{}{}, []string{}
	for _, line := range strings.Split(asString(status["stdout"]), "\n") {
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if arrow := strings.LastIndex(path, " -> "); arrow >= 0 {
			path = strings.TrimSpace(path[arrow+4:])
		}
		path = filepath.ToSlash(strings.Trim(path, `"`))
		if path == "" || strings.HasSuffix(path, "/") || path == ".codelocal/worktrees" || strings.HasPrefix(path, ".codelocal/worktrees/") {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func workspacePathForRepository(repoPath, path string) string {
	repoPath = filepath.ToSlash(filepath.Clean(strings.TrimSpace(repoPath)))
	path = filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	if repoPath == "" || repoPath == "." {
		return path
	}
	if path == "." || path == "" {
		return repoPath
	}
	return filepath.ToSlash(filepath.Join(repoPath, path))
}

func (e *Engine) changedPathsFromGitStatus() []string {
	seen, paths := map[string]struct{}{}, []string{}
	for _, repo := range e.Repositories.All() {
		for _, repoPath := range changedRepositoryPaths(repo.Root) {
			path := workspacePathForRepository(repo.RelativePath, repoPath)
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

type repositoryPathScope struct {
	Repository      repository.Checkout
	WorkspacePaths  []string
	RepositoryPaths []string
}

func (e *Engine) repositoryScopesForPaths(paths []string) ([]repositoryPathScope, error) {
	byKey := map[string]*repositoryPathScope{}
	for _, path := range paths {
		if err := safeGitPath(path); err != nil {
			return nil, err
		}
		repo, repoPath, err := e.Repositories.ResolvePath(path)
		if err != nil {
			// A logical multi-repo workspace may also contain project-level files at
			// its root that are not owned by any Git repository. They still belong
			// to verification/quality scope, but there is no Git diff to collect.
			continue
		}
		key := repo.ID + "\x00" + repo.RelativePath
		scope := byKey[key]
		if scope == nil {
			scope = &repositoryPathScope{Repository: repo}
			byKey[key] = scope
		}
		scope.WorkspacePaths = append(scope.WorkspacePaths, filepath.ToSlash(filepath.Clean(path)))
		scope.RepositoryPaths = append(scope.RepositoryPaths, filepath.ToSlash(filepath.Clean(repoPath)))
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return byKey[keys[i]].Repository.RelativePath < byKey[keys[j]].Repository.RelativePath
	})
	out := make([]repositoryPathScope, 0, len(keys))
	for _, key := range keys {
		out = append(out, *byKey[key])
	}
	return out, nil
}

func (e *Engine) verificationGitDiff(paths []string) (map[string]any, error) {
	if len(paths) == 0 {
		return e.gitDiff("", "", false)
	}
	scopes, err := e.repositoryScopesForPaths(paths)
	if err != nil {
		return nil, err
	}
	items := []map[string]any{}
	var combined strings.Builder
	for _, scope := range scopes {
		args := []string{"diff", "--no-ext-diff", "--unified=2", "--"}
		args = append(args, scope.RepositoryPaths...)
		result, err := runGit(scope.Repository.Root, args...)
		if err != nil {
			return nil, err
		}
		diff := asString(result["output"])
		items = append(items, map[string]any{"repositoryId": scope.Repository.ID, "repositoryPath": scope.Repository.RelativePath, "paths": scope.WorkspacePaths, "diff": diff})
		if strings.TrimSpace(diff) != "" {
			combined.WriteString("[" + scope.Repository.RelativePath + "]\n")
			combined.WriteString(diff)
			if !strings.HasSuffix(diff, "\n") {
				combined.WriteByte('\n')
			}
		}
	}
	return map[string]any{"repositoryCount": len(scopes), "repositories": items, "diff": combined.String(), "stdout": combined.String(), "stderr": "", "output": combined.String(), "exitCode": 0}, nil
}

func recommendedCheckRunsForChanges(project map[string]any, paths []string) []map[string]any {
	plan := verificationPlanForChanges(project, paths)
	out := make([]map[string]any, 0, len(plan.Checks))
	for _, check := range plan.Checks {
		if strings.TrimSpace(check.Command) == "" {
			continue
		}
		out = append(out, map[string]any{"key": check.Key, "command": check.Command, "cwd": check.CWD, "required": check.Required, "scope": check.Scope, "reason": check.Reason})
	}
	return out
}
