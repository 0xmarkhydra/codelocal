package cloud

import (
	"context"
	"errors"
	"strings"
	"time"
)

const canonicalEmbeddingFreshnessSQL = `
WITH source AS (
 SELECT p.user_id,p.project_id,
        COUNT(o.knowledge_id)::int AS source_revision_count,
        COALESCE(MAX(GREATEST(o.updated_at,r.created_at)),0)::bigint AS source_updated_at
 FROM codelocal_projects p
 LEFT JOIN codelocal_knowledge_objects o
   ON o.user_id=p.user_id AND o.project_id=p.project_id
  AND o.privacy_classification='private_project' AND o.status='active'
 LEFT JOIN codelocal_knowledge_revisions r
   ON r.user_id=o.user_id AND r.knowledge_id=o.knowledge_id AND r.revision_id=o.active_revision_id
 WHERE ($1='' OR p.user_id=$1)
   AND ($5='' OR p.project_id=$5)
 GROUP BY p.user_id,p.project_id
), vectors AS (
 SELECT user_id,project_id,COUNT(*)::int AS vector_count
 FROM codelocal_knowledge_embeddings
 WHERE provider=$2 AND model=$3 AND model_version=$4
 GROUP BY user_id,project_id
)
SELECT s.user_id,s.project_id,s.source_revision_count,s.source_updated_at,
 COALESCE(ps.dimensions,-1),COALESCE(ps.source_revision_count,-1),COALESCE(ps.source_updated_at,-1),
 COALESCE(ps.projected_revision_count,-1),COALESCE(ps.projected_at,0),COALESCE(v.vector_count,0)
FROM source s
LEFT JOIN codelocal_knowledge_embedding_projection_state ps
 ON ps.user_id=s.user_id AND ps.project_id=s.project_id
 AND ps.provider=$2 AND ps.model=$3 AND ps.model_version=$4
LEFT JOIN vectors v ON v.user_id=s.user_id AND v.project_id=s.project_id
ORDER BY s.user_id,s.project_id`

type CanonicalEmbeddingFreshness struct {
	UserID                   string `json:"userId,omitempty"`
	ProjectID                string `json:"projectId"`
	Status                   string `json:"status"`
	Provider                 string `json:"provider,omitempty"`
	Model                    string `json:"model,omitempty"`
	ModelVersion             string `json:"modelVersion,omitempty"`
	Dimensions               int    `json:"dimensions"`
	SourceRevisionCount      int    `json:"sourceRevisionCount"`
	SourceUpdatedAt          int64  `json:"sourceUpdatedAt"`
	ProjectedSourceCount     int    `json:"projectedSourceCount"`
	ProjectedSourceUpdatedAt int64  `json:"projectedSourceUpdatedAt"`
	ProjectedRevisionCount   int    `json:"projectedRevisionCount"`
	VectorCount              int    `json:"vectorCount"`
	ProjectedAt              int64  `json:"projectedAt"`
	LagMS                    int64  `json:"lagMs"`
}

type CanonicalEmbeddingFreshnessSummary struct {
	Status          string `json:"status"`
	ProjectCount    int    `json:"projectCount"`
	CurrentProjects int    `json:"currentProjects"`
	EmptyProjects   int    `json:"emptyProjects"`
	MissingProjects int    `json:"missingProjects"`
	StaleProjects   int    `json:"staleProjects"`
	MaxLagMS        int64  `json:"maxLagMs"`
	ConfigError     string `json:"configError,omitempty"`
}

func classifyCanonicalEmbeddingFreshness(value CanonicalEmbeddingFreshness, now int64) CanonicalEmbeddingFreshness {
	if value.SourceRevisionCount == 0 && value.ProjectedAt == 0 && value.VectorCount == 0 {
		value.Status = "empty"
		return value
	}
	if value.ProjectedAt == 0 || value.ProjectedSourceCount < 0 || value.ProjectedRevisionCount < 0 {
		value.Status = "missing"
		value.LagMS = canonicalEmbeddingLag(now, value.SourceUpdatedAt)
		return value
	}
	if value.ProjectedSourceCount != value.SourceRevisionCount || value.ProjectedSourceUpdatedAt != value.SourceUpdatedAt || value.ProjectedRevisionCount != value.VectorCount {
		value.Status = "stale"
		value.LagMS = canonicalEmbeddingLag(max(value.SourceUpdatedAt, now), value.ProjectedAt)
		return value
	}
	value.Status = "current"
	return value
}

func canonicalEmbeddingLag(newer, older int64) int64 {
	if newer <= older || older <= 0 {
		return 0
	}
	return newer - older
}

