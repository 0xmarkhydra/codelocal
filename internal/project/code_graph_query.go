package project

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

const (
	defaultCodeGraphNodes  = 120
	maxCodeGraphNodes      = 300
	maxCodeGraphDepth      = 3
	maxCodeGraphExpansions = 18
)

type CodeGraphNode struct {
	ID                     string  `json:"id"`
	Kind                   string  `json:"kind"`
	Name                   string  `json:"name"`
	Summary                string  `json:"summary,omitempty"`
	Path                   string  `json:"path,omitempty"`
	Line                   int     `json:"line,omitempty"`
	Column                 int     `json:"column,omitempty"`
	RepositoryID           string  `json:"repositoryId,omitempty"`
	RepositoryPath         string  `json:"repositoryPath,omitempty"`
	RepositoryRelativePath string  `json:"repositoryRelativePath,omitempty"`
	QualifiedName          string  `json:"qualifiedName,omitempty"`
	Provider               string  `json:"provider,omitempty"`
	ResolutionMode         string  `json:"resolutionMode,omitempty"`
	FallbackReason         string  `json:"fallbackReason,omitempty"`
	Confidence             float64 `json:"confidence,omitempty"`
	Canonical              bool    `json:"canonical"`
	Selected               bool    `json:"selected,omitempty"`
}

type CodeGraphEdge struct {
	ID             string  `json:"id"`
	From           string  `json:"from"`
	To             string  `json:"to"`
	Relation       string  `json:"relation"`
	Provider       string  `json:"provider,omitempty"`
	ResolutionMode string  `json:"resolutionMode,omitempty"`
	FallbackReason string  `json:"fallbackReason,omitempty"`
	Confidence     float64 `json:"confidence,omitempty"`
	Count          int     `json:"count,omitempty"`
}

type CodeGraphView struct {
	Status      string              `json:"status"`
	View        string              `json:"view,omitempty"`
	Query       string              `json:"query,omitempty"`
	Depth       int                 `json:"depth"`
	MaxNodes    int                 `json:"maxNodes"`
	SelectedID  string              `json:"selectedId,omitempty"`
	Snapshots   []CodeGraphSnapshot `json:"snapshots"`
	Snapshot    *CodeGraphSnapshot  `json:"snapshot,omitempty"`
	Nodes       []CodeGraphNode     `json:"nodes"`
	Edges       []CodeGraphEdge     `json:"edges"`
	Impact      *CodeGraphImpact    `json:"impact,omitempty"`
	Truncated   bool                `json:"truncated,omitempty"`
	GeneratedBy string              `json:"generatedBy"`
}

func boundedCodeGraphArgs(depth, maxNodes int) (int, int) {
	if depth < 1 {
		depth = 1
	}
	if depth > maxCodeGraphDepth {
		depth = maxCodeGraphDepth
	}
	if maxNodes <= 0 {
		maxNodes = defaultCodeGraphNodes
	}
	if maxNodes > maxCodeGraphNodes {
		maxNodes = maxCodeGraphNodes
	}
	return depth, maxNodes
}

func codeGraphHash(parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(hash[:12])
}

func graphValue(value map[string]any, key string) string {
	text := strings.TrimSpace(fmt.Sprint(value[key]))
	if text == "<nil>" {
		return ""
	}
	return text
}

func graphConfidence(value map[string]any, fallback float64) float64 {
	switch typed := value["confidence"].(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	default:
		return fallback
	}
}

func (e *Engine) codeGraphNode(value map[string]any) CodeGraphNode {
	e.annotateCanonicalSymbol(value)
	path := e.workspacePath(graphValue(value, "path"))
	name := graphValue(value, "name")
	kind := graphValue(value, "symbolKind")
	if kind == "" {
		kind = graphValue(value, "kind")
	}
	if kind == "" || kind == "symbol" {
		kind = "symbol"
	}
	if name == "" && path != "" {
		name = path
		if line := intValue(value["line"]); line > 0 {
			name += ":" + fmt.Sprint(line)
		}
		kind = "callsite"
	}
	repositoryID := graphValue(value, "repositoryId")
	repositoryPath := graphValue(value, "repositoryPath")
	if repositoryID == "" && path != "" {
		repositoryID, repositoryPath = e.repositoryIdentity(path)
	}
	id := graphValue(value, "symbolId")
	canonical := id != ""
	if id == "" {
		id = "local_" + codeGraphHash(repositoryID, path, name, fmt.Sprint(intValue(value["line"])), fmt.Sprint(intValue(value["column"])))
	}
	return CodeGraphNode{
		ID: id, Kind: kind, Name: name, Summary: graphValue(value, "detail"), Path: path,
		Line: intValue(value["line"]), Column: intValue(value["column"]), RepositoryID: repositoryID,
		RepositoryPath: repositoryPath, RepositoryRelativePath: graphValue(value, "repositoryRelativePath"),
		QualifiedName: graphValue(value, "qualifiedName"), Provider: graphValue(value, "provider"),
		ResolutionMode: graphValue(value, "resolutionMode"), FallbackReason: graphValue(value, "fallbackReason"),
		Confidence: graphConfidence(value, .5), Canonical: canonical,
	}
}

