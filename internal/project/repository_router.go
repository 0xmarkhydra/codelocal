package project

import (
	"fmt"
	"sort"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/repository"
)

type repositoryRouteScore struct {
	Repository repository.Checkout
	Score      int
	FileScores []int
	Explicit   bool
	Reasons    []string
}

type repositoryRoute struct {
	Mode     string
	Ranked   []repositoryRouteScore
	Selected map[string]struct{}
}

type contextScoredPath struct {
	path  string
	score int
}

func routeRepositoryKey(repo repository.Checkout) string {
	return repo.ID + "\x00" + repo.RelativePath
}

func explicitRepositoryMention(task string, repo repository.Checkout) bool {
	if repo.RelativePath == "." {
		return false
	}
	lower, path := strings.ToLower(task), strings.ToLower(repo.RelativePath)
	if strings.Contains(lower, path) {
		return true
	}
	parts := strings.Split(path, "/")
	base := parts[len(parts)-1]
	if len(base) < 3 {
		return false
	}
	for _, token := range strings.FieldsFunc(lower, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-') }) {
		if token == base {
			return true
		}
	}
	return false
}

func topFileContribution(scores []int, limit int) int {
	sort.Sort(sort.Reverse(sort.IntSlice(scores)))
	if len(scores) > limit {
		scores = scores[:limit]
	}
	total := 0
	for _, score := range scores {
		total += score
	}
	return total
}

func (e *Engine) scoreRepositories(task string, pathScores map[string]int) []repositoryRouteScore {
	byKey := map[string]*repositoryRouteScore{}
	for _, repo := range e.Repositories.All() {
		copy := repositoryRouteScore{Repository: repo}
		byKey[routeRepositoryKey(repo)] = &copy
	}
	for path, score := range pathScores {
		repo, ok := e.repositoryForPath(path)
		if !ok || score <= 0 {
			continue
		}
		if item := byKey[routeRepositoryKey(repo)]; item != nil {
			item.FileScores = append(item.FileScores, score)
		}
	}
	out := make([]repositoryRouteScore, 0, len(byKey))
	for _, item := range byKey {
		item.Score = topFileContribution(append([]int(nil), item.FileScores...), 4)
		if explicitRepositoryMention(task, item.Repository) {
			item.Score += 25
			item.Explicit = true
			item.Reasons = append(item.Reasons, "task-mentions-repository")
		}
		if len(item.FileScores) > 0 {
			item.Reasons = append(item.Reasons, "ranked-file-evidence")
		}
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Repository.RelativePath < out[j].Repository.RelativePath
	})
	return out
}

func repositorySelectionThreshold(top int) int {
	threshold := (top*30 + 99) / 100
	if threshold < 5 {
		threshold = 5
	}
	return threshold
}

func buildRepositoryRoute(ranked []repositoryRouteScore) repositoryRoute {
	route := repositoryRoute{Mode: "all", Ranked: ranked, Selected: map[string]struct{}{}}
	if len(ranked) == 0 {
		return route
	}
	if len(ranked) == 1 || ranked[0].Score < 4 {
		for _, item := range ranked {
			route.Selected[routeRepositoryKey(item.Repository)] = struct{}{}
		}
		return route
	}
	route.Mode = "focused"
	threshold := repositorySelectionThreshold(ranked[0].Score)
	for _, item := range ranked {
		if len(route.Selected) >= 6 || (!item.Explicit && item.Score < threshold) {
			continue
		}
		route.Selected[routeRepositoryKey(item.Repository)] = struct{}{}
	}
	if len(route.Selected) == 0 {
		route.Selected[routeRepositoryKey(ranked[0].Repository)] = struct{}{}
	}
	return route
}

func repositoryFromGraphEdge(e *Engine, edge map[string]any, prefix string) (repository.Checkout, bool) {
	id := strings.TrimSpace(fmt.Sprint(edge[prefix+"RepositoryId"]))
	path := strings.TrimSpace(fmt.Sprint(edge[prefix+"RepositoryPath"]))
	if id == "" || id == "<nil>" {
		return repository.Checkout{}, false
	}
	for _, repo := range e.Repositories.All() {
		if repo.ID == id && (path == "" || path == "<nil>" || repo.RelativePath == path) {
			return repo, true
		}
	}
	return repository.Checkout{}, false
}

func markRouteReason(route *repositoryRoute, repo repository.Checkout, reason string) {
	key := routeRepositoryKey(repo)
	for index := range route.Ranked {
		if routeRepositoryKey(route.Ranked[index].Repository) == key {
			route.Ranked[index].Reasons = append(route.Ranked[index].Reasons, reason)
			return
		}
	}
}