func summarizeCanonicalEmbeddingFreshness(values []CanonicalEmbeddingFreshness) CanonicalEmbeddingFreshnessSummary {
	summary := CanonicalEmbeddingFreshnessSummary{Status: "current", ProjectCount: len(values)}
	for _, value := range values {
		switch value.Status {
		case "current":
			summary.CurrentProjects++
		case "empty":
			summary.EmptyProjects++
		case "missing":
			summary.MissingProjects++
			summary.Status = "degraded"
		case "stale":
			summary.StaleProjects++
			summary.Status = "degraded"
		default:
			summary.Status = "degraded"
		}
		if value.LagMS > summary.MaxLagMS {
			summary.MaxLagMS = value.LagMS
		}
	}
	return summary
}

func (s *Store) CanonicalEmbeddingFreshness(ctx context.Context, userID string) ([]CanonicalEmbeddingFreshness, CanonicalEmbeddingFreshnessSummary, error) {
	if !canonicalEmbeddingEnabled() {
		return nil, CanonicalEmbeddingFreshnessSummary{Status: "disabled"}, nil
	}
	if s == nil || s.DB == nil {
		return nil, CanonicalEmbeddingFreshnessSummary{Status: "unavailable"}, errors.New("canonical embedding freshness unavailable")
	}
	if s.canonicalEmbeddingProvider == nil {
		return nil, CanonicalEmbeddingFreshnessSummary{Status: "unavailable", ConfigError: s.canonicalEmbeddingError}, nil
	}
	model, ok := normalizeCanonicalEmbeddingModel(s.canonicalEmbeddingProvider.Model())
	if !ok {
		return nil, CanonicalEmbeddingFreshnessSummary{Status: "unavailable", ConfigError: "invalid provider metadata"}, nil
	}
	rows, err := s.DB.Query(ctx, canonicalEmbeddingFreshnessSQL, strings.TrimSpace(userID), model.Provider, model.Model, model.Version, "")
	if err != nil {
		return nil, CanonicalEmbeddingFreshnessSummary{Status: "unavailable"}, err
	}
	defer rows.Close()
	now := time.Now().UnixMilli()
	values := []CanonicalEmbeddingFreshness{}
	for rows.Next() {
		var value CanonicalEmbeddingFreshness
		value.Provider, value.Model, value.ModelVersion = model.Provider, model.Model, model.Version
		if err := rows.Scan(&value.UserID, &value.ProjectID, &value.SourceRevisionCount, &value.SourceUpdatedAt, &value.Dimensions, &value.ProjectedSourceCount, &value.ProjectedSourceUpdatedAt, &value.ProjectedRevisionCount, &value.ProjectedAt, &value.VectorCount); err != nil {
			return nil, CanonicalEmbeddingFreshnessSummary{Status: "unavailable"}, err
		}
		values = append(values, classifyCanonicalEmbeddingFreshness(value, now))
	}
	if err := rows.Err(); err != nil {
		return nil, CanonicalEmbeddingFreshnessSummary{Status: "unavailable"}, err
	}
	return values, summarizeCanonicalEmbeddingFreshness(values), nil
}

func (s *Store) CanonicalEmbeddingProjectFreshness(ctx context.Context, userID, projectID string) (CanonicalEmbeddingFreshness, error) {
	if !canonicalEmbeddingEnabled() || s == nil || s.DB == nil || s.canonicalEmbeddingProvider == nil {
		return CanonicalEmbeddingFreshness{Status: "unavailable"}, errors.New("canonical embedding project freshness unavailable")
	}
	model, ok := normalizeCanonicalEmbeddingModel(s.canonicalEmbeddingProvider.Model())
	if !ok {
		return CanonicalEmbeddingFreshness{Status: "unavailable"}, errors.New("canonical embedding provider metadata is invalid")
	}
	var value CanonicalEmbeddingFreshness
	value.Provider, value.Model, value.ModelVersion = model.Provider, model.Model, model.Version
	err := s.DB.QueryRow(ctx, canonicalEmbeddingFreshnessSQL, strings.TrimSpace(userID), model.Provider, model.Model, model.Version, strings.TrimSpace(projectID)).Scan(
		&value.UserID, &value.ProjectID, &value.SourceRevisionCount, &value.SourceUpdatedAt, &value.Dimensions, &value.ProjectedSourceCount, &value.ProjectedSourceUpdatedAt, &value.ProjectedRevisionCount, &value.ProjectedAt, &value.VectorCount,
	)
	if err != nil {
		return CanonicalEmbeddingFreshness{Status: "unavailable"}, err
	}
	return classifyCanonicalEmbeddingFreshness(value, time.Now().UnixMilli()), nil
}
