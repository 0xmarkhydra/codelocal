package cloud

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const promotionApprovalMigrationSQL = `
ALTER TABLE codelocal_knowledge_promotion_candidates
 ADD COLUMN IF NOT EXISTS approval_reason TEXT;
ALTER TABLE codelocal_knowledge_promotion_candidates
 ADD COLUMN IF NOT EXISTS evidence_count INTEGER NOT NULL DEFAULT 0 CHECK (evidence_count >= 0);
ALTER TABLE codelocal_knowledge_promotion_candidates
 ADD COLUMN IF NOT EXISTS independent_task_count INTEGER NOT NULL DEFAULT 0 CHECK (independent_task_count >= 0);
ALTER TABLE codelocal_knowledge_promotion_candidates
 ADD COLUMN IF NOT EXISTS evaluated_at BIGINT;
ALTER TABLE codelocal_knowledge_promotion_candidates
 ADD COLUMN IF NOT EXISTS approved_at BIGINT;
CREATE INDEX IF NOT EXISTS idx_codelocal_promotion_candidates_approved
 ON codelocal_knowledge_promotion_candidates(user_id,project_id,approved_at DESC)
 WHERE status='approved';
`

const promotionApprovalEvidenceSQL = `
SELECT ps.experience_id,COALESCE(e.task_id,''),e.outcome,COALESCE(e.verification_summary,''),ps.proposal_fingerprint
FROM codelocal_knowledge_promotion_sources ps
JOIN codelocal_experiences e
 ON e.user_id=ps.user_id AND e.experience_id=ps.experience_id
WHERE ps.user_id=$1 AND ps.candidate_id=$2 AND ps.source_type='verified_experience'
ORDER BY ps.experience_id ASC`

const promotionApprovalSiblingSQL = `
SELECT candidate_id,semantic_fingerprint,status
FROM codelocal_knowledge_promotion_candidates
WHERE user_id=$1
 AND project_id=$2
 AND COALESCE(repository_id,'')=$3
 AND COALESCE(branch,'')=$4
 AND knowledge_type=$5
 AND stable_key=$6
 AND cardinality=$7
 AND qualifier=$8
 AND candidate_id<>$9
 AND status IN ('pending','approved')
ORDER BY candidate_id ASC
FOR UPDATE`

type PromotionApprovalEvaluation struct {
	CandidateID          string `json:"candidateId"`
	Status               string `json:"status"`
	Reason               string `json:"reason"`
	EvidenceCount        int    `json:"evidenceCount"`
	IndependentTaskCount int    `json:"independentTaskCount"`
}

type promotionApprovalPolicy struct {
	MinIndependentTasks int
	MinConfidence       float64
	MinImportance       float64
}

type promotionEvidenceStats struct {
	EvidenceCount        int
	IndependentTaskCount int
	ConflictingEvidence  int
}

type promotionEvidenceRecord struct {
	ExperienceID        string
	TaskID              string
	Outcome             string
	VerificationSummary string
	ProposalFingerprint string
}

func promotionPolicyForCandidate(candidate KnowledgePromotionCandidate) promotionApprovalPolicy {
	switch candidate.KnowledgeType {
	case "architecture", "decision":
		return promotionApprovalPolicy{MinIndependentTasks: 2, MinConfidence: .96, MinImportance: .8}
	case "constraint":
		return promotionApprovalPolicy{MinIndependentTasks: 2, MinConfidence: .95, MinImportance: .75}
	case "project_fact":
		return promotionApprovalPolicy{MinIndependentTasks: 2, MinConfidence: .92, MinImportance: .65}
	case "milestone", "problem":
		return promotionApprovalPolicy{MinIndependentTasks: 2, MinConfidence: .92, MinImportance: .7}
	default:
		return promotionApprovalPolicy{MinIndependentTasks: 99, MinConfidence: 1, MinImportance: 1}
	}
}

func promotionCandidateShapeRejectReason(candidate KnowledgePromotionCandidate) string {
	if candidate.PrivacyClassification != KnowledgeClassPrivateProject {
		return "privacy_classification_not_private"
	}
	if !validPromotionKnowledgeType(candidate.KnowledgeType) || candidate.ProjectID == "" || candidate.StableKey == "" {
		return "invalid_identity_or_type"
	}
	if candidate.Cardinality != promotionCardinalityScalar && candidate.Cardinality != promotionCardinalitySet {
		return "invalid_cardinality"
	}
	if candidate.Cardinality == promotionCardinalitySet && candidate.Qualifier == "" {
		return "set_member_missing_qualifier"
	}
	if candidate.Cardinality == promotionCardinalityScalar && candidate.Qualifier != "" {
		return "scalar_has_qualifier"
	}
	if candidate.SemanticFingerprint == "" || promotionOperationalNoise(candidate.Predicate, candidate.Summary) {
		return "invalid_or_operational_semantics"
	}
	return ""
}

