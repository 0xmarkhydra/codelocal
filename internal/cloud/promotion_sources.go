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
	"github.com/jackc/pgx/v5"
)

const promotionSourcesMigrationSQL = `
CREATE UNIQUE INDEX IF NOT EXISTS idx_codelocal_memories_user_identity
 ON codelocal_memories(user_id,id);

CREATE TABLE IF NOT EXISTS codelocal_knowledge_promotion_sources (
 user_id TEXT NOT NULL,
 candidate_id TEXT NOT NULL,
 source_type TEXT NOT NULL CHECK (source_type IN ('verified_experience','explicit_user_memory')),
 source_id TEXT NOT NULL,
 experience_id TEXT,
 memory_id TEXT,
 proposal_fingerprint TEXT NOT NULL,
 proposal JSONB NOT NULL,
 created_at BIGINT NOT NULL,
 PRIMARY KEY(user_id,candidate_id,source_type,source_id),
 FOREIGN KEY(user_id,candidate_id) REFERENCES codelocal_knowledge_promotion_candidates(user_id,candidate_id) ON DELETE CASCADE,
 FOREIGN KEY(user_id,experience_id) REFERENCES codelocal_experiences(user_id,experience_id) ON DELETE CASCADE,
 FOREIGN KEY(user_id,memory_id) REFERENCES codelocal_memories(user_id,id) ON DELETE CASCADE,
 CHECK (
  (source_type='verified_experience' AND experience_id IS NOT NULL AND memory_id IS NULL AND source_id=experience_id) OR
  (source_type='explicit_user_memory' AND memory_id IS NOT NULL AND experience_id IS NULL AND source_id=memory_id)
 )
);
CREATE INDEX IF NOT EXISTS idx_codelocal_promotion_sources_experience
 ON codelocal_knowledge_promotion_sources(user_id,experience_id,created_at DESC) WHERE experience_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_codelocal_promotion_sources_memory
 ON codelocal_knowledge_promotion_sources(user_id,memory_id,created_at DESC) WHERE memory_id IS NOT NULL;
INSERT INTO codelocal_knowledge_promotion_sources(
 user_id,candidate_id,source_type,source_id,experience_id,memory_id,proposal_fingerprint,proposal,created_at)
SELECT user_id,candidate_id,'verified_experience',experience_id,experience_id,NULL,proposal_fingerprint,proposal,created_at
FROM codelocal_knowledge_promotion_evidence
ON CONFLICT(user_id,candidate_id,source_type,source_id) DO NOTHING;

ALTER TABLE codelocal_knowledge_provenance ADD COLUMN IF NOT EXISTS source_id TEXT;
ALTER TABLE codelocal_knowledge_provenance ADD COLUMN IF NOT EXISTS memory_id TEXT;
UPDATE codelocal_knowledge_provenance
 SET source_id=experience_id
 WHERE source_id IS NULL AND source_type='verified_experience' AND experience_id IS NOT NULL;
ALTER TABLE codelocal_knowledge_provenance ALTER COLUMN source_id SET NOT NULL;
ALTER TABLE codelocal_knowledge_provenance ALTER COLUMN experience_id DROP NOT NULL;
ALTER TABLE codelocal_knowledge_provenance DROP CONSTRAINT IF EXISTS codelocal_knowledge_provenance_source_type_check;
ALTER TABLE codelocal_knowledge_provenance DROP CONSTRAINT IF EXISTS codelocal_knowledge_provenance_source_shape_v31;
ALTER TABLE codelocal_knowledge_provenance ADD CONSTRAINT codelocal_knowledge_provenance_source_type_v31
 CHECK (source_type IN ('verified_experience','explicit_user_memory'));
ALTER TABLE codelocal_knowledge_provenance ADD CONSTRAINT codelocal_knowledge_provenance_source_shape_v31 CHECK (
 (source_type='verified_experience' AND experience_id IS NOT NULL AND memory_id IS NULL AND source_id=experience_id) OR
 (source_type='explicit_user_memory' AND memory_id IS NOT NULL AND experience_id IS NULL AND source_id=memory_id)
);
ALTER TABLE codelocal_knowledge_provenance DROP CONSTRAINT IF EXISTS codelocal_knowledge_provenance_memory_fk;
ALTER TABLE codelocal_knowledge_provenance ADD CONSTRAINT codelocal_knowledge_provenance_memory_fk
 FOREIGN KEY(user_id,memory_id) REFERENCES codelocal_memories(user_id,id) ON DELETE RESTRICT NOT VALID;
ALTER TABLE codelocal_knowledge_provenance VALIDATE CONSTRAINT codelocal_knowledge_provenance_memory_fk;
CREATE UNIQUE INDEX IF NOT EXISTS idx_codelocal_knowledge_provenance_source
 ON codelocal_knowledge_provenance(user_id,revision_id,candidate_id,source_type,source_id);

ALTER TABLE codelocal_knowledge_promotion_candidates DROP CONSTRAINT IF EXISTS codelocal_knowledge_promotion_candidates_knowledge_type_check;
ALTER TABLE codelocal_knowledge_promotion_candidates DROP CONSTRAINT IF EXISTS codelocal_knowledge_promotion_candidates_knowledge_type_v31;
ALTER TABLE codelocal_knowledge_promotion_candidates ADD CONSTRAINT codelocal_knowledge_promotion_candidates_knowledge_type_v31
 CHECK (knowledge_type IN ('architecture','decision','project_fact','constraint','goal','milestone','problem'));
ALTER TABLE codelocal_knowledge_objects DROP CONSTRAINT IF EXISTS codelocal_knowledge_objects_knowledge_type_check;
ALTER TABLE codelocal_knowledge_objects DROP CONSTRAINT IF EXISTS codelocal_knowledge_objects_knowledge_type_v31;
ALTER TABLE codelocal_knowledge_objects ADD CONSTRAINT codelocal_knowledge_objects_knowledge_type_v31
 CHECK (knowledge_type IN ('architecture','decision','project_fact','constraint','goal','milestone','problem'));
`

