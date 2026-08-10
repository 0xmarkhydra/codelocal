package project

import (
	"bufio"
	"bytes"
	"context"
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

	"github.com/0xmarkhydra/codelocal/internal/localfs"
	"github.com/0xmarkhydra/codelocal/internal/lsp"
	"github.com/0xmarkhydra/codelocal/internal/security"
)

type indexedSymbol struct {
	Name string
	Line int
}

type indexedFile struct {
	MTimeNS int64
	Size    int64
	Symbols []indexedSymbol
}

type Engine struct {
	FS      *localfs.FS
	LSP     *lsp.Manager
	mu      sync.RWMutex
	cached  map[string]any
	builtAt int64
	indexMu sync.Mutex
	index   map[string]indexedFile
}

func New(fs *localfs.FS) *Engine {
	return &Engine{FS: fs, LSP: lsp.NewManager(fs.Root), index: map[string]indexedFile{}}
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

func (e *Engine) scan() (map[string]any, error) {
	languages := map[string]int{}
	manifests := []string{}
	sourceFiles := 0
	files := 0
	roots := map[string]struct{}{".": {}}
	modules := map[string]struct{}{}
	entrypoints := []string{}
	err := filepath.WalkDir(e.FS.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel := e.FS.Rel(path)
		if rel == "." {
			return nil
		}
		if security.IsSensitivePath(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if e.FS.Ignored(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		files++
		if _, ok := manifestNames[d.Name()]; ok {
			manifests = append(manifests, rel)
			dir := filepath.ToSlash(filepath.Dir(rel))
			if dir == "." {
				dir = "."
			}
			roots[dir] = struct{}{}
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		lang := sourceExt[ext]
		if lang != "" {
			languages[lang]++
			sourceFiles++
			dir := filepath.ToSlash(filepath.Dir(rel))
			if dir != "." {
				modules[dir] = struct{}{}
			}
			base := strings.ToLower(d.Name())
			if base == "main.go" || base == "main.dart" || base == "main.py" || base == "index.ts" || base == "index.js" || base == "main.rs" {
				entrypoints = append(entrypoints, rel)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	frameworks := detectFrameworks(e.FS.Root, manifests)
	languageList := []string{}
	for lang := range languages {
		languageList = append(languageList, lang)
	}
	sort.Strings(languageList)
	sort.Strings(manifests)
	sort.Strings(entrypoints)
	rootList := keys(roots)
	moduleList := keys(modules)
	commands := map[string][]string{"build": {}, "test": {}, "typecheck": {}, "lint": {}}
	if exists(filepath.Join(e.FS.Root, "package.json")) {
		commands = packageCommands(e.FS.Root, commands)
	}
	if exists(filepath.Join(e.FS.Root, "go.mod")) {
		commands["build"] = append(commands["build"], "go build ./...")
		commands["test"] = append(commands["test"], "go test ./...")
	}
	if exists(filepath.Join(e.FS.Root, "Cargo.toml")) {
		commands["build"] = append(commands["build"], "cargo build")
		commands["test"] = append(commands["test"], "cargo test")
	}
	if exists(filepath.Join(e.FS.Root, "pubspec.yaml")) {
		commands["build"] = append(commands["build"], "flutter analyze")
		commands["test"] = append(commands["test"], "flutter test")
	}
	return map[string]any{"generatedAt": time.Now().UnixMilli(), "rootName": filepath.Base(e.FS.Root), "languages": languageList, "frameworks": frameworks, "workspaceRoots": rootList, "entrypoints": entrypoints, "sourceRoots": sourceRoots(moduleList), "testRoots": testRoots(moduleList), "manifests": manifests, "buildCommands": commands["build"], "testCommands": commands["test"], "lintCommands": commands["lint"], "typecheckCommands": commands["typecheck"], "modules": moduleList, "packageManager": packageManager(e.FS.Root), "intelligence": map[string]any{"builtAt": time.Now().UnixMilli(), "dirty": false, "files": files, "sourceFiles": sourceFiles, "languages": languageList}}, nil
}
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
func cloneMap(in map[string]any) map[string]any {
	raw, _ := json.Marshal(in)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}

var symbolRE = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:async\s+)?(?:func|function|class|interface|type|struct|enum|trait|protocol|def|fn|let|const|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)`)

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
		current, ok := e.index[rel]
		if ok && current.Size == info.Size() && current.MTimeNS == info.ModTime().UnixNano() {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		symbols := []indexedSymbol{}
		for _, match := range symbolRE.FindAllSubmatchIndex(data, -1) {
			symbols = append(symbols, indexedSymbol{Name: string(data[match[2]:match[3]]), Line: 1 + bytes.Count(data[:match[0]], []byte("\n"))})
		}
		e.index[rel] = indexedFile{MTimeNS: info.ModTime().UnixNano(), Size: info.Size(), Symbols: symbols}
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
	needle := strings.ToLower(strings.TrimSpace(query))
	e.indexMu.Lock()
	out := []map[string]any{}
	for path, file := range e.index {
		for _, symbol := range file.Symbols {
			if needle != "" && !strings.Contains(strings.ToLower(symbol.Name), needle) {
				continue
			}
			out = append(out, map[string]any{"name": symbol.Name, "path": path, "line": symbol.Line, "kind": "symbol", "provider": "go-native-structure-index"})
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
	return out, nil
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
		out = append(out, map[string]any{"name": name, "path": path, "line": line, "kind": "symbol", "provider": "go-native-structure"})
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
			return value, nil
		}
	}
	return e.Hover(path, line, column)
}

func (e *Engine) ContextForTask(ctx context.Context, task string, limit int) (map[string]any, error) {
	if limit <= 0 {
		limit = 30
	}
	if limit > 100 {
		limit = 100
	}

	terms := unique(strings.FieldsFunc(strings.ToLower(task), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.')
	}))
	if len(terms) > 16 {
		terms = terms[:16]
	}

	scores := map[string]int{}
	reasons := map[string][]string{}
	preferredLine := map[string]int{}
	symbols := []map[string]any{}
	seenSymbols := map[string]struct{}{}

	// Semantic/structural symbols carry substantially more weight than raw text.
	// WorkspaceSymbols uses an active LSP when available and falls back to the
	// native structural index, so this remains useful across languages.
	for _, term := range terms {
		if len(term) < 3 {
			continue
		}
		found, err := e.WorkspaceSymbols(ctx, term, 80)
		if err != nil {
			continue
		}
		for _, symbol := range found {
			file := strings.TrimPrefix(filepath.ToSlash(fmt.Sprint(symbol["path"])), "./")
			if file == "" || file == "." || security.IsSensitivePath(file) {
				continue
			}
			name := strings.ToLower(fmt.Sprint(symbol["name"]))
			weight := 6
			if name == term {
				weight = 10
			} else if strings.Contains(name, term) {
				weight = 8
			}
			scores[file] += weight
			reasons[file] = append(reasons[file], "semantic-symbol:"+term)
			line := 0
			fmt.Sscan(fmt.Sprint(symbol["line"]), &line)
			if line > 0 && (preferredLine[file] == 0 || weight >= 8) {
				preferredLine[file] = line
			}
			key := fmt.Sprintf("%s:%d:%s", file, line, fmt.Sprint(symbol["name"]))
			if _, ok := seenSymbols[key]; !ok {
				seenSymbols[key] = struct{}{}
				symbols = append(symbols, symbol)
			}
		}
	}

	// Literal/text matches are a fallback signal rather than the primary ranker.
	for _, term := range terms {
		if len(term) < 3 {
			continue
		}
		result, err := e.FS.Search(term, ".", 200, true, false)
		if err != nil {
			continue
		}
		matches, _ := result["matches"].([]string)
		for _, match := range matches {
			parts := strings.SplitN(strings.TrimPrefix(match, "./"), ":", 4)
			if len(parts) == 0 || parts[0] == "" {
				continue
			}
			file := filepath.ToSlash(parts[0])
			scores[file] += 2
			reasons[file] = append(reasons[file], "text-match:"+term)
			if preferredLine[file] == 0 && len(parts) > 1 {
				line := 0
				fmt.Sscan(parts[1], &line)
				if line > 0 {
					preferredLine[file] = line
				}
			}
		}
	}

	// Expand from high-confidence seed files through the import graph. Relative
	// imports are resolved against the structural index when possible.
	_ = e.refreshStructuralIndex()
	e.indexMu.Lock()
	knownPaths := make(map[string]struct{}, len(e.index))
	for file := range e.index {
		knownPaths[filepath.ToSlash(file)] = struct{}{}
	}
	e.indexMu.Unlock()
	resolveTarget := func(from, specifier string) string {
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

	graph, _ := e.ImportGraph(5000)
	seed := map[string]struct{}{}
	for file, score := range scores {
		if score >= 4 {
			seed[file] = struct{}{}
		}
	}
	graphEdges := []map[string]any{}
	for _, edge := range graph {
		from := filepath.ToSlash(fmt.Sprint(edge["from"]))
		specifier := fmt.Sprint(edge["specifier"])
		to := resolveTarget(from, specifier)
		if to == "" {
			to = filepath.ToSlash(fmt.Sprint(edge["to"]))
		}
		_, fromSeed := seed[from]
		_, toSeed := seed[to]
		if fromSeed && to != "" {
			scores[to] += 3
			reasons[to] = append(reasons[to], "graph-neighbor:imported-by:"+from)
		}
		if toSeed && from != "" {
			scores[from] += 3
			reasons[from] = append(reasons[from], "graph-neighbor:imports:"+to)
		}
		if fromSeed || toSeed {
			graphEdges = append(graphEdges, map[string]any{"from": from, "to": to, "specifier": specifier})
		}
	}
	if len(graphEdges) > 120 {
		graphEdges = graphEdges[:120]
	}

	type scored struct {
		path  string
		score int
	}
	ranked := make([]scored, 0, len(scores))
	for path, score := range scores {
		if score > 0 {
			ranked = append(ranked, scored{path, score})
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].path < ranked[j].path
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}

	files := []map[string]any{}
	snippets := []map[string]any{}
	budget := 48000
	used := 0
	maxSnippetFiles := 8
	for i, item := range ranked {
		files = append(files, map[string]any{"path": item.path, "score": item.score, "reasons": unique(reasons[item.path]), "preferredLine": preferredLine[item.path]})
		if i >= maxSnippetFiles || used >= budget {
			continue
		}
		center := preferredLine[item.path]
		if center <= 0 {
			center = 1
		}
		start := center - 30
		if start < 1 {
			start = 1
		}
		end := center + 45
		read, err := e.FS.Read(item.path, start, end)
		if err != nil {
			continue
		}
		content, _ := read["content"].(string)
		if len(content) > 8000 {
			content = content[:8000]
		}
		if used+len(content) > budget {
			content = content[:max(0, budget-used)]
		}
		used += len(content)
		snippets = append(snippets, map[string]any{"path": item.path, "startLine": read["startLine"], "endLine": read["endLine"], "totalLines": read["totalLines"], "content": content, "reason": strings.Join(unique(reasons[item.path]), ", ")})
	}

	projectMap, _ := e.Map(false)
	return map[string]any{
		"taskHint":       task,
		"strategy":       "lsp-or-structural-symbols + literal-fallback + import-graph-neighbors + symbol-centered-bounded-snippets",
		"project":        projectMap,
		"rankedFiles":    files,
		"symbols":        symbols,
		"graphEdges":     graphEdges,
		"snippets":       snippets,
		"contextBudget":  map[string]any{"maxChars": budget, "usedChars": used, "maxFiles": maxSnippetFiles},
		"recommendation": "Use this semantic-first packet as the initial coding context. Follow exact definitions/references/callers/callees or targeted line reads only when needed; use search_code mainly for literal strings and unknown text.",
	}, nil
}

func (e *Engine) SemanticInfo() map[string]any {
	if e.LSP != nil {
		return e.LSP.Info()
	}
	return map[string]any{"providers": []map[string]any{}, "fallback": "go-native-structure/ripgrep", "routing": "file-extension/polyglot"}
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
	edges := []map[string]any{}
	patterns := []*regexp.Regexp{regexp.MustCompile(`(?m)(?:from\s+|import\s+[^"']*?from\s+)["']([^"']+)["']`), regexp.MustCompile(`(?m)require\(["']([^"']+)["']\)`), regexp.MustCompile(`(?m)^\s*use\s+([A-Za-z0-9_:]+)`), regexp.MustCompile(`(?m)^\s*#include\s+[<"]([^>"]+)[>"]`)}
	_ = filepath.WalkDir(e.FS.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil || len(edges) >= limit {
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
		info, _ := d.Info()
		if info == nil || info.Size() > 1<<20 {
			return nil
		}
		data, _ := os.ReadFile(path)
		for _, re := range patterns {
			for _, m := range re.FindAllSubmatch(data, -1) {
				edges = append(edges, map[string]any{"from": rel, "to": string(m[1]), "specifier": string(m[1])})
				if len(edges) >= limit {
					return nil
				}
			}
		}
		return nil
	})
	return edges, nil
}

func (e *Engine) Diagnostics(ctx context.Context, path string, limit int) (map[string]any, error) {
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
					diagnostics = append(diagnostics, value)
					if limit > 0 && len(diagnostics) >= limit {
						return map[string]any{"engine": "lsp+go-native", "diagnostics": diagnostics}, nil
					}
				}
			}
		}
	}

	cmd := exec.CommandContext(ctx, "git", "diff", "--check")
	cmd.Dir = e.FS.Root
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	_ = cmd.Run()
	scanner := bufio.NewScanner(strings.NewReader(out.String()))
	for scanner.Scan() {
		line := scanner.Text()
		if path != "" && !strings.Contains(line, path) {
			continue
		}
		diagnostics = append(diagnostics, map[string]any{"message": line, "category": "Warning", "provider": "git-diff-check"})
		if limit > 0 && len(diagnostics) >= limit {
			break
		}
	}
	return map[string]any{"engine": "lsp+go-native", "diagnostics": diagnostics}, nil
}
func (e *Engine) Hover(path string, line, column int) (map[string]any, error) {
	read, err := e.FS.Read(path, line, line)
	if err != nil {
		return nil, err
	}
	content, _ := read["content"].(string)
	return map[string]any{"path": path, "line": line, "column": column, "provider": "go-native-text", "contents": content}, nil
}
func (e *Engine) Callers(name string, limit int) ([]map[string]any, error) {
	return e.FindByText(name+"(", limit)
}
func (e *Engine) Callees(name string, limit int) ([]map[string]any, error) {
	defs, err := e.Definition(name, 1)
	if err != nil || len(defs) == 0 {
		return []map[string]any{}, err
	}
	path := fmt.Sprint(defs[0]["path"])
	line := 1
	fmt.Sscan(fmt.Sprint(defs[0]["line"]), &line)
	read, err := e.FS.Read(path, line, line+80)
	if err != nil {
		return nil, err
	}
	content, _ := read["content"].(string)
	re := regexp.MustCompile(`\b([A-Za-z_$][A-Za-z0-9_$]*)\s*\(`)
	seen := map[string]struct{}{}
	out := []map[string]any{}
	for _, m := range re.FindAllStringSubmatch(content, -1) {
		callee := m[1]
		if callee == name {
			continue
		}
		if _, ok := seen[callee]; ok {
			continue
		}
		seen[callee] = struct{}{}
		out = append(out, map[string]any{"name": callee, "path": path, "provider": "go-native-callgraph"})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
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
