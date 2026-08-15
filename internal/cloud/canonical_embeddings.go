package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	longmemory "github.com/0xmarkhydra/codelocal/internal/memory"
	"github.com/jackc/pgx/v5"
)

const canonicalKnowledgeEmbeddingVectorSchemaSQL = `
CREATE EXTENSION IF NOT EXISTS vector;
CREATE TABLE IF NOT EXISTS codelocal_knowledge_embeddings (
 user_id TEXT NOT NULL,
 project_id TEXT NOT NULL,
 knowledge_id TEXT NOT NULL,
 revision_id TEXT NOT NULL,
 provider TEXT NOT NULL,
 model TEXT NOT NULL,
 model_version TEXT NOT NULL,
 dimensions INTEGER NOT NULL CHECK (dimensions > 0 AND dimensions <= 8192),
 content_hash TEXT NOT NULL,
 embedding vector NOT NULL,
 embedded_at BIGINT NOT NULL,
 PRIMARY KEY(user_id,project_id,revision_id,provider,model,model_version),
 FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE,
 FOREIGN KEY(user_id,knowledge_id) REFERENCES codelocal_knowledge_objects(user_id,knowledge_id) ON DELETE CASCADE,
 FOREIGN KEY(user_id,revision_id) REFERENCES codelocal_knowledge_revisions(user_id,revision_id) ON DELETE CASCADE,
 CHECK (BTRIM(provider) <> ''),
 CHECK (BTRIM(model) <> ''),
 CHECK (BTRIM(model_version) <> ''),
 CHECK (BTRIM(content_hash) <> '')
);
CREATE INDEX IF NOT EXISTS idx_codelocal_knowledge_embeddings_revision
 ON codelocal_knowledge_embeddings(user_id,project_id,revision_id,embedded_at DESC);
CREATE INDEX IF NOT EXISTS idx_codelocal_knowledge_embeddings_model
 ON codelocal_knowledge_embeddings(user_id,project_id,provider,model,model_version,dimensions);
`

const canonicalKnowledgeEmbeddingMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_knowledge_embedding_projection_state (
 user_id TEXT NOT NULL,
 project_id TEXT NOT NULL,
 provider TEXT NOT NULL,
 model TEXT NOT NULL,
 model_version TEXT NOT NULL,
 dimensions INTEGER NOT NULL CHECK (dimensions >= 0 AND dimensions <= 8192),
 source_revision_count INTEGER NOT NULL DEFAULT 0 CHECK (source_revision_count >= 0),
 source_updated_at BIGINT NOT NULL DEFAULT 0,
 projected_revision_count INTEGER NOT NULL DEFAULT 0 CHECK (projected_revision_count >= 0),
 projected_at BIGINT NOT NULL,
 PRIMARY KEY(user_id,project_id,provider,model,model_version),
 FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_knowledge_embedding_projection_state_projected
 ON codelocal_knowledge_embedding_projection_state(projected_at DESC);
`

const canonicalEmbeddingSourceSQL = `
SELECT o.knowledge_id,o.active_revision_id,o.knowledge_type,o.stable_key,
 r.summary,r.confidence,r.importance,GREATEST(o.updated_at,r.created_at)::bigint
FROM codelocal_knowledge_objects o
JOIN codelocal_knowledge_revisions r
 ON r.user_id=o.user_id AND r.knowledge_id=o.knowledge_id AND r.revision_id=o.active_revision_id
WHERE o.user_id=$1 AND o.project_id=$2
 AND o.privacy_classification='private_project'
 AND o.status='active'
ORDER BY GREATEST(o.updated_at,r.created_at) DESC,o.knowledge_id ASC
LIMIT $3`

const canonicalEmbeddingUpsertSQL = `
INSERT INTO codelocal_knowledge_embeddings(
 user_id,project_id,knowledge_id,revision_id,provider,model,model_version,dimensions,content_hash,embedding,embedded_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::vector,$11)
ON CONFLICT(user_id,project_id,revision_id,provider,model,model_version) DO UPDATE SET
 knowledge_id=EXCLUDED.knowledge_id,dimensions=EXCLUDED.dimensions,content_hash=EXCLUDED.content_hash,
 embedding=EXCLUDED.embedding,embedded_at=EXCLUDED.embedded_at
WHERE codelocal_knowledge_embeddings.content_hash IS DISTINCT FROM EXCLUDED.content_hash
 OR codelocal_knowledge_embeddings.dimensions IS DISTINCT FROM EXCLUDED.dimensions`

const canonicalEmbeddingStateUpsertSQL = `
INSERT INTO codelocal_knowledge_embedding_projection_state(
 user_id,project_id,provider,model,model_version,dimensions,source_revision_count,source_updated_at,projected_revision_count,projected_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT(user_id,project_id,provider,model,model_version) DO UPDATE SET
 dimensions=EXCLUDED.dimensions,source_revision_count=EXCLUDED.source_revision_count,
 source_updated_at=EXCLUDED.source_updated_at,projected_revision_count=EXCLUDED.projected_revision_count,
 projected_at=EXCLUDED.projected_at`

type CanonicalEmbeddingModel struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Version  string `json:"version"`
}

type CanonicalEmbeddingProvider interface {
	Model() CanonicalEmbeddingModel
	Embed(context.Context, []string) ([][]float32, error)
}

type canonicalEmbeddingSource struct {
	KnowledgeID string
	RevisionID  string
	Text        string
	ContentHash string
	UpdatedAt   int64
}

type CanonicalEmbeddingProjectionStats struct {
	Status              string `json:"status"`
	Provider            string `json:"provider,omitempty"`
	Model               string `json:"model,omitempty"`
	ModelVersion        string `json:"modelVersion,omitempty"`
	Dimensions          int    `json:"dimensions,omitempty"`
	SourceRevisionCount int    `json:"sourceRevisionCount"`
	EmbeddedCount       int    `json:"embeddedCount"`
	ReusedCount         int    `json:"reusedCount"`
	SkippedUnsafeCount  int    `json:"skippedUnsafeCount"`
	SourceUpdatedAt     int64  `json:"sourceUpdatedAt"`
	ProjectedAt         int64  `json:"projectedAt,omitempty"`
}

func canonicalEmbeddingEnabled() bool {
	return collectiveEnvEnabled("CODELOCAL_CANONICAL_EMBEDDINGS")
}

func normalizeCanonicalEmbeddingModel(model CanonicalEmbeddingModel) (CanonicalEmbeddingModel, bool) {
	model.Provider = strings.ToLower(strings.TrimSpace(model.Provider))
	model.Model = strings.TrimSpace(model.Model)
	model.Version = strings.TrimSpace(model.Version)
	if model.Provider == "" || model.Model == "" || model.Version == "" {
		return CanonicalEmbeddingModel{}, false
	}
	for _, value := range []string{model.Provider, model.Model, model.Version} {
		if len(value) > 160 || healthStringContainsSecret(value) {
			return CanonicalEmbeddingModel{}, false
		}
	}
	return model, true
}

func canonicalEmbeddingSourceText(knowledgeType, stableKey, summary string) (string, bool) {
	knowledgeType = longmemory.SanitizeText(knowledgeType, 80)
	rawKey := strings.TrimSpace(stableKey)
	rawSummary := strings.TrimSpace(summary)
	if rawSummary == "" || healthStringContainsSecret(rawSummary) {
		return "", false
	}
	parts := []string{}
	if knowledgeType != "" {
		parts = append(parts, "type: "+knowledgeType)
	}
	if rawKey != "" && !healthSecretLikeKey(rawKey) && !healthStringContainsSecret(rawKey) {
		if key := longmemory.SanitizeText(rawKey, 240); key != "" {
			parts = append(parts, "key: "+key)
		}
	}
	if safeSummary := longmemory.SanitizeText(rawSummary, 1600); safeSummary != "" {
		parts = append(parts, "summary: "+safeSummary)
	}
	text := strings.Join(parts, "\n")
	if text == "" {
		return "", false
	}
	return text, true
}

func canonicalEmbeddingContentHash(text string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(text)))
	return hex.EncodeToString(sum[:])
}

func canonicalEmbeddingVectorLiteral(vector []float32) (string, bool) {
	if len(vector) == 0 || len(vector) > 8192 {
		return "", false
	}
	var builder strings.Builder
	builder.Grow(len(vector) * 10)
	builder.WriteByte('[')
	for index, value := range vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return "", false
		}
		if index > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.FormatFloat(float64(value), 'g', -1, 32))
	}
	builder.WriteByte(']')
	return builder.String(), true
}

func canonicalEmbeddingProjectionLimit() int {
	limit := envInt("CODELOCAL_CANONICAL_EMBEDDING_MAX_REVISIONS_PER_PROJECT", 1000)
	if limit < 50 {
		limit = 50
	}
	if limit > 5000 {
		limit = 5000
	}
	return limit
}

type canonicalEmbeddingQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func readCanonicalEmbeddingSources(ctx context.Context, querier canonicalEmbeddingQuerier, userID, projectID string, limit int) ([]canonicalEmbeddingSource, int, int64, error) {
	rows, err := querier.Query(ctx, canonicalEmbeddingSourceSQL, userID, projectID, limit+1)
	if err != nil {
		return nil, 0, 0, err
	}
	defer rows.Close()
	out := []canonicalEmbeddingSource{}
	skippedUnsafe := 0
	sourceUpdatedAt := int64(0)
	for rows.Next() {
		var knowledgeID, revisionID, knowledgeType, stableKey, summary string
		var confidence, importance float64
		var updatedAt int64
		if err := rows.Scan(&knowledgeID, &revisionID, &knowledgeType, &stableKey, &summary, &confidence, &importance, &updatedAt); err != nil {
			return nil, 0, 0, err
		}
		if updatedAt > sourceUpdatedAt {
			sourceUpdatedAt = updatedAt
		}
		text, safe := canonicalEmbeddingSourceText(knowledgeType, stableKey, summary)
		if !safe {
			skippedUnsafe++
			continue
		}
		out = append(out, canonicalEmbeddingSource{
			KnowledgeID: knowledgeID, RevisionID: revisionID, Text: text,
			ContentHash: canonicalEmbeddingContentHash(text), UpdatedAt: updatedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, err
	}
	if len(out)+skippedUnsafe > limit {
		return nil, skippedUnsafe, sourceUpdatedAt, fmt.Errorf("canonical embedding projection exceeds project revision cap: observed>%d", limit)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt != out[j].UpdatedAt {
			return out[i].UpdatedAt < out[j].UpdatedAt
		}
		return out[i].RevisionID < out[j].RevisionID
	})
	return out, skippedUnsafe, sourceUpdatedAt, nil
}

func existingCanonicalEmbeddingHashes(ctx context.Context, tx pgx.Tx, userID, projectID string, model CanonicalEmbeddingModel) (map[string]string, error) {
	rows, err := tx.Query(ctx, `
SELECT revision_id,content_hash
FROM codelocal_knowledge_embeddings
WHERE user_id=$1 AND project_id=$2 AND provider=$3 AND model=$4 AND model_version=$5`,
		userID, projectID, model.Provider, model.Model, model.Version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var revisionID, contentHash string
		if err := rows.Scan(&revisionID, &contentHash); err != nil {
			return nil, err
		}
		out[revisionID] = contentHash
	}
	return out, rows.Err()
}

func (s *Store) RebuildCanonicalKnowledgeEmbeddingsProject(ctx context.Context, provider CanonicalEmbeddingProvider, userID, projectID string) (CanonicalEmbeddingProjectionStats, error) {
	return s.rebuildCanonicalKnowledgeEmbeddingsProject(ctx, provider, userID, projectID)
}
