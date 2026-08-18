package project

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/0xmarkhydra/codelocal/internal/localfs"
	"github.com/0xmarkhydra/codelocal/internal/lsp"
	"github.com/0xmarkhydra/codelocal/internal/repository"
	"github.com/0xmarkhydra/codelocal/internal/security"
)

type indexedSymbol struct {
	Name string
	Line int
}

type indexedFile struct {
	MTimeNS        int64
	Size           int64
	ContentHash    string
	RepositoryID   string
	RepositoryPath string
	Symbols        []indexedSymbol
	Imports        []string
}

type knowledgeSourceCandidate struct {
	Path                      string
	Provider                  string
	SourceType                string
	ScopePath                 string
	Classification            string
	ContentHash               string
	AdapterVersion            string
	ParserVersion             string
	SemanticNormalizerVersion string
	ParserFingerprint         string
	Size                      int64
}

const (
	knowledgeAdapterVersion            = "1"
	knowledgeParserVersion             = "1"
	knowledgeSemanticNormalizerVersion = "1"
	maxKnowledgeSourceBytes            = int64(2 << 20)
	maxKnowledgeSources                = 512
)

func knowledgeScopePath(rel string) string {
	dir := filepath.ToSlash(filepath.Dir(filepath.FromSlash(rel)))
	if dir == "" || dir == "." {
		return "."
	}
	return dir
}

func knowledgeConfigFileScope(rel, configPath string) (string, bool) {
	lower := strings.ToLower(rel)
	configPath = strings.ToLower(strings.TrimPrefix(configPath, "/"))
	if lower == configPath {
		return ".", true
	}
	needle := "/" + configPath
	if !strings.HasSuffix(lower, needle) {
		return "", false
	}
	root := strings.Trim(rel[:len(rel)-len(needle)], "/")
	if root == "" {
		root = "."
	}
	return root, true
}

func knowledgeConfigDirScope(rel, configDir string) (string, bool) {
	lower := strings.ToLower(rel)
	configDir = strings.ToLower(strings.Trim(configDir, "/")) + "/"
	if strings.HasPrefix(lower, configDir) {
		return ".", true
	}
	needle := "/" + configDir
	index := strings.LastIndex(lower, needle)
	if index < 0 {
		return "", false
	}
	root := strings.Trim(rel[:index], "/")
	if root == "" {
		root = "."
	}
	return root, true
}

func knowledgeSourceForPath(rel string) (knowledgeSourceCandidate, bool) {
	rel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(rel))))
	if rel == "" || rel == "." || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, "/") || security.IsSensitivePath(rel) {
		return knowledgeSourceCandidate{}, false
	}
	lower := strings.ToLower(rel)
	base := strings.ToLower(filepath.Base(filepath.FromSlash(rel)))
	cursorScope, cursorConfig := knowledgeConfigDirScope(rel, ".cursor/rules")
	copilotScope, copilotRoot := knowledgeConfigFileScope(rel, ".github/copilot-instructions.md")
	copilotInstructionsScope, copilotInstructions := knowledgeConfigDirScope(rel, ".github/instructions")
	candidate := knowledgeSourceCandidate{Path: rel, ScopePath: ".", Classification: "private_project", AdapterVersion: knowledgeAdapterVersion, ParserVersion: knowledgeParserVersion, SemanticNormalizerVersion: knowledgeSemanticNormalizerVersion}
	switch {
	case base == "agents.md":
		candidate.Provider = "agents"
		candidate.SourceType = "instructions"
		candidate.ScopePath = knowledgeScopePath(rel)
	case base == "claude.md":
		candidate.Provider = "claude"
		candidate.SourceType = "instructions"
		candidate.ScopePath = knowledgeScopePath(rel)
	case base == "claude.local.md":
		candidate.Provider = "claude"
		candidate.SourceType = "instructions"
		candidate.ScopePath = knowledgeScopePath(rel)
		candidate.Classification = "local_private"
	case cursorConfig && (strings.HasSuffix(lower, ".mdc") || strings.HasSuffix(lower, ".md")):
		candidate.Provider = "cursor"
		candidate.SourceType = "rule"
		candidate.ScopePath = cursorScope
	case copilotRoot:
		candidate.Provider = "github-copilot"
		candidate.SourceType = "instructions"
		candidate.ScopePath = copilotScope
	case copilotInstructions && strings.HasSuffix(lower, ".instructions.md"):
		candidate.Provider = "github-copilot"
		candidate.SourceType = "instructions"
		candidate.ScopePath = copilotInstructionsScope
	case lower == ".codelocal/quality.json":
		candidate.Provider = "codelocal"
		candidate.SourceType = "quality_policy"
		candidate.Classification = "private_project"
	case lower == ".codelocal/project.json":
		candidate.Provider = "codelocal"
		candidate.SourceType = "project_metadata"
		candidate.Classification = "local_private"
	default:
		return knowledgeSourceCandidate{}, false
	}
	fingerprint := sha256.Sum256([]byte(strings.Join([]string{"project-knowledge-discovery", candidate.Provider, candidate.SourceType, candidate.AdapterVersion, candidate.ParserVersion, candidate.SemanticNormalizerVersion}, "\x00")))
	candidate.ParserFingerprint = fmt.Sprintf("%x", fingerprint[:])
	return candidate, true
}

