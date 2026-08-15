package cloud

import (
	"strings"
	"testing"
)

func canonicalTestCandidate() KnowledgePromotionCandidate {
	return KnowledgePromotionCandidate{
		UserID: "user-a", CandidateID: "candidate-a", ProjectID: "project-a", KnowledgeType: "architecture",
		StableKey: "project.architecture.project-brain", Cardinality: promotionCardinalityScalar,
		Subject: map[string]any{"type": "system", "id": "project-brain"}, Predicate: "uses-architecture",
		Object: map[string]any{"type": "architecture", "value": "server-first"}, Summary: "Project Brain uses server-first architecture.",
		SemanticFingerprint: "fingerprint-a", PrivacyClassification: KnowledgeClassPrivateProject,
		Confidence: .98, Importance: .95, Status: "approved",
	}
}

func TestCanonicalKnowledgeIdentityUsesScopeCardinalityAndQualifier(t *testing.T) {
	base := canonicalTestCandidate()
	baseID := canonicalKnowledgeID(base)
	wording := base
	wording.Summary = "Different display wording for the same semantic object."
	if got := canonicalKnowledgeID(wording); got != baseID {
		t.Fatalf("summary wording changed canonical identity: %q != %q", got, baseID)
	}
	repository := base
	repository.RepositoryID = "repo-a"
	if got := canonicalKnowledgeID(repository); got == baseID {
		t.Fatal("repository scope collapsed into project identity")
	}
	setA := base
	setA.Cardinality = promotionCardinalitySet
	setA.Qualifier = "max-file-lines"
	setB := setA
	setB.Qualifier = "max-function-lines"
	if canonicalKnowledgeID(setA) == canonicalKnowledgeID(setB) {
		t.Fatal("independent set members collapsed into one canonical identity")
	}
}

func TestCanonicalRevisionIdentityTracksSemanticValueNotCandidate(t *testing.T) {
	knowledgeID := canonicalKnowledgeID(canonicalTestCandidate())
	first := canonicalRevisionID("user-a", knowledgeID, "semantic-a")
	if got := canonicalRevisionID("user-a", knowledgeID, "semantic-a"); got != first {
		t.Fatalf("same semantic revision was not idempotent: %q != %q", got, first)
	}
	if got := canonicalRevisionID("user-a", knowledgeID, "semantic-b"); got == first {
		t.Fatal("changed semantic value reused the prior revision identity")
	}
}

func TestCanonicalPromotionGateIsFailClosed(t *testing.T) {
	valid := canonicalTestCandidate()
	if !canonicalCandidateEligible(valid) {
		t.Fatal("approved private verified candidate should pass the deterministic gate")
	}
	for name, mutate := range map[string]func(*KnowledgePromotionCandidate){
		"pending":               func(c *KnowledgePromotionCandidate) { c.Status = "pending" },
		"conflicted":            func(c *KnowledgePromotionCandidate) { c.Status = "conflicted" },
		"public":                func(c *KnowledgePromotionCandidate) { c.PrivacyClassification = KnowledgeClassPublicProject },
		"weak-confidence":       func(c *KnowledgePromotionCandidate) { c.Confidence = .7 },
		"weak-importance":       func(c *KnowledgePromotionCandidate) { c.Importance = .4 },
		"operational":           func(c *KnowledgePromotionCandidate) { c.Predicate = "test-executed" },
		"set-without-qualifier": func(c *KnowledgePromotionCandidate) { c.Cardinality = promotionCardinalitySet },
		"scalar-with-qualifier": func(c *KnowledgePromotionCandidate) { c.Qualifier = "member" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if canonicalCandidateEligible(candidate) {
				t.Fatalf("unsafe candidate passed canonical gate: %#v", candidate)
			}
		})
	}
}

func TestCanonicalMigrationDefinesOnlyAuthoritativeCore(t *testing.T) {
	normalized := strings.ToLower(strings.Join(strings.Fields(canonicalKnowledgeMigrationSQL), " "))
	for _, required := range []string{
		"create table if not exists codelocal_knowledge_objects",
		"create table if not exists codelocal_knowledge_revisions",
		"create table if not exists codelocal_knowledge_provenance",
		"unique(user_id,knowledge_id,revision_number)",
		"unique(user_id,knowledge_id,semantic_fingerprint)",
		"source_type in ('verified_experience')",
		"privacy_classification in ('private_project')",
		"status in ('active','stale','conflicted','superseded','revoked')",
	} {
		if !strings.Contains(normalized, required) {
			t.Fatalf("canonical migration lost invariant %q", required)
		}
	}
	for _, forbidden := range []string{"embedding", "pgvector", "knowledge_edges", "learned_skill"} {
		if strings.Contains(normalized, forbidden) {
			t.Fatalf("canonical migration mixed derived/runtime state into source of truth: %q", forbidden)
		}
	}
}

func TestCanonicalPromotionQueriesRemainTenantScopedAndTemporal(t *testing.T) {
	for name, query := range map[string]string{
		"candidate": canonicalCandidateSelectForUpdateSQL,
		"object":    canonicalObjectSelectForUpdateSQL,
		"revision":  canonicalActiveRevisionSelectSQL,
		"evidence":  canonicalEvidenceSelectSQL,
	} {
		normalized := strings.ToLower(strings.Join(strings.Fields(query), " "))
		if !strings.Contains(normalized, "user_id=$1") {
			t.Fatalf("%s query lost tenant scope: %s", name, normalized)
		}
	}
	source := strings.ToLower(strings.Join(strings.Fields(canonicalKnowledgeMigrationSQL), " "))
	if !strings.Contains(source, "valid_from bigint not null") || !strings.Contains(source, "valid_until bigint") {
		t.Fatal("canonical model lost temporal validity columns")
	}
}

func TestCanonicalPromotionRequiresCandidateAndExperienceProvenance(t *testing.T) {
	normalized := strings.ToLower(strings.Join(strings.Fields(canonicalKnowledgeMigrationSQL), " "))
	for _, required := range []string{
		"foreign key(user_id,candidate_id) references codelocal_knowledge_promotion_candidates",
		"foreign key(user_id,experience_id) references codelocal_experiences",
		"unique(user_id,revision_id,candidate_id,experience_id)",
	} {
		if !strings.Contains(normalized, required) {
			t.Fatalf("canonical provenance lost required link %q", required)
		}
	}
}
