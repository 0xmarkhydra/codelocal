package project

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/security"
)

type taskContextEvidence struct {
	engine        *Engine
	task          string
	terms         []string
	scores        map[string]int
	reasons       map[string][]string
	preferredLine map[string]int
	symbols       []map[string]any
	seenSymbols   map[string]struct{}
}

func newTaskContextEvidence(engine *Engine, task string) *taskContextEvidence {
	return &taskContextEvidence{
		engine: engine, task: task, terms: contextTerms(task), scores: map[string]int{},
		reasons: map[string][]string{}, preferredLine: map[string]int{}, seenSymbols: map[string]struct{}{},
	}
}

func (e *taskContextEvidence) addSemantic(ctx context.Context) {
	_ = e.engine.refreshStructuralIndex()
	for _, term := range e.terms {
		e.addSemanticTerm(ctx, term)
	}
}

func (e *taskContextEvidence) addSemanticTerm(ctx context.Context, term string) {
	found := e.engine.symbolsFromIndex(term, 80)
	if e.engine.LSP != nil {
		if semantic, err := e.engine.LSP.ActiveWorkspaceSymbols(ctx, term); err == nil {
			found = append(semantic, found...)
		}
	}
	if len(found) > 80 {
		found = found[:80]
	}
	for _, symbol := range found {
		e.addSemanticSymbol(symbol, term)
	}
}

func (e *taskContextEvidence) addSemanticSymbol(symbol map[string]any, term string) {
	e.engine.annotatePathMap(symbol)
	file := strings.TrimPrefix(filepath.ToSlash(fmt.Sprint(symbol["path"])), "./")
	if file == "" || file == "." || security.IsSensitivePath(file) {
		return
	}
	name := strings.ToLower(fmt.Sprint(symbol["name"]))
	weight := semanticTermWeight(name, term)
	e.scores[file] += weight
	e.reasons[file] = append(e.reasons[file], "semantic-symbol:"+term)
	line := 0
	fmt.Sscan(fmt.Sprint(symbol["line"]), &line)
	if line > 0 && (e.preferredLine[file] == 0 || weight >= 8) {
		e.preferredLine[file] = line
	}
	key := fmt.Sprintf("%s:%d:%s", file, line, fmt.Sprint(symbol["name"]))
	if _, exists := e.seenSymbols[key]; !exists {
		e.seenSymbols[key] = struct{}{}
		e.symbols = append(e.symbols, symbol)
	}
}

func semanticTermWeight(name, term string) int {
	if name == term {
		return 10
	}
	if strings.Contains(name, term) {
		return 8
	}
	return 6
}

func (e *taskContextEvidence) addTextMatches() {
	if len(e.terms) == 0 {
		return
	}
	patterns := make([]string, 0, len(e.terms))
	for _, term := range e.terms {
		patterns = append(patterns, regexp.QuoteMeta(term))
	}
	maxMatches := min(600, max(200, len(e.terms)*60))
	result, err := e.engine.FS.Search("(?i)("+strings.Join(patterns, "|")+")", ".", maxMatches, false, false)
	if err != nil {
		return
	}
	matches, _ := result["matches"].([]string)
	for _, match := range matches {
		e.addTextMatch(match)
	}
}

func (e *taskContextEvidence) addTextMatch(match string) {
	parts := strings.SplitN(strings.TrimPrefix(match, "./"), ":", 4)
	if len(parts) == 0 || parts[0] == "" {
		return
	}
	file := filepath.ToSlash(parts[0])
	lowerMatch := strings.ToLower(match)
	matched := false
	for _, term := range e.terms {
		if strings.Contains(lowerMatch, term) {
			e.scores[file] += 2
			e.reasons[file] = append(e.reasons[file], "text-match:"+term)
			matched = true
		}
	}
	if !matched {
		e.scores[file] += 2
		e.reasons[file] = append(e.reasons[file], "text-match:task")
	}
	if e.preferredLine[file] == 0 && len(parts) > 1 {
		line := 0
		fmt.Sscan(parts[1], &line)
		if line > 0 {
			e.preferredLine[file] = line
		}
	}
}

func (e *taskContextEvidence) knownPaths() map[string]struct{} {
	e.engine.indexMu.Lock()
	defer e.engine.indexMu.Unlock()
	known := make(map[string]struct{}, len(e.engine.index))
	for file := range e.engine.index {
		known[filepath.ToSlash(file)] = struct{}{}
	}
	return known
}

