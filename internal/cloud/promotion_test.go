package cloud

import (
	"strings"
	"testing"
)

func validPromotionExperience(hint any) Experience {
	metadata := map[string]any{"source": "codelocal-verification"}
	if hint != nil {
		metadata["promotionCandidate"] = hint
	}
	return Experience{
		UserID: "user-a", ExperienceID: "exp-a", ProjectID: "project-a", RepositoryID: "repo-a", WorkspaceID: "workspace-a",
		TaskID: "task-a", TaskKind: "feature", Objective: "Implement Project Brain", Branch: "feat/brain", Outcome: "succeeded",
		VerificationSummary: "Verified ready quality gate", Metadata: metadata,
	}
}

func validPromotionHint() map[string]any {
	return map[string]any{
		"type": "architecture", "stableKey": "project.architecture.project-brain", "cardinality": "scalar",
		"subject":   map[string]any{"type": "system", "id": "project-brain"},
		"predicate": "uses_architecture", "object": map[string]any{"type": "architecture", "value": "server_first"},
		"summary": "Project Brain uses server-first architecture.", "confidence": 0.98, "importance": 0.95,
	}
}

func TestPromotionCandidateRequiresExplicitStructuredVerifiedHint(t *testing.T) {
	if _, _, ok := promotionCandidateFromExperience(validPromotionExperience(nil)); ok {
		t.Fatal("verified Experience without explicit structured hint must not auto-promote")
	}

	cases := []struct {
		name   string
		mutate func(*Experience, map[string]any)
	}{
		{name: "failed outcome", mutate: func(exp *Experience, _ map[string]any) { exp.Outcome = "failed" }},
		{name: "missing project", mutate: func(exp *Experience, _ map[string]any) { exp.ProjectID = "" }},
		{name: "untrusted source", mutate: func(exp *Experience, _ map[string]any) { exp.Metadata["source"] = "import" }},
		{name: "low confidence", mutate: func(_ *Experience, hint map[string]any) { hint["confidence"] = 0.7 }},
		{name: "public classification", mutate: func(_ *Experience, hint map[string]any) { hint["privacyClassification"] = "public_project" }},
		{name: "operational noise", mutate: func(_ *Experience, hint map[string]any) { hint["predicate"] = "test_executed" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hint := validPromotionHint()
			exp := validPromotionExperience(hint)
			tc.mutate(&exp, hint)
			if _, _, ok := promotionCandidateFromExperience(exp); ok {
				t.Fatalf("unsafe promotion candidate survived %s gate", tc.name)
			}
		})
	}
}

