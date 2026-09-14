package project

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type CodeGraphSnapshot struct {
	RepositoryID   string `json:"repositoryId"`
	RepositoryPath string `json:"repositoryPath"`
	Branch         string `json:"branch,omitempty"`
	Commit         string `json:"commit,omitempty"`
	SourceHash     string `json:"sourceHash"`
	Revision       string `json:"revision"`
	Status         string `json:"status"`
	Dirty          bool   `json:"dirty"`
	IndexedAt      int64  `json:"indexedAt"`
	FileCount      int    `json:"fileCount"`
	SymbolCount    int    `json:"symbolCount"`
}

func shortHash(value string, length int) string {
	if length <= 0 || len(value) <= length {
		return value
	}
	return value[:length]
}

func sourceRevision(commit, sourceHash string) string {
	commit = strings.TrimSpace(commit)
	if commit == "" {
		return "src:" + shortHash(sourceHash, 20)
	}
	return commit + ":" + shortHash(sourceHash, 20)
}

// SemanticProviderInfo returns only provider availability. It is safe to call
// on the workspace activation critical path because it never builds the
// structural code index or inspects Git state across repositories.
func (e *Engine) SemanticProviderInfo() map[string]any {
	info := map[string]any{"providers": []map[string]any{}, "fallback": "go-native-structure/ripgrep", "routing": "file-extension/polyglot"}
	if e.LSP != nil {
		info = e.LSP.Info()
	}
	return info
}

func (e *Engine) SemanticInfo() map[string]any {
	info := e.SemanticProviderInfo()
	if snapshots, err := e.CodeGraphSnapshots(); err == nil {
		info["codeGraph"] = map[string]any{"status": "current", "repositories": snapshots}
	} else {
		info["codeGraph"] = map[string]any{"status": "unavailable", "error": err.Error()}
	}
	return info
}

func (e *Engine) CodeGraphSnapshots() ([]CodeGraphSnapshot, error) {
	if err := e.refreshStructuralIndex(); err != nil {
		return nil, err
	}
	e.indexMu.Lock()
	defer e.indexMu.Unlock()

	byRepository := map[string][]string{}
	fileCount := map[string]int{}
	symbolCount := map[string]int{}
	for path, file := range e.index {
		key := file.RepositoryID + "\x00" + file.RepositoryPath
		fingerprint := file.ContentHash
		if fingerprint == "" {
			fingerprint = fmt.Sprintf("%d:%d", file.MTimeNS, file.Size)
		}
		byRepository[key] = append(byRepository[key], path+"\x00"+fingerprint)
		fileCount[key]++
		symbolCount[key] += len(file.Symbols)
	}

	out := make([]CodeGraphSnapshot, 0, len(e.Repositories.All()))
	for _, repo := range e.Repositories.All() {
		key := repo.ID + "\x00" + repo.RelativePath
		items := byRepository[key]
		sort.Strings(items)
		hash := sha256.Sum256([]byte(strings.Join(items, "\n")))
		sourceHash := hex.EncodeToString(hash[:])
		branch := strings.TrimSpace(gitOutput(repo.Root, "branch", "--show-current"))
		commit := strings.TrimSpace(gitOutput(repo.Root, "rev-parse", "HEAD"))
		dirty := strings.TrimSpace(gitOutput(repo.Root, "status", "--porcelain=v1", "--untracked-files=normal")) != ""
		out = append(out, CodeGraphSnapshot{
			RepositoryID: repo.ID, RepositoryPath: repo.RelativePath, Branch: branch, Commit: commit,
			SourceHash: sourceHash, Revision: sourceRevision(commit, sourceHash), Status: "current", Dirty: dirty,
			IndexedAt: e.indexBuiltAt, FileCount: fileCount[key], SymbolCount: symbolCount[key],
		})
	}
	return out, nil
}

func gitOutput(root string, args ...string) string {
	command := append([]string{"git"}, args...)
	value, err := run(root, command...)
	if err != nil {
		return ""
	}
	return value
}

func (e *Engine) annotateCanonicalSymbol(value map[string]any) map[string]any {
	if value == nil || fmt.Sprint(value["resolutionMode"]) != "lsp" {
		return value
	}
	path := e.workspacePath(fmt.Sprint(value["path"]))
	name := strings.TrimSpace(fmt.Sprint(value["name"]))
	if path == "" || path == "." || name == "" || name == "<nil>" {
		return value
	}
	repo, repoPath, err := e.Repositories.ResolvePath(path)
	if err != nil {
		return value
	}
	line := intValue(value["line"])
	column := intValue(value["column"])
	detail := strings.TrimSpace(fmt.Sprint(value["detail"]))
	if detail == "<nil>" {
		detail = ""
	}
	identity := strings.Join([]string{repo.ID, repoPath, path, name, detail, strconv.Itoa(line), strconv.Itoa(column)}, "\x00")
	hash := sha256.Sum256([]byte(identity))
	value["symbolId"] = "sym_" + hex.EncodeToString(hash[:16])
	value["repositoryRelativePath"] = repoPath
	value["qualifiedName"] = strings.TrimSpace(strings.Join([]string{detail, name}, " "))
	return value
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	case string:
		parsed, _ := strconv.Atoi(typed)
		return parsed
	default:
		return 0
	}
}
