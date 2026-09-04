package orchestration

import "testing"

func TestSummarizeVerificationLanes(t *testing.T) {
	requirements := []VerificationRequirement{
		{ID: "unit-1", Category: "unit", Required: true},
		{ID: "browser-1", Category: "browser", Required: true},
		{ID: "browser-2", Category: "browser", Required: false},
	}
	results := []VerificationEvidence{
		{CheckID: "unit-1", Status: VerificationPassed},
		{CheckID: "browser-1", Status: VerificationFailed},
		{CheckID: "browser-1", Status: VerificationPassed},
		{CheckID: "browser-2", Status: VerificationFailed},
		{CheckID: "ghost", Status: VerificationPassed},
	}
	graph := SummarizeVerification(requirements, results)
	if len(graph.Lanes) != 2 || graph.Lanes[0].Lane != "browser" || graph.Lanes[1].Lane != "unit" {
		t.Fatalf("lane mismatch: %+v", graph.Lanes)
	}
	if !graph.Ready {
		t.Fatalf("graph should be ready: %+v", graph)
	}
	browser := graph.Lanes[0]
	if !browser.Ready || len(browser.Failed) != 1 || browser.Failed[0] != "browser-2" {
		t.Fatalf("optional failure must not block lane: %+v", browser)
	}
}

func TestSummarizeVerificationBlocksOnRequired(t *testing.T) {
	requirements := []VerificationRequirement{{ID: "unit-1", Required: true}}
	if graph := SummarizeVerification(requirements, nil); graph.Ready {
		t.Fatal("pending required check reported ready")
	}
	if graph := SummarizeVerification(nil, nil); graph.Ready || len(graph.Lanes) != 0 {
		t.Fatal("empty requirements reported ready")
	}
	results := []VerificationEvidence{{CheckID: "unit-1", Status: VerificationFailed}}
	if graph := SummarizeVerification(requirements, results); graph.Ready {
		t.Fatal("failed required check reported ready")
	}
}
