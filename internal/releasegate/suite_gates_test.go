package releasegate

import "testing"

func suiteSamples(n int, verified bool) []TaskSample {
	samples := make([]TaskSample, 0, n)
	for i := 0; i < n; i++ {
		samples = append(samples, TaskSample{Tokens: 100, LatencyMillis: 50, Verified: verified, ResumeAttempted: true, ResumeSucceeded: true})
	}
	return samples
}

func TestEvaluateSuiteGuards(t *testing.T) {
	infra := Infrastructure{CIStarted: true, CIHealthy: true, CrossPlatform: true, MigrationReady: true, RollbackReady: true}
	if _, err := EvaluateSuite("nope", suiteSamples(5, true), suiteSamples(5, true), infra, nil, Config{}); err == nil {
		t.Fatal("unknown suite accepted")
	}
	if _, err := EvaluateSuite(SuiteSingleAgent, suiteSamples(2, true), suiteSamples(5, true), infra, nil, Config{}); err == nil {
		t.Fatal("undersized suite accepted")
	}
	report, err := EvaluateSuite(SuiteMultiAgent, suiteSamples(5, true), suiteSamples(5, true), infra, nil, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Suite != SuiteMultiAgent || !report.Report.Gate.Passed {
		t.Fatalf("suite report mismatch: %+v", report)
	}
}