func codeGraphEdge(revision string, from, to CodeGraphNode, relation string, evidence map[string]any) CodeGraphEdge {
	provider := graphValue(evidence, "provider")
	mode := graphValue(evidence, "resolutionMode")
	confidence := graphConfidence(evidence, .5)
	return CodeGraphEdge{
		ID: "edge_" + codeGraphHash(revision, from.ID, to.ID, relation, provider, mode), From: from.ID, To: to.ID,
		Relation: relation, Provider: provider, ResolutionMode: mode, FallbackReason: graphValue(evidence, "fallbackReason"), Confidence: confidence,
	}
}

func snapshotForRepository(snapshots []CodeGraphSnapshot, repository string) *CodeGraphSnapshot {
	if len(snapshots) == 0 {
		return nil
	}
	repository = strings.TrimSpace(repository)
	if repository == "" {
		copy := snapshots[0]
		return &copy
	}
	for _, snapshot := range snapshots {
		if snapshot.RepositoryID == repository || snapshot.RepositoryPath == repository {
			copy := snapshot
			return &copy
		}
	}
	return nil
}

func snapshotForPath(snapshots []CodeGraphSnapshot, e *Engine, path string) *CodeGraphSnapshot {
	repo, ok := e.repositoryForPath(path)
	if !ok {
		return nil
	}
	return snapshotForRepository(snapshots, repo.ID)
}

func addGraphNode(nodes map[string]CodeGraphNode, order *[]string, node CodeGraphNode, maxNodes int) bool {
	if node.ID == "" || node.Name == "" {
		return false
	}
	if _, exists := nodes[node.ID]; exists {
		return true
	}
	if len(nodes) >= maxNodes {
		return false
	}
	nodes[node.ID] = node
	*order = append(*order, node.ID)
	return true
}

func (e *Engine) ambiguousCodeGraphView(query string, candidates []map[string]any, depth, maxNodes int, snapshots []CodeGraphSnapshot) CodeGraphView {
	nodes := make([]CodeGraphNode, 0, min(len(candidates), maxNodes))
	for _, candidate := range candidates {
		node := e.codeGraphNode(candidate)
		node.FallbackReason = "ambiguous_query"
		nodes = append(nodes, node)
		if len(nodes) >= maxNodes {
			break
		}
	}
	return CodeGraphView{
		Status: "ambiguous", View: "symbols", Query: query, Depth: depth, MaxNodes: maxNodes, Snapshots: snapshots,
		Nodes: nodes, Edges: []CodeGraphEdge{}, Truncated: len(candidates) > len(nodes), GeneratedBy: "local-runtime",
	}
}

func (e *Engine) codeGraphSymbolView(ctx context.Context, query, repository string, depth, maxNodes int, snapshots []CodeGraphSnapshot) (CodeGraphView, error) {
	symbols, err := e.WorkspaceSymbols(ctx, query, 40)
	if err != nil {
		return CodeGraphView{}, err
	}
	selectedValue, ambiguous := chooseGraphSymbol(symbols, query, repository)
	if selectedValue == nil {
		if len(ambiguous) > 0 {
			return e.ambiguousCodeGraphView(query, ambiguous, depth, maxNodes, snapshots), nil
		}
		return CodeGraphView{Status: "empty", View: "symbols", Query: query, Depth: depth, MaxNodes: maxNodes, Snapshots: snapshots, Nodes: []CodeGraphNode{}, Edges: []CodeGraphEdge{}, GeneratedBy: "local-runtime"}, nil
	}
	selected := e.codeGraphNode(selectedValue)
	typeRelations := e.goTypeRelationsForPath(selected.Path)
	selected = refineGraphNode(selected, typeRelations, e.codeGraphNode)
	selected.Selected = true
	snapshot := snapshotForPath(snapshots, e, selected.Path)
	if snapshot == nil {
		snapshot = snapshotForRepository(snapshots, repository)
	}
	revision := ""
	if snapshot != nil {
		revision = snapshot.Revision
	}

	state := newCodeGraphBuildState(revision, maxNodes, selected)
	structuralTruncated := e.expandGraphStructuralRelations(selected, typeRelations, depth, state)
	_, callTruncated := e.expandCodeGraphCalls(ctx, selected, depth, state)
	out := state.orderedNodes()
	truncated := structuralTruncated || callTruncated
	return CodeGraphView{
		Status: "current", View: "symbols", Query: query, Depth: depth, MaxNodes: maxNodes, SelectedID: selected.ID,
		Snapshots: snapshots, Snapshot: snapshot, Nodes: out, Edges: state.edges,
		Impact: codeGraphImpact(selected.ID, out, state.edges, truncated), Truncated: truncated, GeneratedBy: "local-runtime",
	}, nil
}