func expandRepositoryRouteByGraph(e *Engine, route *repositoryRoute, edges []map[string]any) {
	if route.Mode != "focused" || len(route.Selected) >= 6 {
		return
	}
	for pass := 0; pass < 2 && len(route.Selected) < 6; pass++ {
		changed := false
		for _, edge := range edges {
			if edge["crossRepository"] != true {
				continue
			}
			from, fromOK := repositoryFromGraphEdge(e, edge, "from")
			to, toOK := repositoryFromGraphEdge(e, edge, "to")
			if !fromOK || !toOK {
				continue
			}
			_, fromSelected := route.Selected[routeRepositoryKey(from)]
			_, toSelected := route.Selected[routeRepositoryKey(to)]
			if fromSelected == toSelected {
				continue
			}
			candidate := from
			if fromSelected {
				candidate = to
			}
			route.Selected[routeRepositoryKey(candidate)] = struct{}{}
			markRouteReason(route, candidate, "cross-repository-graph")
			changed = true
			if len(route.Selected) >= 6 {
				break
			}
		}
		if !changed {
			break
		}
	}
}

func (e *Engine) routeRepositories(task string, pathScores map[string]int, graphEdges []map[string]any) repositoryRoute {
	if e.Repositories == nil {
		return repositoryRoute{Mode: "all", Selected: map[string]struct{}{}}
	}
	route := buildRepositoryRoute(e.scoreRepositories(task, pathScores))
	expandRepositoryRouteByGraph(e, &route, graphEdges)
	return route
}

func (r repositoryRoute) selected(repo repository.Checkout) bool {
	if r.Mode != "focused" {
		return true
	}
	_, ok := r.Selected[routeRepositoryKey(repo)]
	return ok
}

func (r repositoryRoute) allowsPath(e *Engine, path string) bool {
	repo, ok := e.repositoryForPath(path)
	return !ok || r.selected(repo)
}

func (r repositoryRoute) selectedScores() []repositoryRouteScore {
	out := []repositoryRouteScore{}
	for _, item := range r.Ranked {
		if r.selected(item.Repository) {
			out = append(out, item)
		}
	}
	return out
}

func allocateRepositoryBudgets(route repositoryRoute, total int) map[string]int {
	selected := route.selectedScores()
	if route.Mode != "focused" || len(selected) == 0 || total <= 0 {
		return map[string]int{}
	}
	base := total / (len(selected) * 4)
	if base > 6000 {
		base = 6000
	}
	remaining, scoreTotal := total-base*len(selected), 0
	for _, item := range selected {
		scoreTotal += max(1, item.Score)
	}
	out := map[string]int{}
	for _, item := range selected {
		share := 0
		if scoreTotal > 0 {
			share = remaining * max(1, item.Score) / scoreTotal
		}
		out[routeRepositoryKey(item.Repository)] = base + share
	}
	return out
}

func repositorySnippetQuotas(route repositoryRoute, total int) map[string]int {
	selected := route.selectedScores()
	out := map[string]int{}
	if route.Mode != "focused" || len(selected) == 0 {
		return out
	}
	if len(selected) == 1 {
		out[routeRepositoryKey(selected[0].Repository)] = total
		return out
	}
	remaining := total
	for index, item := range selected {
		quota := 2
		if index == 0 {
			quota = max(2, total-2*(len(selected)-1))
		}
		if quota > remaining {
			quota = remaining
		}
		out[routeRepositoryKey(item.Repository)] = quota
		remaining -= quota
	}
	return out
}

