package cloud

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type canonicalEmbeddingExisting struct {
	hashes    map[string]string
	dimension int
}

type canonicalEmbeddedVector struct {
	literal   string
	dimension int
}

func canonicalEmbeddingBatchSize() int {
	value := envInt("CODELOCAL_CANONICAL_EMBEDDING_BATCH_SIZE", 16)
	if value < 1 {
		return 1
	}
	if value > 64 {
		return 64
	}
	return value
}

func loadCanonicalEmbeddingExisting(ctx context.Context, s *Store, userID, projectID string, model CanonicalEmbeddingModel) (canonicalEmbeddingExisting, error) {
	rows, err := s.DB.Query(ctx, `SELECT revision_id,content_hash,dimensions FROM codelocal_knowledge_embeddings WHERE user_id=$1 AND project_id=$2 AND provider=$3 AND model=$4 AND model_version=$5`, userID, projectID, model.Provider, model.Model, model.Version)
	if err != nil {
		return canonicalEmbeddingExisting{}, err
	}
	defer rows.Close()
	state := canonicalEmbeddingExisting{hashes: map[string]string{}}
	for rows.Next() {
		var revisionID, hash string
		var dimension int
		if err := rows.Scan(&revisionID, &hash, &dimension); err != nil {
			return canonicalEmbeddingExisting{}, err
		}
		if state.dimension != 0 && state.dimension != dimension {
			return canonicalEmbeddingExisting{}, errors.New("canonical embedding cohort contains mixed dimensions")
		}
		state.dimension, state.hashes[revisionID] = dimension, hash
	}
	return state, rows.Err()
}

func splitCanonicalEmbeddingPending(sources []canonicalEmbeddingSource, existing canonicalEmbeddingExisting) ([]canonicalEmbeddingSource, int) {
	pending := make([]canonicalEmbeddingSource, 0, len(sources))
	reused := 0
	for _, source := range sources {
		if existing.hashes[source.RevisionID] == source.ContentHash {
			reused++
		} else {
			pending = append(pending, source)
		}
	}
	return pending, reused
}

func canonicalEmbeddingTexts(sources []canonicalEmbeddingSource) []string {
	texts := make([]string, len(sources))
	for index := range sources {
		texts[index] = sources[index].Text
	}
	return texts
}

func validateCanonicalEmbeddingBatch(vectors [][]float32, expected, existingDimension int) (map[int]canonicalEmbeddedVector, int, error) {
	if len(vectors) != expected {
		return nil, 0, errors.New("canonical embedding provider returned incomplete batch")
	}
	out, dimension := map[int]canonicalEmbeddedVector{}, existingDimension
	for index, vector := range vectors {
		literal, valid := canonicalEmbeddingVectorLiteral(vector)
		if !valid {
			return nil, 0, errors.New("canonical embedding provider returned invalid vector")
		}
		if dimension != 0 && dimension != len(vector) {
			return nil, 0, fmt.Errorf("canonical embedding dimension changed within cohort: %d -> %d", dimension, len(vector))
		}
		dimension = len(vector)
		out[index] = canonicalEmbeddedVector{literal: literal, dimension: len(vector)}
	}
	return out, dimension, nil
}

func embedCanonicalPending(ctx context.Context, provider CanonicalEmbeddingProvider, pending []canonicalEmbeddingSource, existingDimension int) (map[string]canonicalEmbeddedVector, int, error) {
	out, dimension := map[string]canonicalEmbeddedVector{}, existingDimension
	batchSize := canonicalEmbeddingBatchSize()
	for start := 0; start < len(pending); start += batchSize {
		end := min(start+batchSize, len(pending))
		batch := pending[start:end]
		vectors, err := provider.Embed(ctx, canonicalEmbeddingTexts(batch))
		if err != nil {
			return nil, 0, err
		}
		validated, nextDimension, err := validateCanonicalEmbeddingBatch(vectors, len(batch), dimension)
		if err != nil {
			return nil, 0, err
		}
		dimension = nextDimension
		for index, source := range batch {
			out[source.RevisionID] = validated[index]
		}
	}
	return out, dimension, nil
}

func sameCanonicalEmbeddingSources(left, right []canonicalEmbeddingSource) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].RevisionID != right[index].RevisionID || left[index].ContentHash != right[index].ContentHash {
			return false
		}
	}
	return true
}

