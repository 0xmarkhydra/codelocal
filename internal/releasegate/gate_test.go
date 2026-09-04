package releasegate

import "testing"

func healthyInfra() Infrastructure {
	return Infrastructure{CIStarted: true, CIHealthy: true, CrossPlatform: true, MigrationReady: true, RollbackReady: true}
}

func TestReleaseGatePassesOnlyWithHealthyEvidence(t *testing.T) {
	baseline := Metrics{VerifiedSuccessRate: .9, MedianTokensPerVerifiedTask: 10000, P95LatencyMillis: 1000, ResumeSuccessRate: .99}
	candidate := Metrics{VerifiedSuccessRate: .92, MedianTokensPerVerifiedTask: 8000, P95LatencyMillis: 1100, ResumeSuccessRate: .995}
	chaos := []ChaosResult{{ID: "runtime_restart", Passed: true}, {ID: "stale_patch", Passed: true}, {ID: "dirty_checkout", Passed: true}}
	gate := Evaluate(baseline, candidate, healthyInfra(), chaos, Config{})
	if !gate.Passed || len(gate.Blockers) != 0 {
		t.Fatalf("healthy release blocked: %+v", gate)
	}
}

func TestReleaseGateNeverTradesCorrectnessForTokens(t *testing.T) {
	baseline := Metrics{VerifiedSuccessRate: .9, MedianTokensPerVerifiedTask: 10000, ResumeSuccessRate: .99}
	candidate := Metrics{VerifiedSuccessRate: .8, MedianTokensPerVerifiedTask: 1000, ResumeSuccessRate: 1}
	gate := Evaluate(baseline, candidate, healthyInfra(), []ChaosResult{{ID: "restart", Passed: true}}, Config{})
	if gate.Passed {
		t.Fatalf("token win hid correctness regression: %+v", gate)
	}
}

func TestReleaseGateBlocksConcurrencySecurityAndBrokenCI(t *testing.T) {
	baseline := Metrics{VerifiedSuccessRate: .9, ResumeSuccessRate: .99}
	candidate := Metrics{VerifiedSuccessRate: .95, ResumeSuccessRate: 1, LostUpdates: 1, SecurityRegressions: 1, PolicyBypasses: 1}
	gate := Evaluate(baseline, candidate, Infrastructure{CIStarted: false}, []ChaosResult{{ID: "crash_recovery", Passed: false}}, Config{})
	if gate.Passed || len(gate.Blockers) < 5 {
		t.Fatalf("hard blockers ignored: %+v", gate)
	}
}

func TestReleaseGateRequiresResumeReliability(t *testing.T) {
	baseline := Metrics{VerifiedSuccessRate: .9, ResumeSuccessRate: .99}
	candidate := Metrics{VerifiedSuccessRate: .92, ResumeSuccessRate: .8}
	gate := Evaluate(baseline, candidate, healthyInfra(), []ChaosResult{{ID: "restart", Passed: true}}, Config{})
	if gate.Passed {
		t.Fatalf("poor resume reliability passed: %+v", gate)
	}
}
