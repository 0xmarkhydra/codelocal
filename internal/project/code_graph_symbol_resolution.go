package project

import (
	"context"
	"strings"
)

func graphQualifiedSymbolNames(symbol map[string]any) []string {
	name := strings.TrimSpace(graphValue(symbol, "name"))
	if name == "" {
		return nil
	}
	detail := strings.TrimSpace(graphValue(symbol, "detail"))
	if detail == "" {
		return []string{name}
	}
	cleanDetail := strings.TrimSpace(strings.TrimPrefix(detail, "*"))
	out := []string{name, detail + "." + name}
	if cleanDetail != "" && cleanDetail != detail {
		out = append(out, cleanDetail+"."+name)
	}
	return out
}

func symbolMatchesQualifiedQuery(symbol map[string]any, query string) bool {
	for _, candidate := range graphQualifiedSymbolNames(symbol) {
		if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(query)) {
			return true
		}
	}
	return false
}

func filterGraphSymbols(symbols []map[string]any, repository string) []map[string]any {
	filtered := make([]map[string]any, 0, len(symbols))
	for _, symbol := range symbols {
		if repository != "" && graphValue(symbol, "repositoryId") != repository && graphValue(symbol, "repositoryPath") != repository {
			continue
		}
		filtered = append(filtered, symbol)
	}
	return filtered
}

func matchingGraphSymbols(symbols []map[string]any, predicate func(map[string]any) bool) []map[string]any {
	out := make([]map[string]any, 0, len(symbols))
	for _, symbol := range symbols {
		if predicate(symbol) {
			out = append(out, symbol)
		}
	}
	return out
}

// chooseGraphSymbol refuses to guess when a search returns multiple plausible
// symbols. A qualified query such as Store.Save may disambiguate receiver
// methods; otherwise the candidates are returned to the UI without fake edges.
func chooseGraphSymbol(symbols []map[string]any, query, repository string) (map[string]any, []map[string]any) {
	query = strings.TrimSpace(query)
	filtered := filterGraphSymbols(symbols, repository)
	if strings.Contains(query, ".") {
		qualified := matchingGraphSymbols(filtered, func(symbol map[string]any) bool { return symbolMatchesQualifiedQuery(symbol, query) })
		if len(qualified) == 1 {
			return qualified[0], nil
		}
		if len(qualified) > 1 {
			return nil, qualified
		}
	}
	exact := matchingGraphSymbols(filtered, func(symbol map[string]any) bool { return strings.EqualFold(graphValue(symbol, "name"), query) })
	if len(exact) == 1 {
		return exact[0], nil
	}
	if len(exact) > 1 {
		return nil, exact
	}
	if len(filtered) == 1 {
		return filtered[0], nil
	}
	if len(filtered) > 1 {
		return nil, filtered
	}
	return nil, nil
}

func graphNodesEquivalent(left, right CodeGraphNode) bool {
	if !strings.EqualFold(strings.TrimSpace(left.Name), strings.TrimSpace(right.Name)) || left.Path != right.Path {
		return false
	}
	return left.Line <= 0 || right.Line <= 0 || left.Line == right.Line
}

func graphNodeCallable(node CodeGraphNode) bool {
	switch strings.ToLower(strings.TrimSpace(node.Kind)) {
	case "type", "interface", "struct", "class", "enum", "module", "package", "file", "external":
		return false
	default:
		return true
	}
}

func refineGraphNode(node CodeGraphNode, relations []codeGraphStructuralRelation, buildNode func(map[string]any) CodeGraphNode) CodeGraphNode {
	for _, relation := range relations {
		for _, raw := range []map[string]any{relation.From, relation.To} {
			candidate := buildNode(raw)
			if graphNodesEquivalent(node, candidate) {
				return mergeRefinedGraphNode(node, candidate)
			}
		}
	}
	return node
}

func mergeRefinedGraphNode(node, candidate CodeGraphNode) CodeGraphNode {
	if node.Kind == "" || node.Kind == "symbol" {
		node.Kind = candidate.Kind
	}
	if node.Provider == "" || strings.Contains(node.Provider, "structure") {
		node.Provider = candidate.Provider
		node.ResolutionMode = candidate.ResolutionMode
		node.Confidence = candidate.Confidence
	}
	return node
}

type codeGraphBuildState struct {
	revision string
	maxNodes int
	nodes    map[string]CodeGraphNode
	order    []string
	edges    []CodeGraphEdge
	edgeSeen map[string]struct{}
}

func newCodeGraphBuildState(revision string, maxNodes int, selected CodeGraphNode) *codeGraphBuildState {
	state := &codeGraphBuildState{revision: revision, maxNodes: maxNodes, nodes: map[string]CodeGraphNode{}, edgeSeen: map[string]struct{}{}}
	state.addNode(selected)
	return state
}

func (s *codeGraphBuildState) addNode(node CodeGraphNode) bool {
	return addGraphNode(s.nodes, &s.order, node, s.maxNodes)
}

func (s *codeGraphBuildState) align(node CodeGraphNode) CodeGraphNode {
	for id, existing := range s.nodes {
		if !graphNodesEquivalent(existing, node) {
			continue
		}
		node.ID, node.Canonical, node.Selected = id, existing.Canonical, existing.Selected
		if existing.Kind != "" && existing.Kind != "symbol" {
			node.Kind = existing.Kind
		}
		return node
	}
	return node
}