func hydrateKnowledgeSource(path string, candidate knowledgeSourceCandidate) (knowledgeSourceCandidate, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maxKnowledgeSourceBytes {
		return knowledgeSourceCandidate{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return knowledgeSourceCandidate{}, false
	}
	probe := data
	if len(probe) > 8192 {
		probe = probe[:8192]
	}
	if bytes.IndexByte(probe, 0) >= 0 {
		return knowledgeSourceCandidate{}, false
	}
	hash := sha256.Sum256(data)
	candidate.ContentHash = fmt.Sprintf("%x", hash[:])
	candidate.Size = info.Size()
	return candidate, true
}

func knowledgeSourceMap(candidate knowledgeSourceCandidate) map[string]any {
	return map[string]any{
		"path": candidate.Path, "provider": candidate.Provider, "sourceType": candidate.SourceType,
		"scopePath": candidate.ScopePath, "classification": candidate.Classification, "contentHash": candidate.ContentHash,
		"adapterVersion": candidate.AdapterVersion, "parserVersion": candidate.ParserVersion,
		"semanticNormalizerVersion": candidate.SemanticNormalizerVersion, "parserFingerprint": candidate.ParserFingerprint,
		"size": candidate.Size,
	}
}

type Engine struct {
	FS           *localfs.FS
	LSP          *lsp.Manager
	Repositories *repository.Registry
	mu           sync.RWMutex
	cached       map[string]any
	builtAt      int64
	indexMu      sync.Mutex
	index        map[string]indexedFile
	indexBuiltAt int64
}

func New(fs *localfs.FS) *Engine {
	return NewWithRepositories(fs, repository.New(fs.Root, filepath.Base(fs.Root)))
}

func NewWithRepositories(fs *localfs.FS, repositories *repository.Registry) *Engine {
	if repositories == nil {
		repositories = repository.New(fs.Root, filepath.Base(fs.Root))
	}
	return &Engine{FS: fs, LSP: lsp.NewManager(fs.Root), Repositories: repositories, index: map[string]indexedFile{}}
}

func (e *Engine) Close() {
	if e.LSP != nil {
		e.LSP.Close()
	}
}

func (e *Engine) Invalidate() {
	e.mu.Lock()
	e.cached = nil
	e.builtAt = 0
	e.mu.Unlock()
}

var sourceExt = map[string]string{".ts": "typescript", ".tsx": "typescript", ".js": "javascript", ".jsx": "javascript", ".mjs": "javascript", ".cjs": "javascript", ".go": "go", ".rs": "rust", ".py": "python", ".swift": "swift", ".c": "c", ".h": "c", ".cc": "cpp", ".cpp": "cpp", ".hpp": "cpp", ".dart": "dart", ".java": "java", ".kt": "kotlin", ".kts": "kotlin", ".lua": "lua", ".zig": "zig", ".rb": "ruby", ".php": "php"}
var manifestNames = map[string]struct{}{"package.json": {}, "go.mod": {}, "Cargo.toml": {}, "pyproject.toml": {}, "requirements.txt": {}, "pubspec.yaml": {}, "Package.swift": {}, "pom.xml": {}, "build.gradle": {}, "build.gradle.kts": {}, "Makefile": {}}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }
func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func sourceRoots(mods []string) []string {
	roots := []string{}
	for _, m := range mods {
		base := filepath.Base(m)
		if base == "src" || base == "lib" || base == "app" || strings.HasSuffix(m, "/src") {
			roots = append(roots, m)
		}
	}
	if len(roots) == 0 {
		roots = []string{"."}
	}
	return unique(roots)
}
func testRoots(mods []string) []string {
	roots := []string{}
	for _, m := range mods {
		l := strings.ToLower(m)
		if strings.Contains(l, "test") || strings.Contains(l, "spec") {
			roots = append(roots, m)
		}
	}
	return unique(roots)
}
func unique(in []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, v := range in {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

var contextStopwords = map[string]struct{}{
	"and": {}, "are": {}, "can": {}, "code": {}, "for": {}, "from": {}, "into": {}, "the": {}, "this": {}, "that": {}, "with": {},
	"các": {}, "cần": {}, "cho": {}, "của": {}, "đang": {}, "được": {}, "không": {}, "khi": {}, "làm": {}, "này": {}, "thêm": {}, "trong": {}, "tối": {}, "việc": {}, "với": {}, "sửa": {},
}

func contextTerms(task string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 8)
	for _, token := range strings.FieldsFunc(strings.ToLower(task), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' || r == '/')
	}) {
		token = strings.Trim(token, "._-/")
		if utf8Len := len([]rune(token)); utf8Len < 3 || utf8Len > 80 {
			continue
		}
		if _, ignored := contextStopwords[token]; ignored {
			continue
		}
		if _, duplicate := seen[token]; duplicate {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
		if len(out) == 8 {
			break
		}
	}
	return out
}

func packageManager(root string) string {
	if exists(filepath.Join(root, "pnpm-lock.yaml")) {
		return "pnpm"
	}
	if exists(filepath.Join(root, "yarn.lock")) {
		return "yarn"
	}
	if exists(filepath.Join(root, "bun.lockb")) {
		return "bun"
	}
	if exists(filepath.Join(root, "package-lock.json")) {
		return "npm"
	}
	return ""
}
func detectFrameworks(root string, manifests []string) []string {
	set := map[string]struct{}{}
	for _, manifest := range manifests {
		if filepath.Base(manifest) != "package.json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, manifest))
		if err != nil {
			continue
		}
		text := strings.ToLower(string(data))
		for _, pair := range [][2]string{{"next", "next"}, {"react", "react"}, {"express", "express"}, {"fastify", "fastify"}, {"nestjs", "@nestjs"}, {"vite", "vite"}} {
			if strings.Contains(text, `"`+pair[1]+`"`) {
				set[pair[0]] = struct{}{}
			}
		}
	}
	if exists(filepath.Join(root, "pubspec.yaml")) {
		set["flutter/dart"] = struct{}{}
	}
	if exists(filepath.Join(root, "Cargo.toml")) {
		set["rust"] = struct{}{}
	}
	return keys(set)
}
func packageCommands(root string, commands map[string][]string) map[string][]string {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return commands
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return commands
	}
	pm := packageManager(root)
	if pm == "" {
		pm = "npm"
	}
	for name := range pkg.Scripts {
		cmd := pm + " "
		if pm == "npm" {
			cmd += "run "
		}
		cmd += name
		lower := strings.ToLower(name)
		switch {
		case lower == "build" || strings.HasPrefix(lower, "build:"):
			commands["build"] = append(commands["build"], cmd)
		case lower == "test" || strings.HasPrefix(lower, "test:"):
			commands["test"] = append(commands["test"], cmd)
		case strings.Contains(lower, "typecheck") || lower == "check":
			commands["typecheck"] = append(commands["typecheck"], cmd)
		case strings.Contains(lower, "lint"):
			commands["lint"] = append(commands["lint"], cmd)
		}
	}
	return commands
}

