package cloud

import (
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/projectidentity"
)

func TestChooseProjectCandidateForSingleRepoRequiresSingleRepoProject(t *testing.T) {
	candidate, ok := chooseProjectCandidate([]projectCandidate{
		{ProjectID: "single", Matched: 1, Total: 1},
		{ProjectID: "biddi", Matched: 1, Total: 8},
	}, 1)
	if !ok || candidate.ProjectID != "single" {
		t.Fatalf("single checkout must not be swallowed by a multi-repo project through one shared repo: %#v %v", candidate, ok)
	}
}

func TestChooseProjectCandidateMatchesStrongMultiRepoOverlap(t *testing.T) {
	candidate, ok := chooseProjectCandidate([]projectCandidate{
		{ProjectID: "biddi", Name: "BIDDI", Matched: 4, Total: 8},
		{ProjectID: "other", Name: "Other", Matched: 2, Total: 4},
	}, 5)
	if !ok || candidate.ProjectID != "biddi" || candidate.Coverage != .8 {
		t.Fatalf("expected strong BIDDI match, got %#v ok=%v", candidate, ok)
	}
}

func TestChooseProjectCandidateRejectsWeakOrAmbiguousOverlap(t *testing.T) {
	if _, ok := chooseProjectCandidate([]projectCandidate{{ProjectID: "weak", Matched: 2, Total: 8}}, 6); ok {
		t.Fatal("two shared repositories out of six incoming repos is too weak to auto-merge")
	}
	if _, ok := chooseProjectCandidate([]projectCandidate{
		{ProjectID: "a", Matched: 3, Total: 8},
		{ProjectID: "b", Matched: 3, Total: 7},
	}, 5); ok {
		t.Fatal("equal-strength project candidates must remain separate until explicitly resolved")
	}
}

func TestChooseProjectCandidateRejectsTinyOverlapWithHugeProject(t *testing.T) {
	if _, ok := chooseProjectCandidate([]projectCandidate{{ProjectID: "huge", Matched: 2, Total: 100}}, 4); ok {
		t.Fatal("two common repositories must not merge a four-repo checkout into an unrelated hundred-repo project")
	}
	candidate, ok := chooseProjectCandidate([]projectCandidate{{ProjectID: "biddi", Matched: 2, Total: 8}}, 2)
	if !ok || candidate.ProjectID != "biddi" {
		t.Fatalf("a focused two-repo checkout of an eight-repo project should still match: %#v ok=%v", candidate, ok)
	}
}

func TestExistingProjectCandidateKeepsPartialCheckoutIdentity(t *testing.T) {
	candidate, ok := existingProjectCandidate([]projectCandidate{
		{ProjectID: "biddi", Matched: 1, Total: 8},
		{ProjectID: "other", Matched: 0, Total: 4},
	}, "biddi", 1)
	if !ok || candidate.ProjectID != "biddi" || candidate.Coverage != 1 {
		t.Fatalf("existing BIDDI workspace should retain identity with a one-repo partial checkout: %#v ok=%v", candidate, ok)
	}
	if _, ok := existingProjectCandidate([]projectCandidate{{ProjectID: "other", Matched: 1, Total: 1}}, "biddi", 1); ok {
		t.Fatal("existing workspace continuity must still require repository evidence for that project")
	}
}

func TestRepositoryLineagesAreDeduplicatedAndBounded(t *testing.T) {
	values := repositoryLineages([]projectidentity.Repository{
		{Lineage: "ABC"}, {Lineage: "abc"}, {Lineage: "def"}, {Lineage: ""},
	})
	if len(values) != 2 || values[0] != "abc" || values[1] != "def" {
		t.Fatalf("unexpected lineage normalization: %#v", values)
	}
}

func TestRemoteRepositoryIDsExcludeLineageOnlyEvidence(t *testing.T) {
	ids := remoteRepositoryIDs([]projectidentity.Repository{
		{ID: "remote-id", Remote: "github.com/company/repo", IdentitySource: "remote"},
		{ID: "lineage-id", Lineage: "abc", IdentitySource: "lineage"},
	})
	if len(ids) != 1 || ids[0] != "remote-id" {
		t.Fatalf("lineage-only repositories must not auto-merge forks/projects: %#v", ids)
	}
}

func TestSafeMarkerProjectID(t *testing.T) {
	if got := safeMarkerProjectID("prj_biddi_01"); got != "prj_biddi_01" {
		t.Fatalf("valid explicit marker rejected: %q", got)
	}
	if got := safeMarkerProjectID("../../other-user"); got != "" {
		t.Fatalf("unsafe marker must be rejected: %q", got)
	}
}

func TestSafeRepositoryRelativePath(t *testing.T) {
	for input, want := range map[string]string{
		".":                  ".",
		"services/product":   "services/product",
		"./services/trading": "services/trading",
	} {
		if got := safeRepositoryRelativePath(input); got != want {
			t.Fatalf("safe relative path %q normalized to %q, want %q", input, got, want)
		}
	}
	for _, input := range []string{"../secret", "/Users/example/repo", "../../other"} {
		if got := safeRepositoryRelativePath(input); got != "" {
			t.Fatalf("unsafe/local absolute repository path must not sync: %q -> %q", input, got)
		}
	}
}
