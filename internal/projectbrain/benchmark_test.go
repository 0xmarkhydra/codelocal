package projectbrain

import "testing"

func TestCompareBenchmarkMeasuresEfficiencyWithoutHidingRegression(t *testing.T) {
	scenario := BenchmarkScenario{ID: "repeat-task", Name: "repeat"}
	baseline := BenchmarkResult{Scenario: scenario, Variant: "plain-agent", Metrics: BenchmarkMetrics{VerifiedSuccess: true, ContextBytes: 10000, ToolCalls: 20, FileReads: 10, RoundTrips: 12, CompletionMS: 10000, RuleAccuracy: .9, UnrelatedFilesTouched: 0}}
	candidate := BenchmarkResult{Scenario: scenario, Variant: "codelocal", Metrics: BenchmarkMetrics{VerifiedSuccess: true, ContextBytes: 5000, ToolCalls: 10, FileReads: 5, RoundTrips: 6, CompletionMS: 8000, RuleAccuracy: .95, UnrelatedFilesTouched: 0}}
	comparison := CompareBenchmark(baseline, candidate)
	if comparison.ContextReduction != .5 || comparison.ToolCallReduction != .5 || comparison.RoundTripReduction != .5 || comparison.CompletionTimeReduction != .2 || comparison.CandidateRegressed {
		t.Fatalf("unexpected comparison: %#v", comparison)
	}
	candidate.Metrics.VerifiedSuccess = false
	if degraded := CompareBenchmark(baseline, candidate); !degraded.CandidateRegressed || degraded.VerifiedSuccessDelta != -1 {
		t.Fatalf("verified-success regression must dominate efficiency gains: %#v", degraded)
	}
}

func TestDefaultBenchmarkScenariosCoverCriticalProjectBrainRisks(t *testing.T) {
	seen := map[string]bool{}
	for _, scenario := range DefaultBenchmarkScenarios() {
		seen[scenario.ID] = true
	}
	for _, required := range []string{"repeat-task", "new-machine", "monorepo-scope", "rule-conflict", "branch-applicability", "skill-invalidation", "malicious-config"} {
		if !seen[required] {
			t.Fatalf("missing benchmark scenario %q", required)
		}
	}
}
