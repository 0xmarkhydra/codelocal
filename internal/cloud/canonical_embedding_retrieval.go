package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	longmemory "github.com/0xmarkhydra/codelocal/internal/memory"
)

const canonicalEmbeddingRecallSQL = `
SELECT
 o.user_id,o.knowledge_id,o.project_id,COALESCE(o.repository_id,''),COALESCE(o.branch,''),o.knowledge_type,o.stable_key,o.cardinality,o.qualifier,
 o.status,o.privacy_classification,COALESCE(o.active_revision_id,''),o.confidence,o.importance,o.valid_from,COALESCE(o.valid_until,0),o.created_at,o.updated_at,
 r.revision_id,r.revision_number,r.subject,r.predicate,r.object,r.summary,r.semantic_fingerprint,r.confidence,r.importance,r.valid_from,COALESCE(r.valid_until,0),r.created_at,
 1-(e.embedding <=> $7::vector) AS similarity
FROM codelocal_knowledge_embeddings e
JOIN codelocal_knowledge_objects o
 ON o.user_id=e.user_id AND o.knowledge_id=e.knowledge_id AND o.project_id=e.project_id AND o.active_revision_id=e.revision_id
JOIN codelocal_knowledge_revisions r
 ON r.user_id=o.user_id AND r.knowledge_id=o.knowledge_id AND r.revision_id=e.revision_id
WHERE e.user_id=$1 AND e.project_id=$2
 AND e.provider=$3 AND e.model=$4 AND e.model_version=$5 AND e.dimensions=$6
 AND o.status='active' AND o.privacy_classification='private_project'
 AND o.valid_from <= $10 AND o.valid_until IS NULL
 AND r.valid_from <= $10 AND r.valid_until IS NULL
 AND (o.repository_id IS NULL OR o.repository_id=ANY($8::text[]))
 AND (o.branch IS NULL OR ($9<>'' AND o.branch=$9))
ORDER BY e.embedding <=> $7::vector ASC,o.confidence DESC,o.importance DESC,o.knowledge_id ASC
LIMIT $11`

type canonicalEmbeddingQueryProvider interface {
	EmbedQuery(context.Context, string) ([]float32, error)
}

type CanonicalSemanticKnowledgeHit struct {
	CanonicalKnowledgeHit
	Similarity float64 `json:"similarity"`
}

func scanCanonicalSemanticKnowledgeHit(row interface{ Scan(...any) error }) (CanonicalSemanticKnowledgeHit, error) {
	var result CanonicalSemanticKnowledgeHit
	var subjectJSON, objectJSON []byte
	hit := &result.CanonicalKnowledgeHit
	err := row.Scan(
		&hit.Knowledge.UserID, &hit.Knowledge.KnowledgeID, &hit.Knowledge.ProjectID, &hit.Knowledge.RepositoryID, &hit.Knowledge.Branch,
		&hit.Knowledge.KnowledgeType, &hit.Knowledge.StableKey, &hit.Knowledge.Cardinality, &hit.Knowledge.Qualifier,
		&hit.Knowledge.Status, &hit.Knowledge.PrivacyClassification, &hit.Knowledge.ActiveRevisionID, &hit.Knowledge.Confidence, &hit.Knowledge.Importance,
		&hit.Knowledge.ValidFrom, &hit.Knowledge.ValidUntil, &hit.Knowledge.CreatedAt, &hit.Knowledge.UpdatedAt,
		&hit.Revision.RevisionID, &hit.Revision.RevisionNumber, &subjectJSON, &hit.Revision.Predicate, &objectJSON, &hit.Revision.Summary,
		&hit.Revision.SemanticFingerprint, &hit.Revision.Confidence, &hit.Revision.Importance, &hit.Revision.ValidFrom, &hit.Revision.ValidUntil, &hit.Revision.CreatedAt,
		&result.Similarity,
	)
	if err != nil {
		return CanonicalSemanticKnowledgeHit{}, err
	}
	hit.Revision.UserID, hit.Revision.KnowledgeID = hit.Knowledge.UserID, hit.Knowledge.KnowledgeID
	if err := json.Unmarshal(subjectJSON, &hit.Revision.Subject); err != nil {
		return CanonicalSemanticKnowledgeHit{}, err
	}
	if err := json.Unmarshal(objectJSON, &hit.Revision.Object); err != nil {
		return CanonicalSemanticKnowledgeHit{}, err
	}
	return result, nil
}

func canonicalSemanticRecallLimit(limit int) int {
	if limit <= 0 {
		return 6
	}
	if limit > 16 {
		return 16
	}
	return limit
}

func (s *Store) RecallCanonicalKnowledgeSemantic(ctx context.Context, input CanonicalKnowledgeRecallInput, query string) ([]CanonicalSemanticKnowledgeHit, error) {
	if !canonicalEmbeddingEnabled() || s == nil || s.DB == nil || s.canonicalEmbeddingProvider == nil {
		return nil, errors.New("canonical semantic recall unavailable")
	}
	normalized, err := normalizeCanonicalRecallInput(input)
	if err != nil {
		return nil, err
	}
	query = longmemory.SanitizeText(strings.TrimSpace(query), 1200)
	if query == "" {
		return nil, errors.New("canonical semantic recall query is required")
	}
	freshness, err := s.CanonicalEmbeddingProjectFreshness(ctx, normalized.UserID, normalized.ProjectID)
	if err != nil || freshness.Status == "empty" {
		return nil, err
	}
	if freshness.Status != "current" || freshness.Dimensions <= 0 {
		return nil, errors.New("canonical semantic index is not current")
	}
	queryProvider, ok := s.canonicalEmbeddingProvider.(canonicalEmbeddingQueryProvider)
	if !ok {
		return nil, errors.New("canonical embedding provider does not support query embeddings")
	}
	vector, err := queryProvider.EmbedQuery(ctx, query)
	if err != nil {
		return nil, err
	}
	literal, valid := canonicalEmbeddingVectorLiteral(vector)
	if !valid || len(vector) != freshness.Dimensions {
		return nil, errors.New("canonical query embedding dimension mismatch")
	}
	rows, err := s.DB.Query(ctx, canonicalEmbeddingRecallSQL,
		normalized.UserID, normalized.ProjectID, freshness.Provider, freshness.Model, freshness.ModelVersion, freshness.Dimensions,
		literal, normalized.RepositoryIDs, normalized.Branch, time.Now().UnixMilli(), canonicalSemanticRecallLimit(normalized.Limit),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hits := []CanonicalSemanticKnowledgeHit{}
	for rows.Next() {
		hit, err := scanCanonicalSemanticKnowledgeHit(rows)
		if err != nil {
			return nil, err
		}
		hit.ScopeRank = canonicalScopeRank(hit.Knowledge)
		hit.TypeRank = canonicalTypeRank(hit.Knowledge.KnowledgeType)
		hits = append(hits, hit)
	}
	return hits, rows.Err()
}