func (e *Engine) Map(force bool) (map[string]any, error) {
	e.mu.RLock()
	cached := e.cached
	built := e.builtAt
	e.mu.RUnlock()
	if !force && cached != nil && time.Since(time.UnixMilli(built)) < 30*time.Second {
		return cloneMap(cached), nil
	}
	value, err := e.scan()
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	e.cached = value
	e.builtAt = time.Now().UnixMilli()
	e.mu.Unlock()
	return cloneMap(value), nil
}

// CachedMap returns the last project snapshot without triggering a filesystem
// walk. Background Project Brain sync uses this path so polling never turns
// into a periodic full-repository scan. An invalidated or never-built cache is
// reported as unavailable and may be refreshed only on an explicit work path.
func (e *Engine) CachedMap() (map[string]any, bool) {
	if e == nil {
		return nil, false
	}
	e.mu.RLock()
	cached := e.cached
	e.mu.RUnlock()
	if cached == nil {
		return nil, false
	}
	return cloneMap(cached), true
}
func cloneMap(in map[string]any) map[string]any {
	raw, _ := json.Marshal(in)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}

var symbolRE = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:async\s+)?(?:func|function|class|interface|type|struct|enum|trait|protocol|def|fn|let|const|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)`)
var importPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)(?:from\s+|import\s+[^"']*?from\s+)["']([^"']+)["']`),
	regexp.MustCompile(`(?m)require\(["']([^"']+)["']\)`),
	regexp.MustCompile(`(?m)^\s*use\s+([A-Za-z0-9_:]+)`),
	regexp.MustCompile(`(?m)^\s*#include\s+[<"]([^>"]+)[>"]`),
}

