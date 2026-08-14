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