func promotionCandidateShapeEligible(candidate KnowledgePromotionCandidate) bool {
	return promotionCandidateShapeRejectReason(candidate) == ""
}

func promotionApprovalReason(candidate KnowledgePromotionCandidate, stats promotionEvidenceStats) (string, bool) {
	policy := promotionPolicyForCandidate(candidate)
	if stats.ConflictingEvidence > 0 {
		return "evidence_fingerprint_conflict", false
	}
	if candidate.Confidence < policy.MinConfidence {
		return "confidence_below_policy", false
	}
	if candidate.Importance < policy.MinImportance {
		return "importance_below_policy", false
	}
	if stats.IndependentTaskCount < policy.MinIndependentTasks {
		return "insufficient_independent_evidence", false
	}
	return "deterministic_corroboration", true
}

func summarizePromotionEvidence(candidate KnowledgePromotionCandidate, records []promotionEvidenceRecord) promotionEvidenceStats {
	stats := promotionEvidenceStats{}
	independentTasks := map[string]struct{}{}
	for _, record := range records {
		if record.ProposalFingerprint != candidate.SemanticFingerprint {
			stats.ConflictingEvidence++
			continue
		}
		if record.Outcome != "succeeded" || strings.TrimSpace(record.VerificationSummary) == "" {
			continue
		}
		stats.EvidenceCount++
		if taskID := strings.TrimSpace(record.TaskID); taskID != "" {
			independentTasks[taskID] = struct{}{}
		}
	}
	stats.IndependentTaskCount = len(independentTasks)
	return stats
}