func canonicalEmbeddingActiveRevisionIDs(sources []canonicalEmbeddingSource) []string {
	ids := make([]string, len(sources))
	for index := range sources {
		ids[index] = sources[index].RevisionID
	}
	return ids
}

func writeCanonicalEmbeddingProjection(ctx context.Context, s *Store, model CanonicalEmbeddingModel, userID, projectID string, sources []canonicalEmbeddingSource, vectors map[string]canonicalEmbeddedVector, totalSources, skippedUnsafe, dimension int, sourceUpdatedAt int64) (int64, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	current, currentSkipped, currentUpdatedAt, err := readCanonicalEmbeddingSources(ctx, tx, userID, projectID, canonicalEmbeddingProjectionLimit())
	if err != nil {
		return 0, err
	}
	if currentSkipped != skippedUnsafe || currentUpdatedAt != sourceUpdatedAt || !sameCanonicalEmbeddingSources(current, sources) {
		return 0, errors.New("canonical embedding source changed during projection")
	}
	embeddedAt := time.Now().UnixMilli()
	for _, source := range sources {
		vector, changed := vectors[source.RevisionID]
		if !changed {
			continue
		}
		if _, err := tx.Exec(ctx, canonicalEmbeddingUpsertSQL, userID, projectID, source.KnowledgeID, source.RevisionID, model.Provider, model.Model, model.Version, vector.dimension, source.ContentHash, vector.literal, embeddedAt); err != nil {
			return 0, err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM codelocal_knowledge_embeddings WHERE user_id=$1 AND project_id=$2 AND provider=$3 AND model=$4 AND model_version=$5 AND NOT(revision_id=ANY($6::text[]))`, userID, projectID, model.Provider, model.Model, model.Version, canonicalEmbeddingActiveRevisionIDs(sources)); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, canonicalEmbeddingStateUpsertSQL, userID, projectID, model.Provider, model.Model, model.Version, dimension, totalSources, sourceUpdatedAt, len(sources), embeddedAt); err != nil {
		return 0, err
	}
	return embeddedAt, tx.Commit(ctx)
}

func (s *Store) rebuildCanonicalKnowledgeEmbeddingsProject(ctx context.Context, provider CanonicalEmbeddingProvider, userID, projectID string) (CanonicalEmbeddingProjectionStats, error) {
	stats := CanonicalEmbeddingProjectionStats{Status: "disabled"}
	if !canonicalEmbeddingEnabled() {
		return stats, nil
	}
	if s == nil || s.DB == nil || provider == nil || strings.TrimSpace(userID) == "" || strings.TrimSpace(projectID) == "" {
		return stats, errors.New("canonical embedding projection requires store, provider, user and project")
	}
	model, ok := normalizeCanonicalEmbeddingModel(provider.Model())
	if !ok {
		return stats, errors.New("canonical embedding provider metadata is invalid")
	}
	stats.Provider, stats.Model, stats.ModelVersion = model.Provider, model.Model, model.Version
	sources, skipped, updatedAt, err := readCanonicalEmbeddingSources(ctx, s.DB, userID, projectID, canonicalEmbeddingProjectionLimit())
	if err != nil {
		return stats, err
	}
	stats.SourceRevisionCount, stats.SkippedUnsafeCount, stats.SourceUpdatedAt = len(sources)+skipped, skipped, updatedAt
	existing, err := loadCanonicalEmbeddingExisting(ctx, s, userID, projectID, model)
	if err != nil {
		return stats, err
	}
	pending, reused := splitCanonicalEmbeddingPending(sources, existing)
	stats.ReusedCount = reused
	vectors, dimension, err := embedCanonicalPending(ctx, provider, pending, existing.dimension)
	if err != nil {
		return stats, err
	}
	if len(sources) > 0 && dimension <= 0 {
		return stats, errors.New("canonical embedding projection could not resolve vector dimensions")
	}
	projectedAt, err := writeCanonicalEmbeddingProjection(ctx, s, model, userID, projectID, sources, vectors, len(sources)+skipped, skipped, dimension, updatedAt)
	if err != nil {
		return stats, err
	}
	stats.Status, stats.Dimensions, stats.EmbeddedCount, stats.ProjectedAt = "current", dimension, len(pending), projectedAt
	return stats, nil
}
