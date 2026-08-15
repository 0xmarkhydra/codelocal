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

func TestKnowledgeConflictIDSeparatesTenantSourceAndCandidates(t *testing.T) {
	base := knowledgeConflictID("user-a", "source-a", "active-a", "candidate-a")
	variants := []string{
		knowledgeConflictID("user-b", "source-a", "active-a", "candidate-a"),
		knowledgeConflictID("user-a", "source-b", "active-a", "candidate-a"),
		knowledgeConflictID("user-a", "source-a", "active-b", "candidate-a"),
		knowledgeConflictID("user-a", "source-a", "active-a", "candidate-b"),
	}
	for index, variant := range variants {
		if variant == base {
			t.Fatalf("variant %d collapsed into base conflict ID %q", index, base)
		}
	}
}
