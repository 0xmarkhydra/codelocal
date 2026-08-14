package orchestration

import "testing"

func TestEvaluateExecutionEfficiencyRewardsStructuredBoundedFlow(t *testing.T) {
	plan := AgentPlan{Route: Decision{Primary: LaneCode}}
	trace := []ExecutionTraceStep{
		{Operation: "context.task", Lane: LaneCode, Status: "succeeded"},
		{Operation: "edit.apply", Lane: LaneCode, Status: "succeeded"},
		{Operation: "verify.changes", Lane: LaneCode, Status: "succeeded", Verification: true},
		{Operation: "terminal.run", Lane: LaneShell, Status: "succeeded", Verification: true},
	}
	efficiency := EvaluateExecutionEfficiency(plan, trace)
	if efficiency.Score < 90 || efficiency.Grade != "excellent" {
		t.Fatalf("expected efficient bounded flow, got %#v", efficiency)
	}
	if efficiency.ModelRoundTripsAvoided != 3 {
		t.Fatalf("expected three avoided model round trips, got %#v", efficiency)
	}
}

func TestEvaluateExecutionEfficiencyPenalizesRepeatedFallbacks(t *testing.T) {
	plan := AgentPlan{Route: Decision{Primary: LaneCode}}
	trace := []ExecutionTraceStep{
		{Operation: "context.task", Lane: LaneCode, Status: "succeeded"},
		{Operation: "computer.observe", Lane: LaneComputer, Status: "succeeded", Fallback: true},
		{Operation: "computer.observe", Lane: LaneComputer, Status: "succeeded", Fallback: true},
		{Operation: "browser.snapshot", Lane: LaneBrowser, Status: "succeeded", Fallback: true},
		{Operation: "computer.observe", Lane: LaneComputer, Status: "succeeded", Fallback: true},
	}
	efficiency := EvaluateExecutionEfficiency(plan, trace)
	if efficiency.Score >= 80 || efficiency.RepeatedOperations == 0 || efficiency.FallbackSteps != 4 {
		t.Fatalf("expected fallback/repetition penalties, got %#v", efficiency)
	}
}

func TestSummarizeExecutionTraceKeepsCompactDecisionEvidence(t *testing.T) {
	trace := []ExecutionTraceStep{
		{Operation: "context.task", Status: "succeeded", DurationMS: 4},
		{Operation: "edit.apply", Status: "succeeded", DurationMS: 6, Replanned: true},
		{Operation: "verify.changes", Status: "skipped", DurationMS: 1, Verification: true},
		{Operation: "terminal.run", Status: "halted", DurationMS: 9, Verification: true, Fallback: true},
	}
	summary := SummarizeExecutionTrace(trace)
	if summary.Operations != 4 || summary.Succeeded != 2 || summary.Skipped != 1 || summary.Halted != 1 {
		t.Fatalf("unexpected status summary: %#v", summary)
	}
	if summary.Verifications != 2 || summary.Replans != 1 || summary.Fallbacks != 1 || summary.DurationMS != 20 || summary.LastOperation != "terminal.run" {
		t.Fatalf("unexpected trace summary evidence: %#v", summary)
	}
}

func TestLaneForOperation(t *testing.T) {
	cases := map[string]Lane{
		"edit.apply":       LaneCode,
		"terminal.run":     LaneShell,
		"browser.snapshot": LaneBrowser,
		"computer.observe": LaneComputer,
	}
	for operation, want := range cases {
		if got := LaneForOperation(operation); got != want {
			t.Fatalf("LaneForOperation(%q)=%q want %q", operation, got, want)
		}
	}
}