func promotionEvidenceStatsForCandidate(ctx context.Context, tx pgx.Tx, candidate KnowledgePromotionCandidate) (promotionEvidenceStats, error) {
	rows, err := tx.Query(ctx, promotionApprovalEvidenceSQL, candidate.UserID, candidate.CandidateID)
	if err != nil {
		return promotionEvidenceStats{}, err
	}
	defer rows.Close()
	records := []promotionEvidenceRecord{}
	for rows.Next() {
		var record promotionEvidenceRecord
		if err := rows.Scan(&record.ExperienceID, &record.TaskID, &record.Outcome, &record.VerificationSummary, &record.ProposalFingerprint); err != nil {
			return promotionEvidenceStats{}, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return promotionEvidenceStats{}, err
	}
	return summarizePromotionEvidence(candidate, records), nil
}

func conflictingPromotionSiblings(ctx context.Context, tx pgx.Tx, candidate KnowledgePromotionCandidate) ([]string, error) {
	rows, err := tx.Query(ctx, promotionApprovalSiblingSQL,
		candidate.UserID, candidate.ProjectID, candidate.RepositoryID, candidate.Branch, candidate.KnowledgeType,
		candidate.StableKey, candidate.Cardinality, candidate.Qualifier, candidate.CandidateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var candidateID, fingerprint, status string
		if err := rows.Scan(&candidateID, &fingerprint, &status); err != nil {
			return nil, err
		}
		if fingerprint != candidate.SemanticFingerprint {
			ids = append(ids, candidateID)
		}
	}
	return ids, rows.Err()
}

func updatePromotionEvaluation(ctx context.Context, tx pgx.Tx, candidate KnowledgePromotionCandidate, status, reason string, stats promotionEvidenceStats, now int64) error {
	approvedAt := int64(0)
	if status == "approved" {
		approvedAt = now
	}
	_, err := tx.Exec(ctx, `
UPDATE codelocal_knowledge_promotion_candidates
SET status=$1,approval_reason=$2,evidence_count=$3,independent_task_count=$4,evaluated_at=$5,
 approved_at=CASE WHEN $6::bigint>0 THEN $6 ELSE approved_at END,updated_at=$5
WHERE user_id=$7 AND candidate_id=$8`,
		status, reason, stats.EvidenceCount, stats.IndependentTaskCount, now, approvedAt, candidate.UserID, candidate.CandidateID)
	return err
}

func markPromotionSiblingsConflicted(ctx context.Context, tx pgx.Tx, userID string, candidateIDs []string, now int64) error {
	for _, candidateID := range candidateIDs {
		if _, err := tx.Exec(ctx, `
UPDATE codelocal_knowledge_promotion_candidates
SET status='conflicted',approval_reason='concurrent_semantic_candidates',evaluated_at=$1,updated_at=$1
WHERE user_id=$2 AND candidate_id=$3 AND status IN ('pending','approved')`, now, userID, candidateID); err != nil {
			return err
		}
	}
	return nil
}

func markPromotionSiblingsSuperseded(ctx context.Context, tx pgx.Tx, userID string, candidateIDs []string, now int64) error {
	for _, candidateID := range candidateIDs {
		if _, err := tx.Exec(ctx, `
UPDATE codelocal_knowledge_promotion_candidates
SET status='superseded',approval_reason='superseded_by_explicit_user_memory',evaluated_at=$1,updated_at=$1
WHERE user_id=$2 AND candidate_id=$3 AND status IN ('pending','approved')`, now, userID, candidateID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) EvaluatePromotionCandidate(ctx context.Context, userID, candidateID string) (PromotionApprovalEvaluation, error) {
	if s == nil || s.DB == nil {
		return PromotionApprovalEvaluation{}, errors.New("promotion approval store unavailable")
	}
	userID = strings.TrimSpace(userID)
	candidateID = strings.TrimSpace(candidateID)
	if userID == "" || candidateID == "" {
		return PromotionApprovalEvaluation{}, errors.New("promotion evaluation requires user and candidate")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return PromotionApprovalEvaluation{}, err
	}
	defer tx.Rollback(ctx)
	candidate, err := scanCanonicalCandidate(tx.QueryRow(ctx, canonicalCandidateSelectForUpdateSQL, userID, candidateID))
	if err != nil {
		return PromotionApprovalEvaluation{}, err
	}
	if candidate.Status != promotionStatusPending {
		return finishPromotionEvaluation(ctx, tx, PromotionApprovalEvaluation{CandidateID: candidateID, Status: candidate.Status, Reason: "already_" + candidate.Status})
	}
	now := time.Now().UnixMilli()
	if reason := promotionCandidateShapeRejectReason(candidate); reason != "" {
		if err := updatePromotionEvaluation(ctx, tx, candidate, "rejected", reason, promotionEvidenceStats{}, now); err != nil {
			return PromotionApprovalEvaluation{}, err
		}
		return finishPromotionEvaluation(ctx, tx, PromotionApprovalEvaluation{CandidateID: candidateID, Status: "rejected", Reason: reason})
	}
	explicitMemory, err := promotionCandidateHasExplicitMemorySource(ctx, tx, userID, candidateID)
	if err != nil {
		return PromotionApprovalEvaluation{}, err
	}
	siblings, err := conflictingPromotionSiblings(ctx, tx, candidate)
	if err != nil {
		return PromotionApprovalEvaluation{}, err
	}
	if explicitMemory {
		if err := markPromotionSiblingsSuperseded(ctx, tx, userID, siblings, now); err != nil {
			return PromotionApprovalEvaluation{}, err
		}
		stats := promotionEvidenceStats{EvidenceCount: 1}
		if err := updatePromotionEvaluation(ctx, tx, candidate, "approved", "explicit_user_memory", stats, now); err != nil {
			return PromotionApprovalEvaluation{}, err
		}
		return finishPromotionEvaluation(ctx, tx, PromotionApprovalEvaluation{CandidateID: candidateID, Status: "approved", Reason: "explicit_user_memory", EvidenceCount: 1})
	}
	if len(siblings) > 0 {
		if err := markPromotionSiblingsConflicted(ctx, tx, userID, siblings, now); err != nil {
			return PromotionApprovalEvaluation{}, err
		}
		if err := updatePromotionEvaluation(ctx, tx, candidate, "conflicted", "concurrent_semantic_candidates", promotionEvidenceStats{}, now); err != nil {
			return PromotionApprovalEvaluation{}, err
		}
		return finishPromotionEvaluation(ctx, tx, PromotionApprovalEvaluation{CandidateID: candidateID, Status: "conflicted", Reason: "concurrent_semantic_candidates"})
	}
	stats, err := promotionEvidenceStatsForCandidate(ctx, tx, candidate)
	if err != nil {
		return PromotionApprovalEvaluation{}, err
	}
	reason, approved := promotionApprovalReason(candidate, stats)
	status := promotionStatusPending
	if approved {
		status = "approved"
	}
	if stats.ConflictingEvidence > 0 {
		status = "conflicted"
	}
	if err := updatePromotionEvaluation(ctx, tx, candidate, status, reason, stats, now); err != nil {
		return PromotionApprovalEvaluation{}, err
	}
	return finishPromotionEvaluation(ctx, tx, PromotionApprovalEvaluation{
		CandidateID: candidateID, Status: status, Reason: reason,
		EvidenceCount: stats.EvidenceCount, IndependentTaskCount: stats.IndependentTaskCount,
	})
}

func finishPromotionEvaluation(ctx context.Context, tx pgx.Tx, evaluation PromotionApprovalEvaluation) (PromotionApprovalEvaluation, error) {
	if err := tx.Commit(ctx); err != nil {
		return PromotionApprovalEvaluation{}, err
	}
	return evaluation, nil
}

func (s *Store) evaluateAndPromoteCandidate(ctx context.Context, candidate *KnowledgePromotionCandidate) error {
	if candidate == nil {
		return nil
	}
	return s.evaluateAndPromoteCandidateRef(ctx, candidate.UserID, candidate.ProjectID, candidate.CandidateID)
}