const insertPromotionExperienceSourceSQL = `
INSERT INTO codelocal_knowledge_promotion_sources(
 user_id,candidate_id,source_type,source_id,experience_id,memory_id,proposal_fingerprint,proposal,created_at)
VALUES($1,$2,'verified_experience',$3,$3,NULL,$4,$5::jsonb,$6)
ON CONFLICT(user_id,candidate_id,source_type,source_id) DO NOTHING`

const insertPromotionMemorySourceSQL = `
INSERT INTO codelocal_knowledge_promotion_sources(
 user_id,candidate_id,source_type,source_id,experience_id,memory_id,proposal_fingerprint,proposal,created_at)
VALUES($1,$2,'explicit_user_memory',$3,NULL,$3,$4,$5::jsonb,$6)
ON CONFLICT(user_id,candidate_id,source_type,source_id) DO NOTHING`

const promotionSourcesSelectSQL = `
SELECT source_type,source_id,COALESCE(experience_id,''),COALESCE(memory_id,'')
FROM codelocal_knowledge_promotion_sources
WHERE user_id=$1 AND candidate_id=$2
ORDER BY source_type ASC,source_id ASC`

const explicitMemorySourceSelectSQL = `
SELECT kind,summary,confidence,importance,symbols
FROM codelocal_memories
WHERE user_id=$1 AND id=$2 AND project_id=$3 AND scope='project' AND source_type='conversation'`

const promotionHasExplicitMemorySourceSQL = `
SELECT EXISTS(
 SELECT 1 FROM codelocal_knowledge_promotion_sources
 WHERE user_id=$1 AND candidate_id=$2 AND source_type='explicit_user_memory'
)`

type PromotionSource struct {
	SourceType   string `json:"sourceType"`
	SourceID     string `json:"sourceId"`
	ExperienceID string `json:"experienceId,omitempty"`
	MemoryID     string `json:"memoryId,omitempty"`
}

type ExplicitMemoryPromotionInput struct {
	UserID        string `json:"userId"`
	ProjectID     string `json:"projectId"`
	MemoryID      string `json:"memoryId"`
	SourceKey     string `json:"sourceKey"`
	StableKey     string `json:"stableKey"`
	RevisionToken string `json:"revisionToken"`
}

type explicitMemorySourceRecord struct {
	Kind       string
	Summary    string
	Confidence float64
	Importance float64
	Symbols    []string
}

