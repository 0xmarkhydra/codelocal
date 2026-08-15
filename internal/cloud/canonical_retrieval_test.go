package cloud

import (
	"strings"
	"testing"
)

func canonicalRecallTestHit(id, knowledgeType string, repositoryID, branch string, confidence, importance float64, updatedAt int64) CanonicalKnowledgeHit {
	return CanonicalKnowledgeHit{
		Knowledge: CanonicalKnowledge{
			UserID: "user-a", KnowledgeID: id, ProjectID: "project-a", RepositoryID: repositoryID, Branch: branch,
			KnowledgeType: knowledgeType, StableKey: "key." + id, Cardinality: promotionCardinalityScalar,
			Status: KnowledgeStatusActive, PrivacyClassification: KnowledgeClassPrivateProject, ActiveRevisionID: "rev-" + id,
			Confidence: confidence, Importance: importance, ValidFrom: 100, UpdatedAt: updatedAt,
		},
		Revision: CanonicalKnowledgeRevision{
			UserID: "user-a", KnowledgeID: id, RevisionID: "rev-" + id, RevisionNumber: 1,
			Subject: map[string]any{"type": "system", "id": id}, Predicate: "describes",
			Object: map[string]any{"type": "value", "value": id}, Summary: id, SemanticFingerprint: "fp-" + id,
			Confidence: confidence, Importance: importance, ValidFrom: 100, CreatedAt: updatedAt,
		},
	}
}

func TestCanonicalRecallInputIsBoundedAndDeterministic(t *testing.T) {
	repositories := []string{"repo-b", "repo-a", "repo-a", "", "repo-c"}
	input, err := normalizeCanonicalRecallInput(CanonicalKnowledgeRecallInput{UserID: " user-a ", ProjectID: " project-a ", RepositoryIDs: repositories, Limit: 999})
	if err != nil {
		t.Fatal(err)
	}
	if input.UserID != "user-a" || input.ProjectID != "project-a" || input.Limit != canonicalRecallMaxLimit {
		t.Fatalf("unexpected normalized recall input: %#v", input)
	}
	if strings.Join(input.RepositoryIDs, ",") != "repo-a,repo-b,repo-c" {
		t.Fatalf("repository scope is not stable/deduplicated: %#v", input.RepositoryIDs)
	}
	if canonicalRecallDatabaseLimit(input.Limit) > canonicalRecallCandidateLimit {
		t.Fatal("canonical recall database candidate set is unbounded")
	}
}

func TestCanonicalRecallEligibilityEnforcesTenantLifecycleTemporalAndActiveRevision(t *testing.T) {
	input := CanonicalKnowledgeRecallInput{UserID: "user-a", ProjectID: "project-a", Limit: 10}
	valid := canonicalRecallTestHit("valid", "project_fact", "", "", .95, .8, 1000)
	for name, mutate := range map[string]func(*CanonicalKnowledgeHit){
		"other-tenant":          func(h *CanonicalKnowledgeHit) { h.Knowledge.UserID = "user-b" },
		"other-project":         func(h *CanonicalKnowledgeHit) { h.Knowledge.ProjectID = "project-b" },
		"stale":                 func(h *CanonicalKnowledgeHit) { h.Knowledge.Status = KnowledgeStatusStale },
		"revoked":               func(h *CanonicalKnowledgeHit) { h.Knowledge.Status = KnowledgeStatusRevoked },
		"expired-object":        func(h *CanonicalKnowledgeHit) { h.Knowledge.ValidUntil = 500 },
		"expired-revision":      func(h *CanonicalKnowledgeHit) { h.Revision.ValidUntil = 500 },
		"future-object":         func(h *CanonicalKnowledgeHit) { h.Knowledge.ValidFrom = 3000 },
		"wrong-active-revision": func(h *CanonicalKnowledgeHit) { h.Knowledge.ActiveRevisionID = "other-revision" },
		"non-private":           func(h *CanonicalKnowledgeHit) { h.Knowledge.PrivacyClassification = KnowledgeClassPublicProject },
	} {
		t.Run(name, func(t *testing.T) {
			hit := valid
			mutate(&hit)
			if canonicalHitEligible(hit, input, 2000) {
				t.Fatalf("ineligible canonical knowledge survived %s gate: %#v", name, hit)
			}
		})
	}
	if !canonicalHitEligible(valid, input, 2000) {
		t.Fatal("valid active canonical knowledge was rejected")
	}
}