func (e *Engine) codeGraphOverview(repository string, depth, maxNodes int, snapshots []CodeGraphSnapshot) (CodeGraphView, error) {
	snapshot := snapshotForRepository(snapshots, repository)
	if snapshot == nil && strings.TrimSpace(repository) != "" {
		return CodeGraphView{Status: "empty", View: "files", Depth: depth, MaxNodes: maxNodes, Snapshots: snapshots, Nodes: []CodeGraphNode{}, Edges: []CodeGraphEdge{}, GeneratedBy: "local-runtime"}, nil
	}
	revision := ""
	if snapshot != nil {
		revision = snapshot.Revision
	}
	rawEdges, err := e.ImportGraph(maxNodes * 4)
	if err != nil {
		return CodeGraphView{}, err
	}
	nodes := map[string]CodeGraphNode{}
	order := []string{}
	edges := []CodeGraphEdge{}
	for _, raw := range rawEdges {
		fromPath := e.workspacePath(graphValue(raw, "from"))
		fromRepo := graphValue(raw, "fromRepositoryId")
		fromRepoPath := graphValue(raw, "fromRepositoryPath")
		if snapshot != nil && fromRepo != snapshot.RepositoryID && fromRepoPath != snapshot.RepositoryPath {
			continue
		}
		toPath := e.workspacePath(graphValue(raw, "to"))
		toRepo := graphValue(raw, "toRepositoryId")
		toRepoPath := graphValue(raw, "toRepositoryPath")
		from := CodeGraphNode{ID: "file_" + codeGraphHash(fromRepo, fromPath), Kind: "file", Name: fromPath, Path: fromPath, RepositoryID: fromRepo, RepositoryPath: fromRepoPath, Provider: "structural-index", ResolutionMode: "structural", Confidence: .9}
		toKind := "external"
		toName := graphValue(raw, "specifier")
		if toRepo != "" {
			toKind, toName = "file", toPath
		}
		to := CodeGraphNode{ID: "file_" + codeGraphHash(toRepo, toName), Kind: toKind, Name: toName, Path: toPath, RepositoryID: toRepo, RepositoryPath: toRepoPath, Provider: "structural-index", ResolutionMode: "structural", Confidence: .8}
		if !addGraphNode(nodes, &order, from, maxNodes) || !addGraphNode(nodes, &order, to, maxNodes) {
			break
		}
		evidence := map[string]any{"provider": "structural-index", "resolutionMode": "structural", "confidence": .8}
		edges = append(edges, codeGraphEdge(revision, from, to, "IMPORTS", evidence))
		if len(edges) >= maxNodes*2 {
			break
		}
	}
	out := make([]CodeGraphNode, 0, len(order))
	for _, id := range order {
		out = append(out, nodes[id])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	status := "current"
	if len(out) == 0 {
		status = "empty"
	}
	return CodeGraphView{Status: status, View: "files", Depth: depth, MaxNodes: maxNodes, Snapshots: snapshots, Snapshot: snapshot, Nodes: out, Edges: edges, Truncated: len(nodes) >= maxNodes, GeneratedBy: "local-runtime"}, nil
}

func (e *Engine) CodeGraphNeighborhoodView(ctx context.Context, query, repository, view string, depth, maxNodes int) (CodeGraphView, error) {
	depth, maxNodes = boundedCodeGraphArgs(depth, maxNodes)
	snapshots, err := e.CodeGraphSnapshots()
	if err != nil {
		return CodeGraphView{}, err
	}
	if strings.TrimSpace(query) != "" {
		return e.codeGraphSymbolView(ctx, query, repository, depth, maxNodes, snapshots)
	}
	if codeGraphViewMode(view) == "files" {
		return e.codeGraphOverview(repository, depth, maxNodes, snapshots)
	}
	return e.codeGraphArchitecture(repository, depth, maxNodes, snapshots)
}

func (e *Engine) CodeGraphNeighborhood(ctx context.Context, query, repository string, depth, maxNodes int) (CodeGraphView, error) {
	return e.CodeGraphNeighborhoodView(ctx, query, repository, "architecture", depth, maxNodes)
}
