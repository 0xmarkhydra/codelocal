package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	longmemory "github.com/0xmarkhydra/codelocal/internal/memory"
)

const promotionCandidateMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_knowledge_promotion_candidates (
 user_id TEXT NOT NULL,
 candidate_id TEXT NOT NULL,
 project_id TEXT NOT NULL,
 repository_id TEXT,
 branch TEXT,
 knowledge_type TEXT NOT NULL CHECK (knowledge_type IN ('architecture','decision','project_fact','constraint','milestone','problem')),
 stable_key TEXT NOT NULL,
 cardinality TEXT NOT NULL CHECK (cardinality IN ('scalar','set')),
 qualifier TEXT NOT NULL DEFAULT '',
 subject JSONB NOT NULL,
 predicate TEXT NOT NULL,
 object JSONB NOT NULL,
 summary TEXT NOT NULL,
 semantic_fingerprint TEXT NOT NULL,
 privacy_classification TEXT NOT NULL CHECK (privacy_classification IN ('public_project','team_project','private_project','local_private','sensitive')),
 confidence DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
 importance DOUBLE PRECISION NOT NULL CHECK (importance >= 0 AND importance <= 1),
 status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','conflicted','rejected','approved','promoted','superseded')),
 created_at BIGINT NOT NULL,
 updated_at BIGINT NOT NULL,
 PRIMARY KEY(user_id,candidate_id),
 FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE,
 FOREIGN KEY(user_id,repository_id) REFERENCES codelocal_repositories(user_id,repository_id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_codelocal_promotion_candidate_identity
 ON codelocal_knowledge_promotion_candidates(
  user_id,project_id,(COALESCE(repository_id,'')),(COALESCE(branch,'')),knowledge_type,stable_key,cardinality,qualifier,semantic_fingerprint
 );
CREATE INDEX IF NOT EXISTS idx_codelocal_promotion_candidates_pending
 ON codelocal_knowledge_promotion_candidates(user_id,project_id,status,updated_at DESC)
 WHERE status IN ('pending','conflicted');

CREATE TABLE IF NOT EXISTS codelocal_knowledge_promotion_evidence (
 user_id TEXT NOT NULL,
 candidate_id TEXT NOT NULL,
 experience_id TEXT NOT NULL,
 proposal_fingerprint TEXT NOT NULL,
 proposal JSONB NOT NULL,
 created_at BIGINT NOT NULL,
 PRIMARY KEY(user_id,candidate_id,experience_id),
 FOREIGN KEY(user_id,candidate_id) REFERENCES codelocal_knowledge_promotion_candidates(user_id,candidate_id) ON DELETE CASCADE,
 FOREIGN KEY(user_id,experience_id) REFERENCES codelocal_experiences(user_id,experience_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_promotion_evidence_experience
 ON codelocal_knowledge_promotion_evidence(user_id,experience_id,created_at DESC);
`

const upsertPromotionCandidateSQL = `
INSERT INTO codelocal_knowledge_promotion_candidates(
 user_id,candidate_id,project_id,repository_id,branch,knowledge_type,stable_key,cardinality,qualifier,subject,predicate,object,
 summary,semantic_fingerprint,privacy_classification,confidence,importance,status,created_at,updated_at)
VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,$10::jsonb,$11,$12::jsonb,$13,$14,$15,$16,$17,'pending',$18,$18)
ON CONFLICT(user_id,candidate_id) DO UPDATE SET
 confidence=GREATEST(codelocal_knowledge_promotion_candidates.confidence,EXCLUDED.confidence),
 importance=GREATEST(codelocal_knowledge_promotion_candidates.importance,EXCLUDED.importance),
 updated_at=GREATEST(codelocal_knowledge_promotion_candidates.updated_at,EXCLUDED.updated_at)`

const insertPromotionEvidenceSQL = `
INSERT INTO codelocal_knowledge_promotion_evidence(user_id,candidate_id,experience_id,proposal_fingerprint,proposal,created_at)
VALUES($1,$2,$3,$4,$5::jsonb,$6)
ON CONFLICT(user_id,candidate_id,experience_id) DO NOTHING`

const selectPromotionCandidateStateSQL = `
SELECT status,created_at,updated_at
FROM codelocal_knowledge_promotion_candidates
WHERE user_id=$1 AND candidate_id=$2`

const (
	promotionCardinalityScalar = "scalar"
	promotionCardinalitySet    = "set"
	promotionStatusPending     = "pending"
	promotionStatusConflicted  = "conflicted"
)

type promotionHint struct {
	Type                  string         `json:"type"`
	StableKey             string         `json:"stableKey"`
	Cardinality           string         `json:"cardinality"`
	Qualifier             string         `json:"qualifier,omitempty"`
	Scope                 string         `json:"scope,omitempty"`
	BranchScoped          bool           `json:"branchScoped,omitempty"`
	Subject               map[string]any `json:"subject"`
	Predicate             string         `json:"predicate"`
	Object                map[string]any `json:"object"`
	Summary               string         `json:"summary"`
	Confidence            float64        `json:"confidence"`
	Importance            float64        `json:"importance"`
	PrivacyClassification string         `json:"privacyClassification,omitempty"`
}

type KnowledgePromotionCandidate struct {
	UserID                string         `json:"userId"`
	CandidateID           string         `json:"candidateId"`
	ProjectID             string         `json:"projectId"`
	RepositoryID          string         `json:"repositoryId,omitempty"`
	Branch                string         `json:"branch,omitempty"`
	KnowledgeType         string         `json:"knowledgeType"`
	StableKey             string         `json:"stableKey"`
	Cardinality           string         `json:"cardinality"`
	Qualifier             string         `json:"qualifier,omitempty"`
	Subject               map[string]any `json:"subject"`
	Predicate             string         `json:"predicate"`
	Object                map[string]any `json:"object"`
	Summary               string         `json:"summary"`
	SemanticFingerprint   string         `json:"semanticFingerprint"`
	PrivacyClassification string         `json:"privacyClassification"`
	Confidence            float64        `json:"confidence"`
	Importance            float64        `json:"importance"`
	Status                string         `json:"status"`
	CreatedAt             int64          `json:"createdAt"`
	UpdatedAt             int64          `json:"updatedAt"`
}

type promotionProposal struct {
	KnowledgeType         string         `json:"knowledgeType"`
	StableKey             string         `json:"stableKey"`
	Cardinality           string         `json:"cardinality"`
	Qualifier             string         `json:"qualifier,omitempty"`
	ProjectID             string         `json:"projectId"`
	RepositoryID          string         `json:"repositoryId,omitempty"`
	Branch                string         `json:"branch,omitempty"`
	Subject               map[string]any `json:"subject"`
	Predicate             string         `json:"predicate"`
	Object                map[string]any `json:"object"`
	Summary               string         `json:"summary"`
	PrivacyClassification string         `json:"privacyClassification"`
	Confidence            float64        `json:"confidence"`
	Importance            float64        `json:"importance"`
}

func validPromotionKnowledgeType(value string) bool {
	switch value {
	case "architecture", "decision", "project_fact", "constraint", "goal", "milestone", "problem":
		return true
	default:
		return false
	}
}

func normalizePromotionStableKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || len(value) > 160 {
		return ""
	}
	var out strings.Builder
	out.Grow(len(value))
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-', r == ':':
			out.WriteRune(r)
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if out.Len() > 0 && !strings.HasSuffix(out.String(), "-") {
				out.WriteByte('-')
			}
		default:
			return ""
		}
	}
	return strings.Trim(out.String(), ".:_-")
}

func promotionEntity(value map[string]any, requireID bool) (map[string]any, bool) {
	kind, _ := value["type"].(string)
	kind = normalizeKnowledgeToken(kind, 80)
	if kind == "" {
		return nil, false
	}
	out := map[string]any{"type": kind}
	if requireID {
		id, _ := value["id"].(string)
		id = longmemory.SanitizeText(id, 160)
		if id == "" {
			return nil, false
		}
		out["id"] = id
		return out, true
	}
	raw, ok := value["value"]
	if !ok {
		return nil, false
	}
	switch typed := raw.(type) {
	case string:
		text := longmemory.SanitizeText(typed, 600)
		if text == "" {
			return nil, false
		}
		out["value"] = text
	case bool, float64, int, int64:
		out["value"] = typed
	default:
		return nil, false
	}
	return out, true
}

func normalizePromotionPredicate(value string) string {
	return normalizeKnowledgeToken(strings.ReplaceAll(value, "_", "-"), 120)
}

func promotionOperationalNoise(predicate, summary string) bool {
	switch predicate {
	case "files-edited", "verification-refreshed", "test-executed", "terminal-progress", "context-refreshed", "agent-iteration", "tool-failed":
		return true
	}
	for _, prefix := range []string{
		"Files edited for task:",
		"Verification evidence refreshed;",
		"Fresh verification evidence reached the ready quality gate for task:",
		"Agent quality gate reached ready state for task:",
	} {
		if strings.HasPrefix(summary, prefix) {
			return true
		}
	}
	return false
}

func normalizePromotionHint(raw any) (promotionHint, bool) {
	if raw == nil {
		return promotionHint{}, false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return promotionHint{}, false
	}
	var hint promotionHint
	if err := json.Unmarshal(encoded, &hint); err != nil {
		return promotionHint{}, false
	}
	hint.Type = normalizeKnowledgeToken(hint.Type, 80)
	hint.StableKey = normalizePromotionStableKey(hint.StableKey)
	hint.Cardinality = strings.ToLower(strings.TrimSpace(hint.Cardinality))
	if hint.Cardinality == "" {
		hint.Cardinality = promotionCardinalityScalar
	}
	hint.Qualifier = normalizePromotionStableKey(hint.Qualifier)
	hint.Scope = strings.ToLower(strings.TrimSpace(hint.Scope))
	if hint.Scope == "" {
		hint.Scope = "project"
	}
	hint.Predicate = normalizePromotionPredicate(hint.Predicate)
	hint.Summary = longmemory.SanitizeText(hint.Summary, 700)
	hint.PrivacyClassification = strings.ToLower(strings.TrimSpace(hint.PrivacyClassification))
	if hint.PrivacyClassification == "" {
		hint.PrivacyClassification = KnowledgeClassPrivateProject
	}
	subject, subjectOK := promotionEntity(hint.Subject, true)
	object, objectOK := promotionEntity(hint.Object, false)
	if !validPromotionKnowledgeType(hint.Type) || hint.StableKey == "" || hint.Predicate == "" || hint.Summary == "" || !subjectOK || !objectOK {
		return promotionHint{}, false
	}
	hint.Subject = subject
	hint.Object = object
	if hint.Cardinality != promotionCardinalityScalar && hint.Cardinality != promotionCardinalitySet {
		return promotionHint{}, false
	}
	if hint.Cardinality == promotionCardinalitySet && hint.Qualifier == "" {
		return promotionHint{}, false
	}
	if hint.Cardinality == promotionCardinalityScalar {
		hint.Qualifier = ""
	}
	if hint.Scope != "project" && hint.Scope != "repository" {
		return promotionHint{}, false
	}
	if hint.PrivacyClassification != KnowledgeClassPrivateProject || hint.Confidence < 0.9 || hint.Confidence > 1 || hint.Importance < 0.65 || hint.Importance > 1 {
		return promotionHint{}, false
	}
	if promotionOperationalNoise(hint.Predicate, hint.Summary) {
		return promotionHint{}, false
	}
	return hint, true
}

func sanitizePromotionCandidateMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return metadata
	}
	raw, exists := metadata["promotionCandidate"]
	if !exists {
		return metadata
	}
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	hint, ok := normalizePromotionHint(raw)
	if !ok {
		delete(cloned, "promotionCandidate")
		return cloned
	}
	cloned["promotionCandidate"] = hint
	return cloned
}

func promotionHintFromExperience(experience Experience) (promotionHint, bool) {
	if experience.Outcome != "succeeded" || strings.TrimSpace(experience.ProjectID) == "" || strings.TrimSpace(experience.VerificationSummary) == "" {
		return promotionHint{}, false
	}
	if source, _ := experience.Metadata["source"].(string); source != "codelocal-verification" {
		return promotionHint{}, false
	}
	hint, ok := normalizePromotionHint(experience.Metadata["promotionCandidate"])
	if !ok {
		return promotionHint{}, false
	}
	if hint.Scope == "repository" && strings.TrimSpace(experience.RepositoryID) == "" {
		return promotionHint{}, false
	}
	if hint.BranchScoped && strings.TrimSpace(experience.Branch) == "" {
		return promotionHint{}, false
	}
	return hint, true
}

func promotionCandidateFromExperience(experience Experience) (KnowledgePromotionCandidate, promotionProposal, bool) {
	hint, ok := promotionHintFromExperience(experience)
	if !ok {
		return KnowledgePromotionCandidate{}, promotionProposal{}, false
	}
	subject, ok := promotionEntity(hint.Subject, true)
	if !ok {
		return KnowledgePromotionCandidate{}, promotionProposal{}, false
	}
	object, ok := promotionEntity(hint.Object, false)
	if !ok {
		return KnowledgePromotionCandidate{}, promotionProposal{}, false
	}
	repositoryID := ""
	if hint.Scope == "repository" {
		repositoryID = strings.TrimSpace(experience.RepositoryID)
	}
	branch := ""
	if hint.BranchScoped {
		branch = longmemory.SanitizeText(experience.Branch, 200)
	}
	proposal := promotionProposal{
		KnowledgeType: hint.Type, StableKey: hint.StableKey, Cardinality: hint.Cardinality, Qualifier: hint.Qualifier,
		ProjectID: strings.TrimSpace(experience.ProjectID), RepositoryID: repositoryID, Branch: branch,
		Subject: subject, Predicate: hint.Predicate, Object: object, Summary: hint.Summary,
		PrivacyClassification: KnowledgeClassPrivateProject, Confidence: hint.Confidence, Importance: hint.Importance,
	}
	semanticPayload, _ := json.Marshal(struct {
		Subject   map[string]any `json:"subject"`
		Predicate string         `json:"predicate"`
		Object    map[string]any `json:"object"`
	}{Subject: subject, Predicate: proposal.Predicate, Object: object})
	fingerprintBytes := sha256.Sum256(semanticPayload)
	fingerprint := hex.EncodeToString(fingerprintBytes[:])
	candidateID := knowledgeStableID("kpc_", experience.UserID, proposal.ProjectID, proposal.RepositoryID, proposal.Branch, proposal.KnowledgeType, proposal.StableKey, proposal.Cardinality, proposal.Qualifier, fingerprint)
	now := time.Now().UnixMilli()
	return KnowledgePromotionCandidate{
		UserID: experience.UserID, CandidateID: candidateID, ProjectID: proposal.ProjectID, RepositoryID: proposal.RepositoryID, Branch: proposal.Branch,
		KnowledgeType: proposal.KnowledgeType, StableKey: proposal.StableKey, Cardinality: proposal.Cardinality, Qualifier: proposal.Qualifier,
		Subject: subject, Predicate: proposal.Predicate, Object: object, Summary: proposal.Summary, SemanticFingerprint: fingerprint,
		PrivacyClassification: KnowledgeClassPrivateProject, Confidence: hint.Confidence, Importance: hint.Importance,
		Status: promotionStatusPending, CreatedAt: now, UpdatedAt: now,
	}, proposal, true
}

func (s *Store) recordPromotionCandidateFromExperience(ctx context.Context, experience Experience) (*KnowledgePromotionCandidate, error) {
	if s == nil || s.DB == nil {
		return nil, errors.New("promotion candidate store unavailable")
	}
	candidate, proposal, ok := promotionCandidateFromExperience(experience)
	if !ok {
		return nil, nil
	}
	proposalJSON, err := json.Marshal(proposal)
	if err != nil {
		return nil, err
	}
	subjectJSON, err := json.Marshal(candidate.Subject)
	if err != nil {
		return nil, err
	}
	objectJSON, err := json.Marshal(candidate.Object)
	if err != nil {
		return nil, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, upsertPromotionCandidateSQL,
		candidate.UserID, candidate.CandidateID, candidate.ProjectID, candidate.RepositoryID, candidate.Branch, candidate.KnowledgeType,
		candidate.StableKey, candidate.Cardinality, candidate.Qualifier, subjectJSON, candidate.Predicate, objectJSON, candidate.Summary,
		candidate.SemanticFingerprint, candidate.PrivacyClassification, candidate.Confidence, candidate.Importance, candidate.CreatedAt); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, insertPromotionEvidenceSQL, candidate.UserID, candidate.CandidateID, experience.ExperienceID, candidate.SemanticFingerprint, proposalJSON, candidate.CreatedAt); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, insertPromotionExperienceSourceSQL, candidate.UserID, candidate.CandidateID, experience.ExperienceID, candidate.SemanticFingerprint, proposalJSON, candidate.CreatedAt); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(ctx, selectPromotionCandidateStateSQL, candidate.UserID, candidate.CandidateID).Scan(&candidate.Status, &candidate.CreatedAt, &candidate.UpdatedAt); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &candidate, nil
}