func TestCanonicalRecallRepositoryAndBranchScopeAreConservative(t *testing.T) {
	input := CanonicalKnowledgeRecallInput{UserID: "user-a", ProjectID: "project-a", RepositoryIDs: []string{"repo-a"}, Branch: "feat/a", Limit: 10}
	for name, hit := range map[string]CanonicalKnowledgeHit{
		"project":     canonicalRecallTestHit("project", "project_fact", "", "", .95, .8, 1000),
		"repo":        canonicalRecallTestHit("repo", "project_fact", "repo-a", "", .95, .8, 1000),
		"branch":      canonicalRecallTestHit("branch", "project_fact", "", "feat/a", .95, .8, 1000),
		"repo-branch": canonicalRecallTestHit("repo-branch", "project_fact", "repo-a", "feat/a", .95, .8, 1000),
	} {
		if !canonicalHitEligible(hit, input, 2000) {
			t.Fatalf("matching %s scope was rejected", name)
		}
	}
	if canonicalHitEligible(canonicalRecallTestHit("other-repo", "project_fact", "repo-b", "", .95, .8, 1000), input, 2000) {
		t.Fatal("unrelated repository-scoped knowledge leaked into recall")
	}
	if canonicalHitEligible(canonicalRecallTestHit("other-branch", "project_fact", "", "main", .95, .8, 1000), input, 2000) {
		t.Fatal("unrelated branch-scoped knowledge leaked into recall")
	}
	branchless := input
	branchless.Branch = ""
	if canonicalHitEligible(canonicalRecallTestHit("branch-without-context", "project_fact", "", "feat/a", .95, .8, 1000), branchless, 2000) {
		t.Fatal("branch-scoped knowledge leaked when caller had no branch evidence")
	}
}

func TestCanonicalRecallRankingPrefersSpecificAuthoritativeKnowledge(t *testing.T) {
	input := CanonicalKnowledgeRecallInput{UserID: "user-a", ProjectID: "project-a", RepositoryIDs: []string{"repo-a"}, Branch: "feat/a", Limit: 4}
	hits := []CanonicalKnowledgeHit{
		canonicalRecallTestHit("project-constraint", "constraint", "", "", .99, .99, 5000),
		canonicalRecallTestHit("repo-fact", "project_fact", "repo-a", "", .92, .7, 2000),
		canonicalRecallTestHit("repo-branch-fact", "project_fact", "repo-a", "feat/a", .92, .7, 1000),
		canonicalRecallTestHit("repo-branch-constraint", "constraint", "repo-a", "feat/a", .96, .8, 900),
		canonicalRecallTestHit("other-repo", "constraint", "repo-b", "", 1, 1, 9999),
	}
	ranked := rankCanonicalKnowledgeHits(hits, input, 6000)
	if len(ranked) != 4 {
		t.Fatalf("unexpected bounded ranking length: %d", len(ranked))
	}
	want := []string{"repo-branch-constraint", "repo-branch-fact", "repo-fact", "project-constraint"}
	for i, knowledgeID := range want {
		if ranked[i].Knowledge.KnowledgeID != knowledgeID {
			t.Fatalf("rank[%d]=%q want %q; ranked=%#v", i, ranked[i].Knowledge.KnowledgeID, knowledgeID, ranked)
		}
	}
}

func TestCanonicalRecallQueryIsTenantScopedActiveAndBounded(t *testing.T) {
	normalized := strings.ToLower(strings.Join(strings.Fields(canonicalKnowledgeRecallSQL), " "))
	for _, required := range []string{
		"where o.user_id=$1",
		"and o.project_id=$2",
		"and o.status='active'",
		"and o.privacy_classification='private_project'",
		"and o.valid_until is null",
		"and r.valid_until is null",
		"o.repository_id=any($3::text[])",
		"o.branch=$4",
		"limit $6",
	} {
		if !strings.Contains(normalized, required) {
			t.Fatalf("canonical recall query lost invariant %q: %s", required, normalized)
		}
	}
	for _, forbidden := range []string{"embedding", "knowledge_edges", "learned_skill"} {
		if strings.Contains(normalized, forbidden) {
			t.Fatalf("K8 canonical read crossed into derived runtime state: %q", forbidden)
		}
	}
}
