package cloud

import (
	"strings"
	"testing"
)

func TestPromotionEvidenceDoesNotCountRetriesAsIndependentCorroboration(t *testing.T) {
	candidate := canonicalTestCandidate()
	candidate.Status = promotionStatusPending
	records := []promotionEvidenceRecord{
		{ExperienceID: "exp-1", TaskID: "task-a", Outcome: "succeeded", VerificationSummary: "verified", ProposalFingerprint: candidate.SemanticFingerprint},
		{ExperienceID: "exp-2", TaskID: "task-a", Outcome: "succeeded", VerificationSummary: "verified again", ProposalFingerprint: candidate.SemanticFingerprint},
	}
	stats := summarizePromotionEvidence(candidate, records)
	if stats.EvidenceCount != 2 || stats.IndependentTaskCount != 1 {
		t.Fatalf("retries were treated as independent corroboration: %#v", stats)
	}
	records = append(records, promotionEvidenceRecord{ExperienceID: "exp-3", TaskID: "task-b", Outcome: "succeeded", VerificationSummary: "verified separately", ProposalFingerprint: candidate.SemanticFingerprint})
	stats = summarizePromotionEvidence(candidate, records)
	if stats.EvidenceCount != 3 || stats.IndependentTaskCount != 2 {
		t.Fatalf("independent task evidence was not recognized: %#v", stats)
	}
}

func TestPromotionEvidenceSurfacesFingerprintConflict(t *testing.T) {
	candidate := canonicalTestCandidate()
	candidate.Status = promotionStatusPending
	stats := summarizePromotionEvidence(candidate, []promotionEvidenceRecord{
		{ExperienceID: "exp-1", TaskID: "task-a", Outcome: "succeeded", VerificationSummary: "verified", ProposalFingerprint: candidate.SemanticFingerprint},
		{ExperienceID: "exp-2", TaskID: "task-b", Outcome: "succeeded", VerificationSummary: "verified", ProposalFingerprint: "different-fingerprint"},
	})
	if stats.ConflictingEvidence != 1 || stats.IndependentTaskCount != 1 {
		t.Fatalf("conflicting proposal evidence was not isolated: %#v", stats)
	}
	candidate.Confidence = .1 // Conflict reason must outrank weaker secondary policy signals.
	if reason, approved := promotionApprovalReason(candidate, stats); approved || reason != "evidence_fingerprint_conflict" {
		t.Fatalf("conflicting evidence should fail closed: reason=%q approved=%t", reason, approved)
	}
}

func TestPromotionApprovalPolicyRequiresIndependentVerifiedTasks(t *testing.T) {
	for _, knowledgeType := range []string{"architecture", "decision", "constraint", "project_fact", "milestone", "problem"} {
		candidate := canonicalTestCandidate()
		candidate.Status = promotionStatusPending
		candidate.KnowledgeType = knowledgeType
		policy := promotionPolicyForCandidate(candidate)
		candidate.Confidence = policy.MinConfidence
		candidate.Importance = policy.MinImportance
		stats := promotionEvidenceStats{EvidenceCount: policy.MinIndependentTasks, IndependentTaskCount: policy.MinIndependentTasks}
		if reason, approved := promotionApprovalReason(candidate, stats); !approved || reason != "deterministic_corroboration" {
			t.Fatalf("%s policy rejected sufficient deterministic evidence: reason=%q approved=%t", knowledgeType, reason, approved)
		}
		stats.IndependentTaskCount--
		if reason, approved := promotionApprovalReason(candidate, stats); approved || reason != "insufficient_independent_evidence" {
			t.Fatalf("%s policy accepted insufficient independent evidence: reason=%q approved=%t", knowledgeType, reason, approved)
		}
	}
}

func TestPromotionApprovalShapeRejectsUnsafeCandidates(t *testing.T) {
	candidate := canonicalTestCandidate()
	candidate.Status = promotionStatusPending
	if reason := promotionCandidateShapeRejectReason(candidate); reason != "" {
		t.Fatalf("valid candidate shape rejected: %q", reason)
	}
	candidate.PrivacyClassification = KnowledgeClassSensitive
	if reason := promotionCandidateShapeRejectReason(candidate); reason != "privacy_classification_not_private" {
		t.Fatalf("unsafe privacy reason=%q", reason)
	}
	candidate = canonicalTestCandidate()
	candidate.Status = promotionStatusPending
	candidate.Predicate = "terminal-progress"
	if reason := promotionCandidateShapeRejectReason(candidate); reason != "invalid_or_operational_semantics" {
		t.Fatalf("operational candidate reason=%q", reason)
	}
}

func TestPromotionApprovalMigrationAndQueriesStayDeterministicAndTenantScoped(t *testing.T) {
	migration := strings.ToLower(strings.Join(strings.Fields(promotionApprovalMigrationSQL), " "))
	for _, required := range []string{"approval_reason", "evidence_count", "independent_task_count", "evaluated_at", "approved_at"} {
		if !strings.Contains(migration, required) {
			t.Fatalf("approval migration lost field %q", required)
		}
	}
	evidence := strings.ToLower(strings.Join(strings.Fields(promotionApprovalEvidenceSQL), " "))
	if !strings.Contains(evidence, "ps.user_id=$1") || !strings.Contains(evidence, "ps.source_type='verified_experience'") || !strings.Contains(evidence, "e.task_id") || !strings.Contains(evidence, "verification_summary") {
		t.Fatalf("evidence query lost tenant/task verification boundary: %s", evidence)
	}
	siblings := strings.ToLower(strings.Join(strings.Fields(promotionApprovalSiblingSQL), " "))
	if !strings.Contains(siblings, "user_id=$1") || !strings.Contains(siblings, "status in ('pending','approved')") {
		t.Fatalf("sibling conflict query lost active tenant-scoped consolidation boundary: %s", siblings)
	}
	if strings.Contains(siblings, "'promoted'") {
		t.Fatal("already-promoted historical candidate must not block a later scalar revision")
	}
}