func (s *codeGraphBuildState) addEdge(from, to CodeGraphNode, relation string, evidence map[string]any) {
	edge := codeGraphEdge(s.revision, from, to, relation, evidence)
	if _, exists := s.edgeSeen[edge.ID]; exists {
		return
	}
	s.edges = append(s.edges, edge)
	s.edgeSeen[edge.ID] = struct{}{}
}

func (s *codeGraphBuildState) orderedNodes() []CodeGraphNode {
	out := make([]CodeGraphNode, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.nodes[id])
	}
	return out
}

func structuralRelationEndpoints(e *Engine, state *codeGraphBuildState, item CodeGraphNode, relation codeGraphStructuralRelation) (CodeGraphNode, CodeGraphNode, CodeGraphNode, bool) {
	from := state.align(e.codeGraphNode(relation.From))
	to := state.align(e.codeGraphNode(relation.To))
	fromMatches, toMatches := graphNodesEquivalent(item, from), graphNodesEquivalent(item, to)
	if !fromMatches && !toMatches {
		return CodeGraphNode{}, CodeGraphNode{}, CodeGraphNode{}, false
	}
	if fromMatches {
		from.ID = item.ID
		return from, to, to, true
	}
	to.ID = item.ID
	return from, to, from, true
}

func (e *Engine) expandGraphStructuralRelations(selected CodeGraphNode, relations []codeGraphStructuralRelation, depth int, state *codeGraphBuildState) bool {
	if len(relations) == 0 || depth <= 0 {
		return false
	}
	type item struct {
		node  CodeGraphNode
		level int
	}
	frontier := []item{{node: selected}}
	expanded := map[string]struct{}{}
	truncated := false
	for len(frontier) > 0 {
		current := frontier[0]
		frontier = frontier[1:]
		if current.level >= depth || alreadyExpandedAtLevel(expanded, current.node.ID, current.level) {
			continue
		}
		for _, relation := range relations {
			from, to, other, ok := structuralRelationEndpoints(e, state, current.node, relation)
			if !ok {
				continue
			}
			other = state.align(other)
			if !state.addNode(other) {
				truncated = true
				continue
			}
			if graphNodesEquivalent(current.node, from) {
				to = other
			} else {
				from = other
			}
			state.addEdge(from, to, relation.Relation, map[string]any{"provider": relation.Provider, "resolutionMode": relation.ResolutionMode, "confidence": relation.Confidence})
			frontier = append(frontier, item{node: other, level: current.level + 1})
		}
	}
	return truncated
}

func alreadyExpandedAtLevel(expanded map[string]struct{}, id string, level int) bool {
	key := id + "\x00" + string(rune(level+'0'))
	if _, exists := expanded[key]; exists {
		return true
	}
	expanded[key] = struct{}{}
	return false
}

type codeGraphCallFrontier struct {
	node  CodeGraphNode
	level int
}

func (e *Engine) graphCallNeighbors(ctx context.Context, node CodeGraphNode, limit int, incoming bool) []map[string]any {
	if incoming {
		values, _ := e.CallersAt(ctx, node.Path, node.Line, node.Column, node.Name, limit)
		return values
	}
	values, _ := e.CalleesAt(ctx, node.Path, node.Line, node.Column, node.Name, limit)
	return values
}

func (e *Engine) appendGraphCallNeighbors(ctx context.Context, current codeGraphCallFrontier, limit int, incoming bool, state *codeGraphBuildState, frontier *[]codeGraphCallFrontier) bool {
	for _, raw := range e.graphCallNeighbors(ctx, current.node, limit, incoming) {
		node := state.align(e.codeGraphNode(raw))
		if !state.addNode(node) {
			return true
		}
		if incoming {
			state.addEdge(node, current.node, "CALLS", raw)
		} else {
			state.addEdge(current.node, node, "CALLS", raw)
		}
		*frontier = append(*frontier, codeGraphCallFrontier{node: node, level: current.level + 1})
	}
	return false
}

func (e *Engine) expandCodeGraphCalls(ctx context.Context, selected CodeGraphNode, depth int, state *codeGraphBuildState) (int, bool) {
	frontier := []codeGraphCallFrontier{{node: selected}}
	expanded, expansions, truncated := map[string]struct{}{}, 0, false
	for len(frontier) > 0 && len(state.nodes) < state.maxNodes && expansions < maxCodeGraphExpansions {
		current := frontier[0]
		frontier = frontier[1:]
		if current.level >= depth || !graphNodeCallable(current.node) {
			continue
		}
		if _, seen := expanded[current.node.ID]; seen {
			continue
		}
		expanded[current.node.ID], expansions = struct{}{}, expansions+1
		limit := min(18, max(4, (state.maxNodes-len(state.nodes))/2))
		truncated = e.appendGraphCallNeighbors(ctx, current, limit, true, state, &frontier) || truncated
		truncated = e.appendGraphCallNeighbors(ctx, current, limit, false, state, &frontier) || truncated
	}
	return expansions, truncated || expansions >= maxCodeGraphExpansions || len(state.nodes) >= state.maxNodes
}