func (e *Engine) refreshStructuralIndex() error {
	e.indexMu.Lock()
	defer e.indexMu.Unlock()
	seen := map[string]struct{}{}
	err := filepath.WalkDir(e.FS.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel := e.FS.Rel(path)
		if rel != "." && (security.IsSensitivePath(rel) || e.FS.Ignored(rel, d.IsDir())) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || sourceExt[strings.ToLower(filepath.Ext(path))] == "" {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil || info.Size() > 2<<20 {
			return nil
		}
		seen[rel] = struct{}{}
		repositoryID, repositoryPath := e.repositoryIdentity(rel)
		current, ok := e.index[rel]
		if ok && current.Size == info.Size() && current.MTimeNS == info.ModTime().UnixNano() {
			current.RepositoryID, current.RepositoryPath = repositoryID, repositoryPath
			e.index[rel] = current
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		symbols, imports := structuralSymbolsAndImports(path, data)
		contentHash := sha256.Sum256(data)
		e.index[rel] = indexedFile{MTimeNS: info.ModTime().UnixNano(), Size: info.Size(), ContentHash: fmt.Sprintf("%x", contentHash[:]), RepositoryID: repositoryID, RepositoryPath: repositoryPath, Symbols: symbols, Imports: imports}
		return nil
	})
	if err != nil {
		return err
	}
	for path := range e.index {
		if _, ok := seen[path]; !ok {
			delete(e.index, path)
		}
	}
	e.indexBuiltAt = time.Now().UnixMilli()
	return nil
}

func (e *Engine) Symbols(query string, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	if err := e.refreshStructuralIndex(); err != nil {
		return nil, err
	}
	return e.symbolsFromIndex(query, limit), nil
}

func (e *Engine) symbolsFromIndex(query string, limit int) []map[string]any {
	needle := strings.ToLower(strings.TrimSpace(query))
	e.indexMu.Lock()
	out := []map[string]any{}
	for path, file := range e.index {
		for _, symbol := range file.Symbols {
			if needle != "" && !strings.Contains(strings.ToLower(symbol.Name), needle) {
				continue
			}
			item := map[string]any{"name": symbol.Name, "path": path, "line": symbol.Line, "kind": "symbol", "provider": "go-native-structure-index"}
			if file.RepositoryID != "" {
				item["repositoryId"], item["repositoryPath"] = file.RepositoryID, file.RepositoryPath
			}
			out = append(out, item)
		}
	}
	e.indexMu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		leftPath, rightPath := fmt.Sprint(out[i]["path"]), fmt.Sprint(out[j]["path"])
		if leftPath != rightPath {
			return leftPath < rightPath
		}
		leftLine, _ := out[i]["line"].(int)
		rightLine, _ := out[j]["line"].(int)
		if leftLine != rightLine {
			return leftLine < rightLine
		}
		return fmt.Sprint(out[i]["name"]) < fmt.Sprint(out[j]["name"])
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
func (e *Engine) DocumentSymbols(path string, limit int) ([]map[string]any, error) {
	read, err := e.FS.Read(path, 0, 0)
	if err != nil {
		return nil, err
	}
	content, _ := read["content"].(string)
	if content == "" {
		return []map[string]any{}, nil
	}
	out := []map[string]any{}
	data := []byte(content)
	for _, match := range symbolRE.FindAllSubmatchIndex(data, -1) {
		name := string(data[match[2]:match[3]])
		line := 1 + bytes.Count(data[:match[0]], []byte("\n"))
		out = append(out, e.annotatePathMap(map[string]any{"name": name, "path": path, "line": line, "kind": "symbol", "provider": "go-native-structure"}))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (e *Engine) WorkspaceSymbols(ctx context.Context, query string, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 200
	}
	if e.LSP != nil {
		if values, err := e.LSP.WorkspaceSymbols(ctx, query); err == nil && len(values) > 0 {
			values = e.annotatePathMaps(values)
			if len(values) > limit {
				values = values[:limit]
			}
			return values, nil
		}
	}
	return e.Symbols(query, limit)
}

func (e *Engine) DocumentSymbolsAt(ctx context.Context, path string, limit int) ([]map[string]any, error) {
	absolute, err := e.FS.Existing(path)
	if err != nil {
		return nil, err
	}
	if e.LSP != nil && e.LSP.Available(absolute) {
		if values, lspErr := e.LSP.DocumentSymbols(ctx, absolute); lspErr == nil && len(values) > 0 {
			for _, value := range values {
				value["path"] = e.FS.Rel(absolute)
				e.annotatePathMap(value)
			}
			if limit > 0 && len(values) > limit {
				values = values[:limit]
			}
			return values, nil
		}
	}
	return e.DocumentSymbols(path, limit)
}

func (e *Engine) DefinitionAt(ctx context.Context, path string, line, column int, name string, limit int) ([]map[string]any, error) {
	if strings.TrimSpace(path) != "" {
		absolute, err := e.FS.Existing(path)
		if err != nil {
			return nil, err
		}
		if e.LSP != nil && e.LSP.Available(absolute) {
			if values, lspErr := e.LSP.Definition(ctx, absolute, line, column); lspErr == nil && len(values) > 0 {
				for _, value := range values {
					if raw, ok := value["path"].(string); ok && raw != "" {
						value["path"] = e.FS.Rel(raw)
					}
					e.annotatePathMap(value)
				}
				if limit > 0 && len(values) > limit {
					values = values[:limit]
				}
				return values, nil
			}
		}
	}
	return e.Definition(name, limit)
}

func (e *Engine) ReferencesAt(ctx context.Context, path string, line, column int, name string, limit int) ([]map[string]any, error) {
	if strings.TrimSpace(path) != "" {
		absolute, err := e.FS.Existing(path)
		if err != nil {
			return nil, err
		}
		if e.LSP != nil && e.LSP.Available(absolute) {
			if values, lspErr := e.LSP.References(ctx, absolute, line, column); lspErr == nil && len(values) > 0 {
				for _, value := range values {
					if raw, ok := value["path"].(string); ok && raw != "" {
						value["path"] = e.FS.Rel(raw)
					}
					e.annotatePathMap(value)
				}
				if limit > 0 && len(values) > limit {
					values = values[:limit]
				}
				return values, nil
			}
		}
	}
	return e.References(name, limit)
}

func (e *Engine) ImplementationsAt(ctx context.Context, path string, line, column int, name string, limit int) ([]map[string]any, error) {
	if strings.TrimSpace(path) != "" {
		absolute, err := e.FS.Existing(path)
		if err != nil {
			return nil, err
		}
		if e.LSP != nil && e.LSP.Available(absolute) {
			if values, lspErr := e.LSP.Implementations(ctx, absolute, line, column); lspErr == nil && len(values) > 0 {
				for _, value := range values {
					if raw, ok := value["path"].(string); ok && raw != "" {
						value["path"] = e.FS.Rel(raw)
					}
					e.annotatePathMap(value)
				}
				if limit > 0 && len(values) > limit {
					values = values[:limit]
				}
				return values, nil
			}
		}
	}
	return e.Definition(name, limit)
}

func (e *Engine) HoverAt(ctx context.Context, path string, line, column int) (map[string]any, error) {
	absolute, err := e.FS.Existing(path)
	if err != nil {
		return nil, err
	}
	if e.LSP != nil && e.LSP.Available(absolute) {
		if value, lspErr := e.LSP.Hover(ctx, absolute, line, column); lspErr == nil && value != nil {
			value["path"] = e.FS.Rel(absolute)
			value["line"] = line
			value["column"] = column
			return e.annotatePathMap(value), nil
		}
	}
	return e.Hover(path, line, column)
}

func (e *Engine) FindByText(query string, limit int) ([]map[string]any, error) {
	if strings.TrimSpace(query) == "" {
		return []map[string]any{}, nil
	}
	result, err := e.FS.Search(query, ".", limit, true, false)
	if err != nil {
		return nil, err
	}
	matchesRaw, _ := result["matches"].([]string)
	out := []map[string]any{}
	for _, line := range matchesRaw {
		parts := strings.SplitN(strings.TrimPrefix(line, "./"), ":", 4)
		item := map[string]any{"provider": "ripgrep", "text": line}
		if len(parts) > 0 {
			item["path"] = parts[0]
		}
		if len(parts) > 1 {
			item["line"] = parts[1]
		}
		if len(parts) > 2 {
			item["column"] = parts[2]
		}
		out = append(out, item)
	}
	return out, nil
}
func (e *Engine) Definition(name string, limit int) ([]map[string]any, error) {
	symbols, err := e.Symbols(name, limit)
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, s := range symbols {
		if strings.EqualFold(fmt.Sprint(s["name"]), name) || strings.Contains(strings.ToLower(fmt.Sprint(s["name"])), strings.ToLower(name)) {
			out = append(out, s)
		}
	}
	return out, nil
}
func (e *Engine) References(name string, limit int) ([]map[string]any, error) {
	return e.FindByText(name, limit)
}
func (e *Engine) ImportGraph(limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 2000
	}
	if err := e.refreshStructuralIndex(); err != nil {
		return nil, err
	}
	return e.importGraphFromIndex(limit), nil
}

func (e *Engine) importGraphFromIndex(limit int) []map[string]any {
	e.indexMu.Lock()
	defer e.indexMu.Unlock()
	paths, known := make([]string, 0, len(e.index)), map[string]struct{}{}
	for path := range e.index {
		paths, known[path] = append(paths, path), struct{}{}
	}
	sort.Strings(paths)
	edges := []map[string]any{}
	for _, path := range paths {
		for _, specifier := range e.index[path].Imports {
			to := resolveIndexedImport(path, specifier, known)
			if to == "" {
				to = specifier
			}
			edges = append(edges, e.annotateGraphEdge(map[string]any{"from": path, "to": to, "specifier": specifier}))
			if len(edges) >= limit {
				return edges
			}
		}
	}
	return edges
}

func (e *Engine) Hover(path string, line, column int) (map[string]any, error) {
	read, err := e.FS.Read(path, line, line)
	if err != nil {
		return nil, err
	}
	content, _ := read["content"].(string)
	return map[string]any{"path": path, "line": line, "column": column, "provider": "go-native-text", "contents": content}, nil
}
func run(root string, args ...string) (string, error) {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PAGER=cat", "GIT_PAGER=cat", "CI=1")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return out.String(), err
		}
	}
	return out.String(), nil
}
