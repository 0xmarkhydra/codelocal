package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const canonicalKnowledgeMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_knowledge_objects (
 user_id TEXT NOT NULL,
 knowledge_id TEXT NOT NULL,
 project_id TEXT NOT NULL,
 repository_id TEXT,
 branch TEXT,
 knowledge_type TEXT NOT NULL CHECK (knowledge_type IN ('architecture','decision','project_fact','constraint','milestone','problem')),
 stable_key TEXT NOT NULL,
 cardinality TEXT NOT NULL CHECK (cardinality IN ('scalar','set')),
 qualifier TEXT NOT NULL DEFAULT '',
 status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','stale','conflicted','superseded','revoked')),
 privacy_classification TEXT NOT NULL CHECK (privacy_classification IN ('private_project')),
 active_revision_id TEXT,
 confidence DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
 importance DOUBLE PRECISION NOT NULL CHECK (importance >= 0 AND importance <= 1),
 valid_from BIGINT NOT NULL,
 valid_until BIGINT,
 created_at BIGINT NOT NULL,
 updated_at BIGINT NOT NULL,
 PRIMARY KEY(user_id,knowledge_id),
 FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE,
 FOREIGN KEY(user_id,repository_id) REFERENCES codelocal_repositories(user_id,repository_id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_codelocal_knowledge_object_identity
 ON codelocal_knowledge_objects(
  user_id,project_id,(COALESCE(repository_id,'')),(COALESCE(branch,'')),knowledge_type,stable_key,cardinality,qualifier
 );
CREATE INDEX IF NOT EXISTS idx_codelocal_knowledge_objects_active
 ON codelocal_knowledge_objects(user_id,project_id,status,updated_at DESC)
 WHERE status IN ('active','stale','conflicted');

CREATE TABLE IF NOT EXISTS codelocal_knowledge_revisions (
 user_id TEXT NOT NULL,
 revision_id TEXT NOT NULL,
 knowledge_id TEXT NOT NULL,
 revision_number INTEGER NOT NULL CHECK (revision_number > 0),
 subject JSONB NOT NULL,
 predicate TEXT NOT NULL,
 object JSONB NOT NULL,
 summary TEXT NOT NULL,
 semantic_fingerprint TEXT NOT NULL,
 confidence DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
 importance DOUBLE PRECISION NOT NULL CHECK (importance >= 0 AND importance <= 1),
 valid_from BIGINT NOT NULL,
 valid_until BIGINT,
 created_at BIGINT NOT NULL,
 PRIMARY KEY(user_id,revision_id),
 UNIQUE(user_id,knowledge_id,revision_number),
 UNIQUE(user_id,knowledge_id,semantic_fingerprint),
 FOREIGN KEY(user_id,knowledge_id) REFERENCES codelocal_knowledge_objects(user_id,knowledge_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_knowledge_revisions_current
 ON codelocal_knowledge_revisions(user_id,knowledge_id,valid_until,revision_number DESC);

CREATE TABLE IF NOT EXISTS codelocal_knowledge_provenance (
 user_id TEXT NOT NULL,
 provenance_id TEXT NOT NULL,
 knowledge_id TEXT NOT NULL,
 revision_id TEXT NOT NULL,
 candidate_id TEXT NOT NULL,
 experience_id TEXT NOT NULL,
 source_type TEXT NOT NULL CHECK (source_type IN ('verified_experience')),
 created_at BIGINT NOT NULL,
 metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
 PRIMARY KEY(user_id,provenance_id),
 UNIQUE(user_id,revision_id,candidate_id,experience_id),
 FOREIGN KEY(user_id,knowledge_id) REFERENCES codelocal_knowledge_objects(user_id,knowledge_id) ON DELETE CASCADE,
 FOREIGN KEY(user_id,revision_id) REFERENCES codelocal_knowledge_revisions(user_id,revision_id) ON DELETE CASCADE,
 FOREIGN KEY(user_id,candidate_id) REFERENCES codelocal_knowledge_promotion_candidates(user_id,candidate_id) ON DELETE RESTRICT,
 FOREIGN KEY(user_id,experience_id) REFERENCES codelocal_experiences(user_id,experience_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_codelocal_knowledge_provenance_object
 ON codelocal_knowledge_provenance(user_id,knowledge_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_codelocal_knowledge_provenance_experience
 ON codelocal_knowledge_provenance(user_id,experience_id,created_at DESC);

ALTER TABLE codelocal_knowledge_objects
 ADD CONSTRAINT fk_codelocal_knowledge_active_revision
 FOREIGN KEY(user_id,active_revision_id)
 REFERENCES codelocal_knowledge_revisions(user_id,revision_id)
 DEFERRABLE INITIALLY DEFERRED;
`

const canonicalCandidateSelectForUpdateSQL = `
SELECT user_id,candidate_id,project_id,COALESCE(repository_id,''),COALESCE(branch,''),knowledge_type,stable_key,cardinality,qualifier,
 subject,predicate,object,summary,semantic_fingerprint,privacy_classification,confidence,importance,status,created_at,updated_at
FROM codelocal_knowledge_promotion_candidates
WHERE user_id=$1 AND candidate_id=$2
FOR UPDATE`

const canonicalObjectSelectForUpdateSQL = `
SELECT knowledge_id,status,COALESCE(active_revision_id,''),confidence,importance,valid_from,COALESCE(valid_until,0),created_at,updated_at
FROM codelocal_knowledge_objects
WHERE user_id=$1 AND knowledge_id=$2
FOR UPDATE`

const canonicalActiveRevisionSelectSQL = `
SELECT revision_id,revision_number,subject,predicate,object,summary,semantic_fingerprint,confidence,importance,valid_from,COALESCE(valid_until,0),created_at
FROM codelocal_knowledge_revisions
WHERE user_id=$1 AND revision_id=$2`

const canonicalEvidenceSelectSQL = promotionSourcesSelectSQL

type CanonicalKnowledge struct {
	UserID                string  `json:"userId"`
	KnowledgeID           string  `json:"knowledgeId"`
	ProjectID             string  `json:"projectId"`
	RepositoryID          string  `json:"repositoryId,omitempty"`
	Branch                string  `json:"branch,omitempty"`
	KnowledgeType         string  `json:"knowledgeType"`
	StableKey             string  `json:"stableKey"`
	Cardinality           string  `json:"cardinality"`
	Qualifier             string  `json:"qualifier,omitempty"`
	Status                string  `json:"status"`
	PrivacyClassification string  `json:"privacyClassification"`
	ActiveRevisionID      string  `json:"activeRevisionId"`
	Confidence            float64 `json:"confidence"`
	Importance            float64 `json:"importance"`
	ValidFrom             int64   `json:"validFrom"`
	ValidUntil            int64   `json:"validUntil,omitempty"`
	CreatedAt             int64   `json:"createdAt"`
	UpdatedAt             int64   `json:"updatedAt"`
}

type CanonicalKnowledgeRevision struct {
	UserID              string         `json:"userId"`
	RevisionID          string         `json:"revisionId"`
	KnowledgeID         string         `json:"knowledgeId"`
	RevisionNumber      int            `json:"revisionNumber"`
	Subject             map[string]any `json:"subject"`
	Predicate           string         `json:"predicate"`
	Object              map[string]any `json:"object"`
	Summary             string         `json:"summary"`
	SemanticFingerprint string         `json:"semanticFingerprint"`
	Confidence          float64        `json:"confidence"`
	Importance          float64        `json:"importance"`
	ValidFrom           int64          `json:"validFrom"`
	ValidUntil          int64          `json:"validUntil,omitempty"`
	CreatedAt           int64          `json:"createdAt"`
}

type canonicalKnowledgeState struct {
	KnowledgeID      string
	Status           string
	ActiveRevisionID string
	Confidence       float64
	Importance       float64
	ValidFrom        int64
	ValidUntil       int64
	CreatedAt        int64
	UpdatedAt        int64
}

type canonicalRevisionState struct {
	RevisionID          string
	RevisionNumber      int
	SemanticFingerprint string
	ValidFrom           int64
	ValidUntil          int64
}

func canonicalKnowledgeID(candidate KnowledgePromotionCandidate) string {
	return knowledgeStableID(
		"knw_", candidate.UserID, candidate.ProjectID, candidate.RepositoryID, candidate.Branch,
		candidate.KnowledgeType, candidate.StableKey, candidate.Cardinality, candidate.Qualifier,
	)
}

func canonicalRevisionID(userID, knowledgeID, semanticFingerprint string) string {
	return knowledgeStableID("knr_", userID, knowledgeID, semanticFingerprint)
}

func canonicalCandidateEligible(candidate KnowledgePromotionCandidate) bool {
	if candidate.Status != "approved" || candidate.PrivacyClassification != KnowledgeClassPrivateProject {
		return false
	}
	if !validPromotionKnowledgeType(candidate.KnowledgeType) || candidate.ProjectID == "" || candidate.StableKey == "" {
		return false
	}
	if candidate.Cardinality != promotionCardinalityScalar && candidate.Cardinality != promotionCardinalitySet {
		return false
	}
	if candidate.Cardinality == promotionCardinalitySet && candidate.Qualifier == "" {
		return false
	}
	if candidate.Cardinality == promotionCardinalityScalar && candidate.Qualifier != "" {
		return false
	}
	if candidate.Confidence < 0.9 || candidate.Importance < 0.65 || candidate.SemanticFingerprint == "" {
		return false
	}
	return !promotionOperationalNoise(candidate.Predicate, candidate.Summary)
}

func scanCanonicalCandidate(row interface{ Scan(...any) error }) (KnowledgePromotionCandidate, error) {
	var candidate KnowledgePromotionCandidate
	var subjectJSON, objectJSON []byte
	err := row.Scan(
		&candidate.UserID, &candidate.CandidateID, &candidate.ProjectID, &candidate.RepositoryID, &candidate.Branch,
		&candidate.KnowledgeType, &candidate.StableKey, &candidate.Cardinality, &candidate.Qualifier,
		&subjectJSON, &candidate.Predicate, &objectJSON, &candidate.Summary, &candidate.SemanticFingerprint,
		&candidate.PrivacyClassification, &candidate.Confidence, &candidate.Importance, &candidate.Status, &candidate.CreatedAt, &candidate.UpdatedAt,
	)
	if err != nil {
		return KnowledgePromotionCandidate{}, err
	}
	if err := json.Unmarshal(subjectJSON, &candidate.Subject); err != nil {
		return KnowledgePromotionCandidate{}, err
	}
	if err := json.Unmarshal(objectJSON, &candidate.Object); err != nil {
		return KnowledgePromotionCandidate{}, err
	}
	return candidate, nil
}

func canonicalPromotionSources(ctx context.Context, tx pgx.Tx, userID, candidateID string) ([]PromotionSource, error) {
	return promotionSourcesForCandidate(ctx, tx, userID, candidateID)
}

func canonicalProvenanceMetadata(candidate KnowledgePromotionCandidate) []byte {
	metadata, _ := json.Marshal(map[string]any{
		"candidateStatus":       candidate.Status,
		"semanticFingerprint":   candidate.SemanticFingerprint,
		"privacyClassification": candidate.PrivacyClassification,
	})
	return metadata
}

func canonicalKnowledgeFromCandidate(candidate KnowledgePromotionCandidate, knowledgeID, revisionID string, now int64) CanonicalKnowledge {
	return CanonicalKnowledge{
		UserID: candidate.UserID, KnowledgeID: knowledgeID, ProjectID: candidate.ProjectID, RepositoryID: candidate.RepositoryID, Branch: candidate.Branch,
		KnowledgeType: candidate.KnowledgeType, StableKey: candidate.StableKey, Cardinality: candidate.Cardinality, Qualifier: candidate.Qualifier,
		Status: KnowledgeStatusActive, PrivacyClassification: candidate.PrivacyClassification, ActiveRevisionID: revisionID,
		Confidence: candidate.Confidence, Importance: candidate.Importance, ValidFrom: now, CreatedAt: now, UpdatedAt: now,
	}
}

func canonicalRevisionFromCandidate(candidate KnowledgePromotionCandidate, knowledgeID string, number int, now int64) CanonicalKnowledgeRevision {
	return CanonicalKnowledgeRevision{
		UserID: candidate.UserID, RevisionID: canonicalRevisionID(candidate.UserID, knowledgeID, candidate.SemanticFingerprint), KnowledgeID: knowledgeID,
		RevisionNumber: number, Subject: candidate.Subject, Predicate: candidate.Predicate, Object: candidate.Object, Summary: candidate.Summary,
		SemanticFingerprint: candidate.SemanticFingerprint, Confidence: candidate.Confidence, Importance: candidate.Importance, ValidFrom: now, CreatedAt: now,
	}
}

func canonicalKnowledgeGateError(candidate KnowledgePromotionCandidate) error {
	status := strings.TrimSpace(candidate.Status)
	if status == "conflicted" || status == "rejected" || status == "superseded" {
		return errors.New("promotion candidate is not promotable: " + status)
	}
	if status != "approved" {
		return errors.New("promotion candidate requires approved status")
	}
	return errors.New("promotion candidate failed deterministic canonical gate")
}

func canonicalRevisionFromRow(row interface{ Scan(...any) error }, userID, knowledgeID string) (CanonicalKnowledgeRevision, error) {
	var revision CanonicalKnowledgeRevision
	var subjectJSON, objectJSON []byte
	revision.UserID = userID
	revision.KnowledgeID = knowledgeID
	err := row.Scan(
		&revision.RevisionID, &revision.RevisionNumber, &subjectJSON, &revision.Predicate, &objectJSON, &revision.Summary,
		&revision.SemanticFingerprint, &revision.Confidence, &revision.Importance, &revision.ValidFrom, &revision.ValidUntil, &revision.CreatedAt,
	)
	if err != nil {
		return CanonicalKnowledgeRevision{}, err
	}
	if err := json.Unmarshal(subjectJSON, &revision.Subject); err != nil {
		return CanonicalKnowledgeRevision{}, err
	}
	if err := json.Unmarshal(objectJSON, &revision.Object); err != nil {
		return CanonicalKnowledgeRevision{}, err
	}
	return revision, nil
}

func canonicalKnowledgeFromState(candidate KnowledgePromotionCandidate, state canonicalKnowledgeState) CanonicalKnowledge {
	return CanonicalKnowledge{
		UserID: candidate.UserID, KnowledgeID: state.KnowledgeID, ProjectID: candidate.ProjectID, RepositoryID: candidate.RepositoryID, Branch: candidate.Branch,
		KnowledgeType: candidate.KnowledgeType, StableKey: candidate.StableKey, Cardinality: candidate.Cardinality, Qualifier: candidate.Qualifier,
		Status: state.Status, PrivacyClassification: candidate.PrivacyClassification, ActiveRevisionID: state.ActiveRevisionID,
		Confidence: state.Confidence, Importance: state.Importance, ValidFrom: state.ValidFrom, ValidUntil: state.ValidUntil, CreatedAt: state.CreatedAt, UpdatedAt: state.UpdatedAt,
	}
}

func loadCanonicalObjectState(ctx context.Context, tx pgx.Tx, userID, knowledgeID string) (canonicalKnowledgeState, error) {
	var state canonicalKnowledgeState
	err := tx.QueryRow(ctx, canonicalObjectSelectForUpdateSQL, userID, knowledgeID).Scan(
		&state.KnowledgeID, &state.Status, &state.ActiveRevisionID, &state.Confidence, &state.Importance,
		&state.ValidFrom, &state.ValidUntil, &state.CreatedAt, &state.UpdatedAt,
	)
	return state, err
}

func loadCanonicalRevision(ctx context.Context, tx pgx.Tx, userID, knowledgeID, revisionID string) (CanonicalKnowledgeRevision, error) {
	return canonicalRevisionFromRow(tx.QueryRow(ctx, canonicalActiveRevisionSelectSQL, userID, revisionID), userID, knowledgeID)
}

func insertCanonicalObject(ctx context.Context, tx pgx.Tx, candidate KnowledgePromotionCandidate, knowledgeID string, now int64) error {
	_, err := tx.Exec(ctx, `
INSERT INTO codelocal_knowledge_objects(
 user_id,knowledge_id,project_id,repository_id,branch,knowledge_type,stable_key,cardinality,qualifier,status,privacy_classification,
 active_revision_id,confidence,importance,valid_from,valid_until,created_at,updated_at)
VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,'active',$10,NULL,$11,$12,$13,NULL,$13,$13)`,
		candidate.UserID, knowledgeID, candidate.ProjectID, candidate.RepositoryID, candidate.Branch, candidate.KnowledgeType,
		candidate.StableKey, candidate.Cardinality, candidate.Qualifier, candidate.PrivacyClassification, candidate.Confidence, candidate.Importance, now)
	return err
}

func insertCanonicalRevision(ctx context.Context, tx pgx.Tx, revision CanonicalKnowledgeRevision) error {
	subjectJSON, err := json.Marshal(revision.Subject)
	if err != nil {
		return err
	}
	objectJSON, err := json.Marshal(revision.Object)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
INSERT INTO codelocal_knowledge_revisions(
 user_id,revision_id,knowledge_id,revision_number,subject,predicate,object,summary,semantic_fingerprint,confidence,importance,valid_from,valid_until,created_at)
VALUES($1,$2,$3,$4,$5::jsonb,$6,$7::jsonb,$8,$9,$10,$11,$12,NULL,$12)`,
		revision.UserID, revision.RevisionID, revision.KnowledgeID, revision.RevisionNumber, subjectJSON, revision.Predicate, objectJSON,
		revision.Summary, revision.SemanticFingerprint, revision.Confidence, revision.Importance, revision.ValidFrom)
	return err
}

func activateCanonicalRevision(ctx context.Context, tx pgx.Tx, candidate KnowledgePromotionCandidate, knowledgeID, revisionID string, now int64) error {
	_, err := tx.Exec(ctx, `
UPDATE codelocal_knowledge_objects
SET status='active',active_revision_id=$1,confidence=GREATEST(confidence,$2),importance=GREATEST(importance,$3),valid_until=NULL,updated_at=$4
WHERE user_id=$5 AND knowledge_id=$6`, revisionID, candidate.Confidence, candidate.Importance, now, candidate.UserID, knowledgeID)
	return err
}

func closeCanonicalRevision(ctx context.Context, tx pgx.Tx, userID, revisionID string, now int64) error {
	_, err := tx.Exec(ctx, `
UPDATE codelocal_knowledge_revisions
SET valid_until=$1
WHERE user_id=$2 AND revision_id=$3 AND valid_until IS NULL`, now, userID, revisionID)
	return err
}

func insertCanonicalProvenance(ctx context.Context, tx pgx.Tx, candidate KnowledgePromotionCandidate, knowledgeID, revisionID string, sources []PromotionSource, now int64) error {
	metadata := canonicalProvenanceMetadata(candidate)
	for _, source := range sources {
		provenanceID := knowledgeStableID("knp_", candidate.UserID, knowledgeID, revisionID, candidate.CandidateID, source.SourceType, source.SourceID)
		if _, err := tx.Exec(ctx, `
INSERT INTO codelocal_knowledge_provenance(
 user_id,provenance_id,knowledge_id,revision_id,candidate_id,experience_id,source_type,source_id,memory_id,created_at,metadata)
VALUES($1,$2,$3,$4,$5,NULLIF($6,''),$7,$8,NULLIF($9,''),$10,$11::jsonb)
ON CONFLICT(user_id,provenance_id) DO NOTHING`,
			candidate.UserID, provenanceID, knowledgeID, revisionID, candidate.CandidateID, source.ExperienceID, source.SourceType, source.SourceID, source.MemoryID, now, metadata); err != nil {
			return err
		}
	}
	return nil
}

func markCanonicalCandidatePromoted(ctx context.Context, tx pgx.Tx, userID, candidateID string, now int64) error {
	tag, err := tx.Exec(ctx, `
UPDATE codelocal_knowledge_promotion_candidates
SET status='promoted',updated_at=$1
WHERE user_id=$2 AND candidate_id=$3 AND status='approved'`, now, userID, candidateID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("promotion candidate state changed before canonical commit")
	}
	return nil
}

func existingCanonicalPromotion(ctx context.Context, tx pgx.Tx, candidate KnowledgePromotionCandidate) (CanonicalKnowledge, CanonicalKnowledgeRevision, error) {
	knowledgeID := canonicalKnowledgeID(candidate)
	state, err := loadCanonicalObjectState(ctx, tx, candidate.UserID, knowledgeID)
	if err != nil {
		return CanonicalKnowledge{}, CanonicalKnowledgeRevision{}, err
	}
	if state.ActiveRevisionID == "" {
		return CanonicalKnowledge{}, CanonicalKnowledgeRevision{}, errors.New("promoted knowledge has no active revision")
	}
	revision, err := loadCanonicalRevision(ctx, tx, candidate.UserID, knowledgeID, state.ActiveRevisionID)
	if err != nil {
		return CanonicalKnowledge{}, CanonicalKnowledgeRevision{}, err
	}
	return canonicalKnowledgeFromState(candidate, state), revision, nil
}

func (s *Store) PromoteApprovedCandidate(ctx context.Context, userID, candidateID string) (CanonicalKnowledge, CanonicalKnowledgeRevision, error) {
	if s == nil || s.DB == nil {
		return CanonicalKnowledge{}, CanonicalKnowledgeRevision{}, errors.New("canonical knowledge store unavailable")
	}
	userID = strings.TrimSpace(userID)
	candidateID = strings.TrimSpace(candidateID)
	if userID == "" || candidateID == "" {
		return CanonicalKnowledge{}, CanonicalKnowledgeRevision{}, errors.New("canonical promotion requires user and candidate")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return CanonicalKnowledge{}, CanonicalKnowledgeRevision{}, err
	}
	defer tx.Rollback(ctx)
	candidate, err := scanCanonicalCandidate(tx.QueryRow(ctx, canonicalCandidateSelectForUpdateSQL, userID, candidateID))
	if err != nil {
		return CanonicalKnowledge{}, CanonicalKnowledgeRevision{}, err
	}
	if candidate.Status == "promoted" {
		knowledge, revision, err := existingCanonicalPromotion(ctx, tx, candidate)
		if err != nil {
			return CanonicalKnowledge{}, CanonicalKnowledgeRevision{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return CanonicalKnowledge{}, CanonicalKnowledgeRevision{}, err
		}
		return knowledge, revision, nil
	}
	if !canonicalCandidateEligible(candidate) {
		return CanonicalKnowledge{}, CanonicalKnowledgeRevision{}, canonicalKnowledgeGateError(candidate)
	}
	sources, err := canonicalPromotionSources(ctx, tx, userID, candidateID)
	if err != nil {
		return CanonicalKnowledge{}, CanonicalKnowledgeRevision{}, err
	}
	if len(sources) == 0 {
		return CanonicalKnowledge{}, CanonicalKnowledgeRevision{}, errors.New("canonical promotion requires durable source provenance")
	}
	knowledgeID := canonicalKnowledgeID(candidate)
	now := canonicalNow()
	state, stateErr := loadCanonicalObjectState(ctx, tx, userID, knowledgeID)
	if stateErr != nil && !errors.Is(stateErr, pgx.ErrNoRows) {
		return CanonicalKnowledge{}, CanonicalKnowledgeRevision{}, stateErr
	}
	var revision CanonicalKnowledgeRevision
	if errors.Is(stateErr, pgx.ErrNoRows) {
		if err := insertCanonicalObject(ctx, tx, candidate, knowledgeID, now); err != nil {
			return CanonicalKnowledge{}, CanonicalKnowledgeRevision{}, err
		}
		revision = canonicalRevisionFromCandidate(candidate, knowledgeID, 1, now)
		if err := insertCanonicalRevision(ctx, tx, revision); err != nil {
			return CanonicalKnowledge{}, CanonicalKnowledgeRevision{}, err
		}
		state = canonicalKnowledgeState{KnowledgeID: knowledgeID, Status: KnowledgeStatusActive, ActiveRevisionID: revision.RevisionID, Confidence: candidate.Confidence, Importance: candidate.Importance, ValidFrom: now, CreatedAt: now, UpdatedAt: now}
	} else {
		if state.Status == KnowledgeStatusRevoked || state.Status == KnowledgeStatusConflicted {
			return CanonicalKnowledge{}, CanonicalKnowledgeRevision{}, errors.New("canonical knowledge is not revision-eligible: " + state.Status)
		}
		if state.ActiveRevisionID == "" {
			return CanonicalKnowledge{}, CanonicalKnowledgeRevision{}, errors.New("canonical knowledge has no active revision")
		}
		current, err := loadCanonicalRevision(ctx, tx, userID, knowledgeID, state.ActiveRevisionID)
		if err != nil {
			return CanonicalKnowledge{}, CanonicalKnowledgeRevision{}, err
		}
		if current.SemanticFingerprint == candidate.SemanticFingerprint {
			revision = current
		} else {
			if err := closeCanonicalRevision(ctx, tx, userID, current.RevisionID, now); err != nil {
				return CanonicalKnowledge{}, CanonicalKnowledgeRevision{}, err
			}
			revision = canonicalRevisionFromCandidate(candidate, knowledgeID, current.RevisionNumber+1, now)
			if err := insertCanonicalRevision(ctx, tx, revision); err != nil {
				return CanonicalKnowledge{}, CanonicalKnowledgeRevision{}, err
			}
		}
		state.Status = KnowledgeStatusActive
		state.ActiveRevisionID = revision.RevisionID
		if candidate.Confidence > state.Confidence {
			state.Confidence = candidate.Confidence
		}
		if candidate.Importance > state.Importance {
			state.Importance = candidate.Importance
		}
		state.ValidUntil = 0
		state.UpdatedAt = now
	}
	if err := activateCanonicalRevision(ctx, tx, candidate, knowledgeID, revision.RevisionID, now); err != nil {
		return CanonicalKnowledge{}, CanonicalKnowledgeRevision{}, err
	}
	if err := insertCanonicalProvenance(ctx, tx, candidate, knowledgeID, revision.RevisionID, sources, now); err != nil {
		return CanonicalKnowledge{}, CanonicalKnowledgeRevision{}, err
	}
	if err := markCanonicalCandidatePromoted(ctx, tx, userID, candidateID, now); err != nil {
		return CanonicalKnowledge{}, CanonicalKnowledgeRevision{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CanonicalKnowledge{}, CanonicalKnowledgeRevision{}, err
	}
	state.ActiveRevisionID = revision.RevisionID
	state.Status = KnowledgeStatusActive
	state.UpdatedAt = now
	return canonicalKnowledgeFromState(candidate, state), revision, nil
}

func canonicalNow() int64 {
	return time.Now().UnixMilli()
}