func resolveContextTarget(from, specifier string, known map[string]struct{}) string {
	if !strings.HasPrefix(specifier, ".") {
		return ""
	}
	base := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(from), specifier)))
	candidates := []string{base}
	for ext := range sourceExt {
		candidates = append(candidates, base+ext, filepath.ToSlash(filepath.Join(base, "index"+ext)))
	}
	for _, candidate := range candidates {
		if _, ok := known[candidate]; ok {
			return candidate
		}
	}
	return ""
}

func (e *taskContextEvidence) graphNeighbors() ([]map[string]any, repositoryRoute) {
	known, graph := e.knownPaths(), e.engine.importGraphFromIndex(5000)
	seed := map[string]struct{}{}
	for file, score := range e.scores {
		if score >= 4 {
			seed[file] = struct{}{}
		}
	}
	edges := []map[string]any{}
	for _, edge := range graph {
		e.addGraphEdge(edge, known, seed, &edges)
	}
	route := e.engine.routeRepositories(e.task, e.scores, edges)
	return filterGraphEdgesForRoute(e.engine, edges, route), route
}

func (e *taskContextEvidence) addGraphEdge(edge map[string]any, known, seed map[string]struct{}, out *[]map[string]any) {
	from := filepath.ToSlash(fmt.Sprint(edge["from"]))
	specifier := fmt.Sprint(edge["specifier"])
	to := resolveContextTarget(from, specifier, known)
	if to == "" {
		to = filepath.ToSlash(fmt.Sprint(edge["to"]))
	}
	_, fromSeed := seed[from]
	_, toSeed := seed[to]
	if fromSeed && to != "" {
		e.scores[to] += 3
		e.reasons[to] = append(e.reasons[to], "graph-neighbor:imported-by:"+from)
	}
	if toSeed && from != "" {
		e.scores[from] += 3
		e.reasons[from] = append(e.reasons[from], "graph-neighbor:imports:"+to)
	}
	if fromSeed || toSeed {
		*out = append(*out, e.engine.annotateGraphEdge(map[string]any{"from": from, "to": to, "specifier": specifier}))
	}
}

func boundedTaskLimit(limit int) int {
	if limit <= 0 {
		return 30
	}
	return min(limit, 100)
}

func (e *taskContextEvidence) packet(limit int, graphEdges []map[string]any, route repositoryRoute) map[string]any {
	edgeLimit := min(40, max(12, limit*2))
	if len(graphEdges) > edgeLimit {
		graphEdges = graphEdges[:edgeLimit]
	}
	ranked := rankContextPaths(e.engine, e.scores, route, limit)
	budget, maxSnippetFiles := 48000, 8
	files, snippets, used, repositoryBudgets, repositoryUsed := buildRepositoryAwareContext(e.engine, ranked, e.reasons, e.preferredLine, route, budget, maxSnippetFiles)
	symbols := filterPathMapsForRoute(e.engine, e.symbols, route)
	symbolLimit := min(60, max(12, limit*2))
	if len(symbols) > symbolLimit {
		symbols = symbols[:symbolLimit]
	}
	projectMap, _ := e.engine.Map(false)
	return map[string]any{
		"taskHint": e.task, "strategy": "lsp-or-structural-symbols + literal-fallback + cached-import-graph-neighbors + symbol-centered-bounded-snippets",
		"project": projectMap, "rankedFiles": files, "symbols": symbols, "graphEdges": graphEdges, "snippets": snippets,
		"repositoryRoute": repositoryRouteMap(route, repositoryBudgets, repositoryUsed),
		"contextBudget":   map[string]any{"maxChars": budget, "usedChars": used, "maxFiles": maxSnippetFiles, "repositoryFocused": route.Mode == "focused"},
		"recommendation":  "Use this semantic-first packet as the initial coding context. Follow exact definitions/references/callers/callees or targeted line reads only when needed; use search_code mainly for literal strings and unknown text.",
	}
}

func (e *Engine) ContextForTask(ctx context.Context, task string, limit int) (map[string]any, error) {
	limit = boundedTaskLimit(limit)
	evidence := newTaskContextEvidence(e, task)
	evidence.addSemantic(ctx)
	evidence.addTextMatches()
	graphEdges, route := evidence.graphNeighbors()
	return evidence.packet(limit, graphEdges, route), nil
}
