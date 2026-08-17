package localclient

import (
	"path/filepath"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/repository"
)

func (e *Engine) repositoryExecutionPath(workspacePath string) (repository.Checkout, string, string, bool) {
	workspacePath = filepath.ToSlash(filepath.Clean(strings.TrimSpace(workspacePath)))
	repo, repoPath, err := e.Repositories.ResolvePath(workspacePath)
	if err != nil {
		return repository.Checkout{}, e.Root, workspacePath, false
	}
	return repo, repo.Root, repoPath, true
}
