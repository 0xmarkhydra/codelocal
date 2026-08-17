package localclient

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/taskexecution"
)

type taskVerificationScope struct {
	Binding        taskexecution.RepositoryBinding
	WorkspacePaths []string
	RepoPaths      []string
}

func (e *Engine) taskExecutionBundle(args map[string]any) (taskexecution.Bundle, bool, error) {
	taskID := strings.TrimSpace(asString(args[privateTaskExecutionID]))
	if taskID == "" || e.TaskExecutions == nil {
		return taskexecution.Bundle{}, false, nil
	}
	return e.TaskExecutions.Store.Get(e.WorkspaceKey, taskID)
}

func bindingKey(binding taskexecution.RepositoryBinding) string {
	return binding.RepositoryID + "\x00" + binding.RepositoryPath
}

func workspacePathForBinding(binding taskexecution.RepositoryBinding, repoPath string) string {
	repoPath = filepath.ToSlash(filepath.Clean(strings.TrimSpace(repoPath)))
	root := filepath.ToSlash(filepath.Clean(strings.TrimSpace(binding.RepositoryPath)))
	if root == "" || root == "." {
		return repoPath
	}
	if repoPath == "" || repoPath == "." {
		return root
	}
	return filepath.ToSlash(filepath.Join(root, repoPath))
}