func rankContextPaths(e *Engine, scores map[string]int, route repositoryRoute, limit int) []contextScoredPath {
	ranked := make([]contextScoredPath, 0, len(scores))
	for path, score := range scores {
		if score > 0 && route.allowsPath(e, path) {
			ranked = append(ranked, contextScoredPath{path: path, score: score})
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
	return ranked
}

func snippetWindow(center int) (int, int) {
	if center <= 0 {
		center = 1
	}
	start := center - 30
	if start < 1 {
		start = 1
	}
	return start, center + 45
}

func truncateContextContent(content string, globalRemaining, repositoryRemaining int) string {
	limit := min(8000, globalRemaining)
	if repositoryRemaining >= 0 {
		limit = min(limit, repositoryRemaining)
	}
	if limit <= 0 {
		return ""
	}
	if len(content) > limit {
		return content[:limit]
	}
	return content
}

func contextRepositoryLimit(e *Engine, route repositoryRoute, budgets map[string]int, path string) (string, int) {
	repo, ok := e.repositoryForPath(path)
	if !ok || route.Mode != "focused" {
		return "", -1
	}
	key := routeRepositoryKey(repo)
	return key, budgets[key]
}

func contextSnippetAllowed(e *Engine, route repositoryRoute, quotas, counts map[string]int, path string) bool {
	repo, ok := e.repositoryForPath(path)
	if !ok || route.Mode != "focused" {
		return true
	}
	key := routeRepositoryKey(repo)
	return quotas[key] == 0 || counts[key] < quotas[key]
}

func buildRepositoryAwareContext(e *Engine, ranked []contextScoredPath, reasons map[string][]string, preferredLine map[string]int, route repositoryRoute, budget, maxFiles int) ([]map[string]any, []map[string]any, int, map[string]int, map[string]int) {
	files, snippets := []map[string]any{}, []map[string]any{}
	budgets, quotas := allocateRepositoryBudgets(route, budget), repositorySnippetQuotas(route, maxFiles)
	used, snippetFiles := 0, 0
	usedByRepository, snippetsByRepository := map[string]int{}, map[string]int{}
	for _, item := range ranked {
		files = append(files, e.annotatePathMap(map[string]any{"path": item.path, "score": item.score, "reasons": unique(reasons[item.path]), "preferredLine": preferredLine[item.path]}))
		if snippetFiles >= maxFiles || used >= budget || !contextSnippetAllowed(e, route, quotas, snippetsByRepository, item.path) {
			continue
		}
		start, end := snippetWindow(preferredLine[item.path])
		read, err := e.FS.Read(item.path, start, end)
		if err != nil {
			continue
		}
		key, repositoryLimit := contextRepositoryLimit(e, route, budgets, item.path)
		repositoryRemaining := -1
		if repositoryLimit >= 0 {
			repositoryRemaining = repositoryLimit - usedByRepository[key]
		}
		content, _ := read["content"].(string)
		content = truncateContextContent(content, budget-used, repositoryRemaining)
		if content == "" {
			continue
		}
		used, snippetFiles = used+len(content), snippetFiles+1
		if key != "" {
			usedByRepository[key] += len(content)
			snippetsByRepository[key]++
		}
		snippets = append(snippets, e.annotatePathMap(map[string]any{"path": item.path, "startLine": read["startLine"], "endLine": read["endLine"], "totalLines": read["totalLines"], "content": content, "reason": strings.Join(unique(reasons[item.path]), ", ")}))
	}
	return files, snippets, used, budgets, usedByRepository
}

func repositoryRouteMap(route repositoryRoute, budgets, used map[string]int) map[string]any {
	selected := []map[string]any{}
	for _, item := range route.selectedScores() {
		entry := map[string]any{
			"repositoryId": item.Repository.ID, "repositoryPath": item.Repository.RelativePath,
			"score": item.Score, "explicit": item.Explicit, "reasons": item.Reasons,
		}
		if route.Mode == "focused" {
			key := routeRepositoryKey(item.Repository)
			entry["maxChars"], entry["usedChars"] = budgets[key], used[key]
		}
		selected = append(selected, entry)
	}
	return map[string]any{"mode": route.Mode, "totalRepositories": len(route.Ranked), "selectedCount": len(selected), "selectedRepositories": selected, "excludedCount": max(0, len(route.Ranked)-len(selected))}
}

func filterPathMapsForRoute(e *Engine, values []map[string]any, route repositoryRoute) []map[string]any {
	if route.Mode != "focused" {
		return values
	}
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		path := strings.TrimSpace(fmt.Sprint(value["path"]))
		if path == "" || path == "<nil>" || route.allowsPath(e, path) {
			out = append(out, value)
		}
	}
	return out
}

func graphEndpointSelected(route repositoryRoute, edge map[string]any, prefix string) (bool, bool) {
	id := strings.TrimSpace(fmt.Sprint(edge[prefix+"RepositoryId"]))
	path := strings.TrimSpace(fmt.Sprint(edge[prefix+"RepositoryPath"]))
	if id == "" || id == "<nil>" || path == "" || path == "<nil>" {
		return false, false
	}
	_, ok := route.Selected[id+"\x00"+path]
	return ok, true
}

func filterGraphEdgesForRoute(e *Engine, edges []map[string]any, route repositoryRoute) []map[string]any {
	if route.Mode != "focused" {
		return edges
	}
	out := make([]map[string]any, 0, len(edges))
	for _, edge := range edges {
		fromSelected, fromKnown := graphEndpointSelected(route, edge, "from")
		toSelected, toKnown := graphEndpointSelected(route, edge, "to")
		if fromKnown || toKnown {
			if fromSelected || toSelected {
				out = append(out, edge)
			}
			continue
		}
		if route.allowsPath(e, fmt.Sprint(edge["from"])) {
			out = append(out, edge)
		}
	}
	return out
}
