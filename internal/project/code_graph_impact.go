package project

import "strings"

type CodeGraphImpact struct {
	Scope             string  `json:"scope"`
	DirectCallers     int     `json:"directCallers"`
	DirectCallees     int     `json:"directCallees"`
	PotentialCallers  int     `json:"potentialCallers"`
	AffectedFiles     int     `json:"affectedFiles"`
	EvidenceEdges     int     `json:"evidenceEdges"`
	SemanticEdges     int     `json:"semanticEdges"`
	AverageConfidence float64 `json:"averageConfidence,omitempty"`
	Risk              string  `json:"risk"`
	Truncated         bool    `json:"truncated,omitempty"`
}

func impactRisk(callers, files int, truncated bool) string {
	if truncated || callers >= 12 || files >= 8 {
		return "high"
	}
	if callers >= 4 || files >= 3 {
		return "medium"
	}
	return "low"
}

func codeGraphImpact(selectedID string, nodes []CodeGraphNode, edges []CodeGraphEdge, truncated bool) *CodeGraphImpact {
	if strings.TrimSpace(selectedID) == "" {
		return nil
	}
	byID := make(map[string]CodeGraphNode, len(nodes))
	incoming := make(map[string][]CodeGraphEdge)
	for _, node := range nodes {
		byID[node.ID] = node
	}
	for _, edge := range edges {
		if edge.Relation == "CALLS" {
			incoming[edge.To] = append(incoming[edge.To], edge)
		}
	}
	result := &CodeGraphImpact{Scope: "bounded-neighborhood", Truncated: truncated}
	for _, edge := range edges {
		if edge.Relation != "CALLS" {
			continue
		}
		if edge.To == selectedID {
			result.DirectCallers++
		}
		if edge.From == selectedID {
			result.DirectCallees++
		}
	}
	seen := map[string]struct{}{selectedID: {}}
	queue := []string{selectedID}
	files := map[string]struct{}{}
	confidenceTotal := 0.0
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range incoming[current] {
			result.EvidenceEdges++
			confidenceTotal += edge.Confidence
			if edge.ResolutionMode == "lsp" || strings.Contains(strings.ToLower(edge.Provider), "gopls") || strings.Contains(strings.ToLower(edge.Provider), "language") {
				result.SemanticEdges++
			}
			if _, exists := seen[edge.From]; exists {
				continue
			}
			seen[edge.From] = struct{}{}
			queue = append(queue, edge.From)
			if node, ok := byID[edge.From]; ok && node.Path != "" {
				files[node.Path] = struct{}{}
			}
		}
	}
	result.PotentialCallers = max(0, len(seen)-1)
	result.AffectedFiles = len(files)
	if result.EvidenceEdges > 0 {
		result.AverageConfidence = confidenceTotal / float64(result.EvidenceEdges)
	}
	result.Risk = impactRisk(result.PotentialCallers, result.AffectedFiles, truncated)
	return result
}