func (e *Engine) taskChangedPaths(bundle taskexecution.Bundle) []string {
	seen, paths := map[string]struct{}{}, []string{}
	for _, binding := range bundle.RepositoryBindings {
		for _, repoPath := range changedRepositoryPaths(binding.LocalPath) {
			path := workspacePathForBinding(binding, repoPath)
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

func (e *Engine) taskVerificationScopes(bundle taskexecution.Bundle, paths []string) ([]taskVerificationScope, error) {
	byKey := map[string]*taskVerificationScope{}
	bindings := map[string]taskexecution.RepositoryBinding{}
	for _, binding := range bundle.RepositoryBindings {
		bindings[bindingKey(binding)] = binding
	}
	for _, path := range paths {
		repo, repoPath, err := e.Repositories.ResolvePath(path)
		if err != nil {
			continue
		}
		key := repo.ID + "\x00" + repo.RelativePath
		binding, ok := bindings[key]
		if !ok {
			continue
		}
		scope := byKey[key]
		if scope == nil {
			scope = &taskVerificationScope{Binding: binding}
			byKey[key] = scope
		}
		scope.WorkspacePaths = append(scope.WorkspacePaths, filepath.ToSlash(filepath.Clean(path)))
		scope.RepoPaths = append(scope.RepoPaths, filepath.ToSlash(filepath.Clean(repoPath)))
	}
	if len(paths) == 0 {
		for _, binding := range bundle.RepositoryBindings {
			byKey[bindingKey(binding)] = &taskVerificationScope{Binding: binding}
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return byKey[keys[i]].Binding.RepositoryPath < byKey[keys[j]].Binding.RepositoryPath
	})
	out := make([]taskVerificationScope, 0, len(keys))
	for _, key := range keys {
		out = append(out, *byKey[key])
	}
	return out, nil
}

func prefixTaskDiagnostic(binding taskexecution.RepositoryBinding, diagnostic map[string]any) map[string]any {
	if diagnostic == nil {
		return diagnostic
	}
	copy := make(map[string]any, len(diagnostic)+2)
	for key, value := range diagnostic {
		copy[key] = value
	}
	if path := strings.TrimSpace(fmt.Sprint(copy["path"])); path != "" && path != "<nil>" {
		copy["path"] = workspacePathForBinding(binding, path)
	}
	copy["repositoryId"] = binding.RepositoryID
	copy["repositoryPath"] = binding.RepositoryPath
	return copy
}

func diagnosticsFromVerificationResult(binding taskexecution.RepositoryBinding, result map[string]any) []map[string]any {
	out := []map[string]any{}
	switch typed := result["diagnostics"].(type) {
	case []map[string]any:
		for _, item := range typed {
			out = append(out, prefixTaskDiagnostic(binding, item))
		}
	case []any:
		for _, raw := range typed {
			if item, ok := raw.(map[string]any); ok {
				out = append(out, prefixTaskDiagnostic(binding, item))
			}
		}
	}
	return out
}

func diagnosticKey(item map[string]any) string {
	return strings.Join([]string{
		strings.TrimSpace(fmt.Sprint(item["provider"])),
		strings.TrimSpace(fmt.Sprint(item["category"])),
		strings.TrimSpace(fmt.Sprint(item["path"])),
		strings.TrimSpace(fmt.Sprint(item["message"])),
	}, "\x00")
}

func countDiagnosticRegression(before, after []map[string]any) int {
	known := map[string]struct{}{}
	for _, item := range before {
		known[diagnosticKey(item)] = struct{}{}
	}
	regression := 0
	for _, item := range after {
		if _, ok := known[diagnosticKey(item)]; !ok {
			regression++
		}
	}
	return regression
}

func (e *Engine) baselineDiagnostics(baselineID string) []map[string]any {
	if strings.TrimSpace(baselineID) == "" {
		return nil
	}
	values := e.baselines[baselineID]
	out := make([]map[string]any, 0, len(values))
	for _, item := range values {
		copy := make(map[string]any, len(item))
		for key, value := range item {
			copy[key] = value
		}
		out = append(out, copy)
	}
	return out
}

func aggregateTaskQuality(repositoryResults []map[string]any) map[string]any {
	status := "ok"
	blocking, advisory := 0, 0
	items := []map[string]any{}
	for _, item := range repositoryResults {
		quality, _ := item["qualityPolicy"].(map[string]any)
		if quality == nil {
			continue
		}
		blocking += asInt(quality["blockingCount"], 0)
		advisory += asInt(quality["advisoryCount"], 0)
		if value := strings.TrimSpace(asString(quality["status"])); value != "" && value != "ok" {
			status = value
		}
		items = append(items, map[string]any{"repositoryId": item["repositoryId"], "repositoryPath": item["repositoryPath"], "status": quality["status"], "blockingCount": quality["blockingCount"], "advisoryCount": quality["advisoryCount"]})
	}
	if blocking > 0 {
		status = "blocked"
	}
	return map[string]any{"status": status, "blockingCount": blocking, "advisoryCount": advisory, "repositories": items}
}

func (e *Engine) taskVerifyChanges(ctx context.Context, args map[string]any, opts HandleOptions, paths []string, baselineID string) (map[string]any, error) {
	bundle, ok, err := e.taskExecutionBundle(args)
	if err != nil {
		return nil, err
	}
	if !ok {
		return e.verifyChanges(ctx, paths, baselineID)
	}
	if len(paths) == 0 {
		paths = e.taskChangedPaths(bundle)
	}
	scopes, err := e.taskVerificationScopes(bundle, paths)
	if err != nil {
		return nil, err
	}
	repositoryResults := []map[string]any{}
	diagnostics := []map[string]any{}
	diffItems := []map[string]any{}
	var combinedDiff strings.Builder
	for _, scope := range scopes {
		child, err := New(scope.Binding.LocalPath, e.WorkspaceID+"::task", e.WorkspaceName, e.WorkspaceKey+"::task", e.DeviceID)
		if err != nil {
			return nil, err
		}
		result, verifyErr := child.verifyChanges(ctx, scope.RepoPaths, "")
		child.Close()
		if verifyErr != nil {
			return nil, verifyErr
		}
		result["repositoryId"] = scope.Binding.RepositoryID
		result["repositoryPath"] = scope.Binding.RepositoryPath
		result["paths"] = append([]string(nil), scope.WorkspacePaths...)
		repositoryResults = append(repositoryResults, result)
		diagnostics = append(diagnostics, diagnosticsFromVerificationResult(scope.Binding, result)...)
		if rawDiff, ok := result["gitDiff"].(map[string]any); ok {
			diff := asString(rawDiff["diff"])
			diffItems = append(diffItems, map[string]any{"repositoryId": scope.Binding.RepositoryID, "repositoryPath": scope.Binding.RepositoryPath, "paths": scope.WorkspacePaths, "diff": diff})
			if strings.TrimSpace(diff) != "" {
				combinedDiff.WriteString("[" + scope.Binding.RepositoryPath + "]\n")
				combinedDiff.WriteString(diff)
				if !strings.HasSuffix(diff, "\n") {
					combinedDiff.WriteByte('\n')
				}
			}
		}
	}
	projectMap, _ := e.Project.Map(false)
	plan := verificationPlanForChanges(projectMap, paths)
	checks := recommendedChecksForChanges(projectMap, paths)
	checkRuns := recommendedCheckRunsForChanges(projectMap, paths)
	before := e.baselineDiagnostics(baselineID)
	return map[string]any{
		"baselineId":             baselineID,
		"beforeDiagnostics":      before,
		"diagnostics":            diagnostics,
		"diagnosticRegression":   countDiagnosticRegression(before, diagnostics),
		"gitDiff":                map[string]any{"repositoryCount": len(diffItems), "repositories": diffItems, "diff": combinedDiff.String(), "stdout": combinedDiff.String(), "stderr": "", "output": combinedDiff.String(), "exitCode": 0},
		"verificationPlan":       plan,
		"recommendedChecks":      checks,
		"recommendedCheckRuns":   checkRuns,
		"verificationScope":      paths,
		"qualityPolicy":          aggregateTaskQuality(repositoryResults),
		"repositoryVerification": repositoryResults,
		"taskExecution":          map[string]any{"taskId": bundle.TaskID, "provider": string(bundle.Provider), "repositoryCount": len(bundle.RepositoryBindings)},
	}, nil
}
