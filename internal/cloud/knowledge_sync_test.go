package cloud

import "testing"

func TestKnowledgeAdvanceAllowedRequiresCurrentBaseForChangedContent(t *testing.T) {
	cases := []struct {
		name      string
		active    string
		candidate string
		base      string
		want      bool
	}{
		{name: "first revision", active: "", candidate: "new", base: "", want: true},
		{name: "same revision reobserved", active: "same", candidate: "same", base: "", want: true},
		{name: "linear advance", active: "old", candidate: "new", base: "old", want: true},
		{name: "missing base conflicts", active: "old", candidate: "new", base: "", want: false},
		{name: "stale base conflicts", active: "newer", candidate: "new", base: "old", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := knowledgeAdvanceAllowed(tc.active, tc.candidate, tc.base); got != tc.want {
				t.Fatalf("knowledgeAdvanceAllowed(%q,%q,%q)=%v want=%v", tc.active, tc.candidate, tc.base, got, tc.want)
			}
		})
	}
}

func TestKnowledgeConflictIDSeparatesTenantBranchSourceAndCandidates(t *testing.T) {
	base := knowledgeConflictID("user-a", "source-a", "main", "active-a", "candidate-a")
	variants := []string{
		knowledgeConflictID("user-b", "source-a", "main", "active-a", "candidate-a"),
		knowledgeConflictID("user-a", "source-b", "main", "active-a", "candidate-a"),
		knowledgeConflictID("user-a", "source-a", "feature/x", "active-a", "candidate-a"),
		knowledgeConflictID("user-a", "source-a", "main", "active-b", "candidate-a"),
		knowledgeConflictID("user-a", "source-a", "main", "active-a", "candidate-b"),
	}
	for index, variant := range variants {
		if variant == base {
			t.Fatalf("variant %d collapsed into base conflict ID %q", index, base)
		}
	}
}

func TestBranchDecisionTreatsNewBranchAsForkWithoutOverwritingAnotherBranch(t *testing.T) {
	tests := []struct {
		name        string
		globalHead  string
		branchHead  string
		branch      string
		base        string
		candidate   string
		wantHead    string
		wantFirst   bool
		wantAllowed bool
		wantGlobal  bool
	}{
		{name: "global linear advance", globalHead: "a", base: "a", candidate: "b", wantHead: "a", wantAllowed: true, wantGlobal: true},
		{name: "global stale base conflicts", globalHead: "c", base: "a", candidate: "b", wantHead: "c", wantAllowed: false, wantGlobal: true},
		{name: "new branch from current global may advance global", globalHead: "a", branch: "feat/x", base: "a", candidate: "b", wantHead: "a", wantFirst: true, wantAllowed: true, wantGlobal: true},
		{name: "new branch from stale fork remains branch local", globalHead: "c", branch: "feat/x", base: "a", candidate: "b", wantHead: "a", wantFirst: true, wantAllowed: true, wantGlobal: false},
		{name: "new branch without base is still isolated fork", globalHead: "c", branch: "feat/x", candidate: "b", wantHead: "", wantFirst: true, wantAllowed: true, wantGlobal: false},
		{name: "existing branch linear advance", globalHead: "c", branchHead: "a", branch: "feat/x", base: "a", candidate: "b", wantHead: "a", wantAllowed: true, wantGlobal: false},
		{name: "existing branch stale base conflicts", globalHead: "c", branchHead: "b", branch: "feat/x", base: "a", candidate: "d", wantHead: "b", wantAllowed: false, wantGlobal: false},
		{name: "existing branch that is global may advance global", globalHead: "a", branchHead: "a", branch: "main", base: "a", candidate: "b", wantHead: "a", wantAllowed: true, wantGlobal: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			head, first, allowed, updateGlobal := branchDecision(tc.globalHead, tc.branchHead, tc.branch, tc.base, tc.candidate)
			if head != tc.wantHead || first != tc.wantFirst || allowed != tc.wantAllowed || updateGlobal != tc.wantGlobal {
				t.Fatalf("branchDecision()=(head=%q first=%v allowed=%v global=%v), want (%q,%v,%v,%v)", head, first, allowed, updateGlobal, tc.wantHead, tc.wantFirst, tc.wantAllowed, tc.wantGlobal)
			}
		})
	}
}
