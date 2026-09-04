package projectbrain

import "testing"

func TestExperiencePromotionGate(t *testing.T) {
	candidate := ExperienceCandidate{Kind: ExperienceWorkflow, Statement: "Run package tests before product verification.", Confidence: .9, EvidenceRefs: []string{"event:1"}, VerificationRefs: []string{"verify:1"}}
	verified := VerifiedOutcome{VerificationPassed: true, SecurityPassed: true, RegressionFree: true, RequiredChecks: 2, PassedRequiredChecks: 2}
	decision, err := EvaluateExperience(candidate, verified)
	if err != nil || decision.Status != ExperiencePromoted || decision.Trust != "verified" {
		t.Fatalf("verified experience not promoted: %+v err=%v", decision, err)
	}
	verified.PassedRequiredChecks = 1
	decision, err = EvaluateExperience(candidate, verified)
	if err != nil || decision.Status != ExperienceRejected {
		t.Fatalf("unverified experience escaped gate: %+v err=%v", decision, err)
	}
}

func TestFailureRecoveryExperienceRequiresPair(t *testing.T) {
	candidate := ExperienceCandidate{Kind: ExperienceFailureRecovery, Statement: "Reconcile stale patch against latest checkout.", Confidence: .95, EvidenceRefs: []string{"failure:1"}, VerificationRefs: []string{"verify:1"}}
	outcome := VerifiedOutcome{VerificationPassed: true, SecurityPassed: true, RegressionFree: true, RequiredChecks: 1, PassedRequiredChecks: 1}
	decision, err := EvaluateExperience(candidate, outcome)
	if err != nil || decision.Status != ExperienceRejected {
		t.Fatalf("incomplete recovery pair passed: %+v err=%v", decision, err)
	}
	candidate.FailureSignature = "failure-signature"
	candidate.RecoveryAction = "reconcile_patch"
	decision, err = EvaluateExperience(candidate, outcome)
	if err != nil || decision.Status != ExperiencePromoted {
		t.Fatalf("verified recovery pair did not promote: %+v err=%v", decision, err)
	}
}

func TestSelectExperiencesScope(t *testing.T) {
	decisions := []ExperienceDecision{
		{Status: ExperiencePromoted, Trust: "verified", Candidate: ExperienceCandidate{ID: "a", Kind: ExperienceFact, Statement: "A", BranchScope: "main", Trigger: "login", Confidence: .95}},
		{Status: ExperiencePromoted, Trust: "verified", Candidate: ExperienceCandidate{ID: "b", Kind: ExperienceFact, Statement: "B", BranchScope: "other", Trigger: "login", Confidence: .99}},
	}
	selected := SelectExperiences(decisions, "main", "login flow", 1)
	if len(selected) != 1 || selected[0].ID != "a" {
		t.Fatalf("wrong experience selected: %+v", selected)
	}
}