func explicitMemoryCanonicalType(kind string) (string, string, bool) {
	switch strings.TrimSpace(kind) {
	case "decision":
		return "decision", "has-decision", true
	case "project_fact":
		return "project_fact", "has-fact", true
	case "constraint":
		return "constraint", "has-constraint", true
	case "goal":
		return "goal", "has-goal", true
	case "milestone":
		return "milestone", "has-milestone", true
	case "problem":
		return "problem", "has-problem", true
	default:
		return "", "", false
	}
}

func explicitMemoryRevisionToken(kind, stableKey, summary string, confidence, importance float64) string {
	payload, _ := json.Marshal(struct {
		Kind       string  `json:"kind"`
		StableKey  string  `json:"stableKey"`
		Summary    string  `json:"summary"`
		Confidence float64 `json:"confidence"`
		Importance float64 `json:"importance"`
	}{kind, stableKey, summary, confidence, importance})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func ExplicitMemoryPromotionInputForRecord(userID, projectID string, record longmemory.Record, sourceKey string) (ExplicitMemoryPromotionInput, bool) {
	sourceKey = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(sourceKey)), " "))
	stableKey := normalizePromotionStableKey(sourceKey)
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(projectID) == "" || strings.TrimSpace(record.ID) == "" || sourceKey == "" || stableKey == "" {
		return ExplicitMemoryPromotionInput{}, false
	}
	if record.Scope != longmemory.ScopeProject || strings.TrimSpace(record.ProjectID) != strings.TrimSpace(projectID) || record.SourceType != "conversation" {
		return ExplicitMemoryPromotionInput{}, false
	}
	if _, _, ok := explicitMemoryCanonicalType(record.Kind); !ok {
		return ExplicitMemoryPromotionInput{}, false
	}
	if record.Confidence < .9 || record.Importance < .65 {
		return ExplicitMemoryPromotionInput{}, false
	}
	return ExplicitMemoryPromotionInput{
		UserID: strings.TrimSpace(userID), ProjectID: strings.TrimSpace(projectID), MemoryID: strings.TrimSpace(record.ID), SourceKey: sourceKey, StableKey: stableKey,
		RevisionToken: explicitMemoryRevisionToken(record.Kind, stableKey, record.Summary, record.Confidence, record.Importance),
	}, true
}

func loadExplicitMemorySource(ctx context.Context, tx pgx.Tx, input ExplicitMemoryPromotionInput) (explicitMemorySourceRecord, error) {
	var record explicitMemorySourceRecord
	var symbolsJSON []byte
	err := tx.QueryRow(ctx, explicitMemorySourceSelectSQL, input.UserID, input.MemoryID, input.ProjectID).Scan(
		&record.Kind, &record.Summary, &record.Confidence, &record.Importance, &symbolsJSON,
	)
	if err != nil {
		return explicitMemorySourceRecord{}, err
	}
	if err := json.Unmarshal(symbolsJSON, &record.Symbols); err != nil {
		return explicitMemorySourceRecord{}, err
	}
	return record, nil
}