func TestNormalizeExperienceInputSanitizesPromotionHintBeforeDurableStorage(t *testing.T) {
	hint := validPromotionHint()
	hint["object"] = map[string]any{"type": "architecture", "value": "api_key=super-secret-value"}
	input := ExperienceInput{
		UserID: "user-a", Objective: "Implement brain", Outcome: "succeeded", VerificationSummary: "verified", Verified: true,
		Metadata: map[string]any{"source": "codelocal-verification", "promotionCandidate": hint},
	}
	normalized, err := normalizeExperienceInput(input)
	if err != nil {
		t.Fatal(err)
	}
	safeHint, ok := normalized.Metadata["promotionCandidate"].(promotionHint)
	if !ok {
		t.Fatalf("promotion hint was not normalized before durable storage: %#v", normalized.Metadata)
	}
	value, _ := safeHint.Object["value"].(string)
	if strings.Contains(value, "super-secret-value") || !strings.Contains(value, "[REDACTED]") {
		t.Fatalf("promotion object secret survived durable boundary: %q", value)
	}

	unsafe := validPromotionHint()
	unsafe["privacyClassification"] = "public_project"
	input.Metadata["promotionCandidate"] = unsafe
	normalized, err = normalizeExperienceInput(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := normalized.Metadata["promotionCandidate"]; exists {
		t.Fatal("unsafe promotion hint should be removed before durable storage")
	}
}

func TestPromotionCandidateStableIdentitySeparatesMeaningFromWording(t *testing.T) {
	hintA := validPromotionHint()
	expA := validPromotionExperience(hintA)
	candidateA, _, ok := promotionCandidateFromExperience(expA)
	if !ok {
		t.Fatal("valid promotion candidate rejected")
	}
	if candidateA.PrivacyClassification != KnowledgeClassPrivateProject || candidateA.Status != promotionStatusPending {
		t.Fatalf("unexpected initial promotion policy: %#v", candidateA)
	}

	hintB := validPromotionHint()
	hintB["summary"] = "Server-first is the architecture used by Project Brain."
	expB := validPromotionExperience(hintB)
	expB.ExperienceID = "exp-b"
	candidateB, _, ok := promotionCandidateFromExperience(expB)
	if !ok {
		t.Fatal("wording-only candidate rejected")
	}
	if candidateA.CandidateID != candidateB.CandidateID {
		t.Fatalf("wording changed stable identity: %q != %q", candidateA.CandidateID, candidateB.CandidateID)
	}
	if candidateA.SemanticFingerprint != candidateB.SemanticFingerprint {
		t.Fatal("summary wording must not create a semantic conflict")
	}

	hintC := validPromotionHint()
	hintC["object"] = map[string]any{"type": "architecture", "value": "local_first"}
	expC := validPromotionExperience(hintC)
	expC.ExperienceID = "exp-c"
	candidateC, _, ok := promotionCandidateFromExperience(expC)
	if !ok {
		t.Fatal("changed semantic value candidate rejected")
	}
	if candidateA.CandidateID == candidateC.CandidateID {
		t.Fatal("changed semantic value must create a distinct promotion candidate revision")
	}
	if canonicalKnowledgeID(candidateA) != canonicalKnowledgeID(candidateC) {
		t.Fatal("scalar value change must retain canonical KnowledgeObject identity")
	}
	if candidateA.SemanticFingerprint == candidateC.SemanticFingerprint {
		t.Fatal("semantic value change must remain detectable for consolidation/conflict handling")
	}
}

func TestPromotionCandidateSetRepositoryAndBranchScopeAreExplicit(t *testing.T) {
	hint := validPromotionHint()
	hint["type"] = "constraint"
	hint["stableKey"] = "project.constraint::max-file-lines"
	hint["cardinality"] = "set"
	hint["scope"] = "repository"
	hint["branchScoped"] = true
	exp := validPromotionExperience(hint)
	if _, _, ok := promotionCandidateFromExperience(exp); ok {
		t.Fatal("set-valued promotion without qualifier must be rejected")
	}

	hint["qualifier"] = "max-file-lines"
	exp = validPromotionExperience(hint)
	candidate, _, ok := promotionCandidateFromExperience(exp)
	if !ok {
		t.Fatal("qualified repository-scoped candidate rejected")
	}
	if candidate.Cardinality != promotionCardinalitySet || candidate.Qualifier != "max-file-lines" || candidate.RepositoryID != "repo-a" || candidate.Branch != "feat/brain" {
		t.Fatalf("unexpected explicit scope/cardinality: %#v", candidate)
	}

	exp.RepositoryID = ""
	if _, _, ok := promotionCandidateFromExperience(exp); ok {
		t.Fatal("repository-scoped candidate without repository evidence must be rejected")
	}
}

func TestPromotionCandidateMigrationAndSQLKeepTenantEvidenceBoundary(t *testing.T) {
	normalizedMigration := strings.ToLower(strings.Join(strings.Fields(promotionCandidateMigrationSQL), " "))
	for _, required := range []string{
		"primary key(user_id,candidate_id)",
		"unique index if not exists idx_codelocal_promotion_candidate_identity",
		"foreign key(user_id,experience_id) references codelocal_experiences(user_id,experience_id)",
		"status in ('pending','conflicted','rejected','approved','promoted','superseded')",
	} {
		if !strings.Contains(normalizedMigration, required) {
			t.Fatalf("promotion migration lost invariant %q: %s", required, normalizedMigration)
		}
	}
	for name, query := range map[string]string{
		"candidate-select": selectPromotionCandidateStateSQL,
		"candidate-upsert": upsertPromotionCandidateSQL,
		"evidence-insert":  insertPromotionEvidenceSQL,
	} {
		normalized := strings.ToLower(strings.Join(strings.Fields(query), " "))
		if !strings.Contains(normalized, "user_id") {
			t.Fatalf("%s lost first-class tenant key: %s", name, normalized)
		}
		if strings.Contains(normalized, "codelocal_knowledge_objects") || strings.Contains(normalized, "codelocal_knowledge_revisions") {
			t.Fatalf("%s crossed K5 candidate-only boundary into canonical knowledge: %s", name, normalized)
		}
	}
	if !strings.Contains(normalizedMigration, "qualifier,semantic_fingerprint") {
		t.Fatal("candidate identity must preserve separate semantic proposal revisions under one canonical identity")
	}
	if strings.Contains(upsertPromotionCandidateSQL, "THEN 'conflicted'") {
		t.Fatal("candidate upsert must not conflate semantic revision identity with cross-candidate conflict consolidation")
	}
}
