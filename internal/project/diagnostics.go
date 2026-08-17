package project

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/repository"
)

func gitDiffCheck(ctx context.Context, repo repository.Checkout, path string) []map[string]any {
	args := []string{"-C", repo.Root, "diff", "--check"}
	if strings.TrimSpace(path) != "" && path != "." {
		args = append(args, "--", path)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(os.Environ(), "PAGER=cat", "GIT_PAGER=cat", "CI=1")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	_ = cmd.Run()
	items := []map[string]any{}
	scanner := bufio.NewScanner(strings.NewReader(out.String()))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		items = append(items, map[string]any{
			"message": line, "category": "Warning", "provider": "git-diff-check",
			"repositoryId": repo.ID, "repositoryPath": repo.RelativePath,
		})
	}
	return items
}

func appendLimitedDiagnostics(dst []map[string]any, values []map[string]any, limit int) ([]map[string]any, bool) {
	for _, value := range values {
		dst = append(dst, value)
		if limit > 0 && len(dst) >= limit {
			return dst[:limit], true
		}
	}
	return dst, false
}

func (e *Engine) repositoryDiagnostics(ctx context.Context, path string, limit int) ([]map[string]any, error) {
	if strings.TrimSpace(path) != "" {
		repo, repoPath, err := e.Repositories.ResolvePath(e.workspacePath(path))
		if err != nil {
			return nil, err
		}
		return gitDiffCheck(ctx, repo, repoPath), nil
	}
	out := []map[string]any{}
	for _, repo := range e.Repositories.All() {
		var full bool
		out, full = appendLimitedDiagnostics(out, gitDiffCheck(ctx, repo, ""), limit)
		if full {
			break
		}
	}
	return out, nil
}

func (e *Engine) Diagnostics(ctx context.Context, path string, limit int) (map[string]any, error) {
	return e.diagnostics(ctx, path, limit)
}

func (e *Engine) diagnostics(ctx context.Context, path string, limit int) (map[string]any, error) {
	diagnostics := []map[string]any{}
	if strings.TrimSpace(path) != "" {
		absolute, err := e.FS.Existing(path)
		if err != nil {
			return nil, err
		}
		if e.LSP != nil && e.LSP.Available(absolute) {
			if values, lspErr := e.LSP.Diagnostics(ctx, absolute); lspErr == nil {
				for _, value := range values {
					value["path"] = e.FS.Rel(absolute)
					e.annotatePathMap(value)
					var full bool
					diagnostics, full = appendLimitedDiagnostics(diagnostics, []map[string]any{value}, limit)
					if full {
						return map[string]any{"engine": "lsp+repo-git", "diagnostics": diagnostics}, nil
					}
				}
			}
		}
	}
	gitDiagnostics, err := e.repositoryDiagnostics(ctx, path, limit-len(diagnostics))
	if err != nil {
		return nil, err
	}
	diagnostics, _ = appendLimitedDiagnostics(diagnostics, gitDiagnostics, limit)
	return map[string]any{"engine": "lsp+repo-git", "diagnostics": diagnostics}, nil
}
