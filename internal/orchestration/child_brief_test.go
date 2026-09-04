package orchestration

import "testing"

func validBrief() ChildBrief {
	return ChildBrief{ID: "brief-1", TaskID: "task", ParentAgentID: "lead", Role: SpecialistImplementer, Objective: "implement retry", WritePaths: []string{"internal/"}, TokenBudget: 1000, Verification: []string{"go-test"}}
}

func TestIssueChildBriefValidates(t *testing.T) {
	if _, err := IssueChildBrief(validBrief()); err != nil {
		t.Fatal(err)
	}
	mutants := []func(*ChildBrief){
		func(b *ChildBrief) { b.Objective = "" },
		func(b *ChildBrief) { b.TokenBudget = 0 },
		func(b *ChildBrief) { b.Role = "ghost" },
		func(b *ChildBrief) { b.TaskID = "" },
	}
	for i, mutate := range mutants {
		brief := validBrief()
		mutate(&brief)
		if _, err := IssueChildBrief(brief); err == nil {
			t.Fatalf("mutant %d accepted", i)
		}
	}
}

func TestJoinTeamResultsVerdicts(t *testing.T) {
	briefs := []ChildBrief{validBrief(), {ID: "brief-2", TaskID: "task", ParentAgentID: "lead", Role: SpecialistTester, Objective: "test it", TokenBudget: 500}}
	allPass := []MemberOutcome{{BriefID: "brief-1", Status: "completed", VerificationPassed: true}, {BriefID: "brief-2", Status: "done", VerificationPassed: true}}
	if join := JoinTeamResults(briefs, allPass); join.Verdict != JoinComplete {
		t.Fatalf("all-pass verdict = %s", join.Verdict)
	}
	oneFailed := []MemberOutcome{{BriefID: "brief-1", Status: "completed", VerificationPassed: true}, {BriefID: "brief-2", Status: "failed"}}
	join := JoinTeamResults(briefs, oneFailed)
	if join.Verdict != JoinFailed || len(join.Failed) != 1 {
		t.Fatalf("one-failed join mismatch: %+v", join)
	}
	unverified := []MemberOutcome{{BriefID: "brief-1", Status: "completed", VerificationPassed: false}}
	if join := JoinTeamResults(briefs, unverified); join.Verdict != JoinNeedsReview {
		t.Fatalf("unverified verdict = %s", join.Verdict)
	}
	unknown := []MemberOutcome{{BriefID: "brief-x", Status: "completed", VerificationPassed: true}}
	if join := JoinTeamResults(briefs, unknown); join.Verdict != JoinFailed || len(join.Unknown) != 1 {
		t.Fatalf("unknown join mismatch: %+v", join)
	}
}
