package project

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/repository"
)

func (e *Engine) workspacePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if filepath.IsAbs(value) {
		return filepath.ToSlash(e.FS.Rel(value))
	}
	return strings.TrimPrefix(filepath.ToSlash(filepath.Clean(value)), "./")
}

func (e *Engine) repositoryForPath(path string) (repository.Checkout, bool) {
	if e == nil || e.Repositories == nil {
		return repository.Checkout{}, false
	}
	repo, _, err := e.Repositories.ResolvePath(e.workspacePath(path))
	return repo, err == nil
}

func (e *Engine) repositoryIdentity(path string) (string, string) {
	repo, ok := e.repositoryForPath(path)
	if !ok {
		return "", ""
	}
	return repo.ID, repo.RelativePath
}

func (e *Engine) annotatePathMap(value map[string]any) map[string]any {
	if value == nil {
		return value
	}
	path := e.workspacePath(fmt.Sprint(value["path"]))
	if path == "" || path == "." {
		return value
	}
	value["path"] = path
	if repo, ok := e.repositoryForPath(path); ok {
		value["repositoryId"] = repo.ID
		value["repositoryPath"] = repo.RelativePath
	}
	return value
}

func (e *Engine) annotatePathMaps(values []map[string]any) []map[string]any {
	for _, value := range values {
		e.annotatePathMap(value)
	}
	return values
}

func (e *Engine) pathOwnedByRepository(path string, repo repository.Checkout) bool {
	owner, ok := e.repositoryForPath(path)
	return ok && owner.ID == repo.ID && owner.RelativePath == repo.RelativePath
}

func (e *Engine) ownedPaths(paths []string, repo repository.Checkout) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if e.pathOwnedByRepository(path, repo) {
			out = append(out, path)
		}
	}
	return out
}

func repositoryCommands(root string) map[string][]string {
	commands := map[string][]string{"build": {}, "test": {}, "typecheck": {}, "lint": {}}
	if exists(filepath.Join(root, "package.json")) {
		commands = packageCommands(root, commands)
	}
	if exists(filepath.Join(root, "go.mod")) {
		commands["build"] = append(commands["build"], "go build ./...")
		commands["test"] = append(commands["test"], "go test ./...")
	}
	if exists(filepath.Join(root, "Cargo.toml")) {
		commands["build"] = append(commands["build"], "cargo build")
		commands["test"] = append(commands["test"], "cargo test")
	}
	if exists(filepath.Join(root, "pubspec.yaml")) {
		commands["build"] = append(commands["build"], "flutter analyze")
		commands["test"] = append(commands["test"], "flutter test")
	}
	return commands
}

func (e *Engine) repositorySummaries(manifests, modules, entrypoints []string) []map[string]any {
	if e.Repositories == nil {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(e.Repositories.All()))
	for _, repo := range e.Repositories.All() {
		repoModules, commands := e.ownedPaths(modules, repo), repositoryCommands(repo.Root)
		out = append(out, map[string]any{
			"id": repo.ID, "path": repo.RelativePath, "identitySource": repo.IdentitySource,
			"manifests": e.ownedPaths(manifests, repo), "modules": repoModules,
			"entrypoints": e.ownedPaths(entrypoints, repo), "sourceRoots": sourceRoots(repoModules), "testRoots": testRoots(repoModules),
			"packageManager": packageManager(repo.Root), "buildCommands": commands["build"], "testCommands": commands["test"],
			"typecheckCommands": commands["typecheck"], "lintCommands": commands["lint"],
		})
	}
	return out
}

func resolveIndexedImport(from, specifier string, knownPaths map[string]struct{}) string {
	if !strings.HasPrefix(specifier, ".") {
		return ""
	}
	base := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(from), specifier)))
	candidates := []string{base}
	for ext := range sourceExt {
		candidates = append(candidates, base+ext, filepath.ToSlash(filepath.Join(base, "index"+ext)))
	}
	for _, candidate := range candidates {
		if _, ok := knownPaths[candidate]; ok {
			return candidate
		}
	}
	return ""
}

func graphTargetIsWorkspacePath(edge map[string]any) bool {
	to := strings.TrimSpace(fmt.Sprint(edge["to"]))
	specifier := strings.TrimSpace(fmt.Sprint(edge["specifier"]))
	return to != "" && to != "<nil>" && (specifier == "" || to != specifier || strings.HasPrefix(specifier, "."))
}

func (e *Engine) annotateGraphEdge(edge map[string]any) map[string]any {
	fromRepo, fromOK := e.repositoryForPath(fmt.Sprint(edge["from"]))
	toRepo, toOK := repository.Checkout{}, false
	if graphTargetIsWorkspacePath(edge) {
		toRepo, toOK = e.repositoryForPath(fmt.Sprint(edge["to"]))
	}
	if fromOK {
		edge["fromRepositoryId"], edge["fromRepositoryPath"] = fromRepo.ID, fromRepo.RelativePath
	}
	if toOK {
		edge["toRepositoryId"], edge["toRepositoryPath"] = toRepo.ID, toRepo.RelativePath
	}
	if fromOK && toOK {
		edge["crossRepository"] = fromRepo.ID != toRepo.ID || fromRepo.RelativePath != toRepo.RelativePath
	}
	return edge
}