func explicitMemorySourceMatches(input ExplicitMemoryPromotionInput, record explicitMemorySourceRecord) bool {
	stableKey := normalizePromotionStableKey(input.StableKey)
	sourceKey := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(input.SourceKey)), " "))
	if stableKey == "" || sourceKey == "" || normalizePromotionStableKey(sourceKey) != stableKey || record.Confidence < .9 || record.Importance < .65 {
		return false
	}
	if _, _, ok := explicitMemoryCanonicalType(record.Kind); !ok {
		return false
	}
	expectedSymbol := "memory-key:" + sourceKey
	found := false
	for _, symbol := range record.Symbols {
		if strings.TrimSpace(symbol) == expectedSymbol {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	return input.RevisionToken == explicitMemoryRevisionToken(record.Kind, stableKey, record.Summary, record.Confidence, record.Importance)
}

func explicitMemoryCandidate(input ExplicitMemoryPromotionInput, source explicitMemorySourceRecord) (KnowledgePromotionCandidate, promotionProposal, bool) {
	knowledgeType, predicate, ok := explicitMemoryCanonicalType(source.Kind)
	if !ok || !explicitMemorySourceMatches(input, source) {
		return KnowledgePromotionCandidate{}, promotionProposal{}, false
	}
	summary := longmemory.SanitizeText(source.Summary, 700)
	if summary == "" || promotionOperationalNoise(predicate, summary) {
		return KnowledgePromotionCandidate{}, promotionProposal{}, false
	}
	subject := map[string]any{"type": "project", "id": input.ProjectID}
	object := map[string]any{"type": knowledgeType, "value": summary}
	proposal := promotionProposal{
		KnowledgeType: knowledgeType, StableKey: normalizePromotionStableKey(input.StableKey), Cardinality: promotionCardinalityScalar,
		ProjectID: input.ProjectID, Subject: subject, Predicate: predicate, Object: object, Summary: summary,
	}
	semanticPayload, _ := json.Marshal(struct {
		Subject   map[string]any `json:"subject"`
		Predicate string         `json:"predicate"`
		Object    map[string]any `json:"object"`
	}{subject, predicate, object})
	fingerprintBytes := sha256.Sum256(semanticPayload)
	fingerprint := hex.EncodeToString(fingerprintBytes[:])
	candidateID := knowledgeStableID("kpc_", input.UserID, input.ProjectID, "", "", knowledgeType, proposal.StableKey, promotionCardinalityScalar, "", fingerprint)
	now := time.Now().UnixMilli()
	return KnowledgePromotionCandidate{
		UserID: input.UserID, CandidateID: candidateID, ProjectID: input.ProjectID,
		KnowledgeType: knowledgeType, StableKey: proposal.StableKey, Cardinality: promotionCardinalityScalar,
		Subject: subject, Predicate: predicate, Object: object, Summary: summary, SemanticFingerprint: fingerprint,
		PrivacyClassification: KnowledgeClassPrivateProject, Confidence: source.Confidence, Importance: source.Importance,
		Status: promotionStatusPending, CreatedAt: now, UpdatedAt: now,
	}, proposal, true
}

func (s *Store) StageExplicitMemoryCandidate(ctx context.Context, input ExplicitMemoryPromotionInput) (*KnowledgePromotionCandidate, error) {
	if s == nil || s.DB == nil {
		return nil, errors.New("promotion candidate store unavailable")
	}
	input.UserID = strings.TrimSpace(input.UserID)
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.MemoryID = strings.TrimSpace(input.MemoryID)
	input.SourceKey = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(input.SourceKey)), " "))
	input.StableKey = normalizePromotionStableKey(input.StableKey)
	input.RevisionToken = strings.TrimSpace(input.RevisionToken)
	if input.UserID == "" || input.ProjectID == "" || input.MemoryID == "" || input.SourceKey == "" || input.StableKey == "" || input.RevisionToken == "" {
		return nil, errors.New("explicit memory promotion requires user, project, memory, source key, stable key and revision token")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	source, err := loadExplicitMemorySource(ctx, tx, input)
	if err != nil {
		return nil, err
	}
	candidate, proposal, ok := explicitMemoryCandidate(input, source)
	if !ok {
		// A stale outbox event means the mutable memory row has already advanced.
		// The newer revision has its own outbox dedupe key and is authoritative.
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
	if _, err := tx.Exec(ctx, upsertPromotionCandidateSQL,
		candidate.UserID, candidate.CandidateID, candidate.ProjectID, candidate.RepositoryID, candidate.Branch, candidate.KnowledgeType,
		candidate.StableKey, candidate.Cardinality, candidate.Qualifier, subjectJSON, candidate.Predicate, objectJSON, candidate.Summary,
		candidate.SemanticFingerprint, candidate.PrivacyClassification, candidate.Confidence, candidate.Importance, candidate.CreatedAt); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, insertPromotionMemorySourceSQL,
		candidate.UserID, candidate.CandidateID, input.MemoryID, candidate.SemanticFingerprint, proposalJSON, candidate.CreatedAt); err != nil {
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

func promotionSourcesForCandidate(ctx context.Context, tx pgx.Tx, userID, candidateID string) ([]PromotionSource, error) {
	rows, err := tx.Query(ctx, promotionSourcesSelectSQL, userID, candidateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sources := []PromotionSource{}
	for rows.Next() {
		var source PromotionSource
		if err := rows.Scan(&source.SourceType, &source.SourceID, &source.ExperienceID, &source.MemoryID); err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

func promotionCandidateHasExplicitMemorySource(ctx context.Context, tx pgx.Tx, userID, candidateID string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, promotionHasExplicitMemorySourceSQL, userID, candidateID).Scan(&exists)
	return exists, err
}
