package project

import "testing"

func TestCodeGraphImpactCountsReverseCallerReachability(t *testing.T) {
	nodes := []CodeGraphNode{
		{ID: "target", Path: "target.go"},
		{ID: "caller-a", Path: "a.go"},
		{ID: "caller-b", Path: "b.go"},
		{ID: "root", Path: "root.go"},
		{ID: "callee", Path: "callee.go"},
	}
	edges := []CodeGraphEdge{
		{ID: "e1", From: "caller-a", To: "target", Relation: "CALLS", Provider: "gopls", ResolutionMode: "lsp", Confidence: 1},
		{ID: "e2", From: "caller-b", To: "target", Relation: "CALLS", Provider: "gopls", ResolutionMode: "lsp", Confidence: .9},
		{ID: "e3", From: "root", To: "caller-a", Relation: "CALLS", Provider: "go-ast", ResolutionMode: "ast", Confidence: .75},
		{ID: "e4", From: "target", To: "callee", Relation: "CALLS", Provider: "gopls", ResolutionMode: "lsp", Confidence: 1},
	}
	impact := codeGraphImpact("target", nodes, edges, false)
	if impact == nil {
		t.Fatal("expected impact")
	}
	if impact.DirectCallers != 2 || impact.DirectCallees != 1 || impact.PotentialCallers != 3 {
		t.Fatalf("unexpected caller counts: %#v", impact)
	}
	if impact.AffectedFiles != 3 || impact.EvidenceEdges != 3 || impact.SemanticEdges != 2 {
		t.Fatalf("unexpected evidence summary: %#v", impact)
	}
	if impact.Risk != "medium" {
		t.Fatalf("risk=%q want medium", impact.Risk)
	}
	if impact.AverageConfidence <= .8 || impact.AverageConfidence > 1 {
		t.Fatalf("unexpected confidence: %#v", impact)
	}
}

func TestCodeGraphImpactMarksTruncatedSliceHighRisk(t *testing.T) {
	impact := codeGraphImpact("target", []CodeGraphNode{{ID: "target"}}, nil, true)
	if impact == nil || impact.Risk != "high" || !impact.Truncated {
		t.Fatalf("truncated impact must remain conservative: %#v", impact)
	}
}

func TestCodeGraphImpactRequiresSelectedSymbol(t *testing.T) {
	if impact := codeGraphImpact("", nil, nil, false); impact != nil {
		t.Fatalf("overview must not invent symbol impact: %#v", impact)
	}
}
