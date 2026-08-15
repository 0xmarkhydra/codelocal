package localclient

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/orchestration"
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

func verificationProjectProfile(project map[string]any) orchestration.ProjectProfile {
	return orchestration.ProjectProfile{
		Languages:         projectStringSlice(project, "languages"),
		Frameworks:        projectStringSlice(project, "frameworks"),
		BuildCommands:     projectStringSlice(project, "buildCommands"),
		TestCommands:      projectStringSlice(project, "testCommands"),
		TypecheckCommands: projectStringSlice(project, "typecheckCommands"),
		LintCommands:      projectStringSlice(project, "lintCommands"),
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

func changedPathsFromGitStatus(root string) []string {
	status, err := runGit(root, "status", "--porcelain=v1")
	if err != nil {
		return nil
	}
	seen := map[string]struct{}{}
	paths := []string{}
	for _, line := range strings.Split(asString(status["stdout"]), "\n") {
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if arrow := strings.LastIndex(path, " -> "); arrow >= 0 {
			path = strings.TrimSpace(path[arrow+4:])
		}
		path = strings.Trim(path, `"`)
		path = filepath.ToSlash(path)
		if path == "" || strings.HasSuffix(path, "/") || strings.HasPrefix(path, ".codelocal/") {
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
