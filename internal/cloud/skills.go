package cloud

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/learnedskills"
	longmemory "github.com/0xmarkhydra/codelocal/internal/memory"
)

const portableLearnedSkillsMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_project_learned_skill_contributions (
 user_id TEXT NOT NULL,
 project_id TEXT NOT NULL,
 portable_skill_id TEXT NOT NULL,
 device_id TEXT NOT NULL,
 workspace_id TEXT NOT NULL,
 intent TEXT NOT NULL,
 task_kind TEXT,
 steps JSONB NOT NULL,
 context JSONB,
 context_hash TEXT,
 status TEXT NOT NULL CHECK (status IN ('candidate','trusted')),
 confidence DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (confidence >= 0 AND confidence <= 1),
 success_count INTEGER NOT NULL DEFAULT 0 CHECK (success_count >= 0),
 failure_count INTEGER NOT NULL DEFAULT 0 CHECK (failure_count >= 0),
 updated_at BIGINT NOT NULL,
 last_used_at BIGINT NOT NULL DEFAULT 0,
 PRIMARY KEY(user_id,project_id,portable_skill_id,device_id,workspace_id),
 FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE,
 FOREIGN KEY(user_id,device_id,workspace_id) REFERENCES codelocal_workspaces(user_id,device_id,workspace_id) ON DELETE CASCADE,
 CHECK (BTRIM(portable_skill_id) <> ''),
 CHECK (BTRIM(intent) <> ''),
 CHECK (jsonb_typeof(steps) = 'array')
);
CREATE INDEX IF NOT EXISTS idx_codelocal_project_learned_skill_contributions_project
 ON codelocal_project_learned_skill_contributions(user_id,project_id,portable_skill_id,updated_at DESC);
`

type LearnedSkillMetadata struct {
	ID           string                        `json:"id"`
	Intent       string                        `json:"intent"`
	TaskKind     string                        `json:"taskKind,omitempty"`
	Status       string                        `json:"status"`
	Confidence   float64                       `json:"confidence"`
	SuccessCount int                           `json:"successCount"`
	FailureCount int                           `json:"failureCount"`
	StepCount    int                           `json:"stepCount"`
	UpdatedAt    int64                         `json:"updatedAt"`
	LastUsedAt   int64                         `json:"lastUsedAt"`
	Portable     *learnedskills.PortableRecipe `json:"portable,omitempty"`
}

func sanitizeLearnedSkillMetadata(value LearnedSkillMetadata) (LearnedSkillMetadata, bool) {
	value.ID = strings.TrimSpace(value.ID)
	value.Intent = longmemory.SanitizeText(value.Intent, 500)
	value.TaskKind = longmemory.SanitizeText(value.TaskKind, 80)
	value.Status = strings.ToLower(strings.TrimSpace(value.Status))
	if value.ID == "" || len(value.ID) > 160 || value.Intent == "" {
		return LearnedSkillMetadata{}, false
	}
	if value.Status != "candidate" && value.Status != "trusted" {
		value.Status = "candidate"
	}
	if value.Confidence < 0 || value.Confidence > 1 {
		value.Confidence = 0
	}
	value.SuccessCount = max(0, value.SuccessCount)
	value.FailureCount = max(0, value.FailureCount)
	value.StepCount = max(0, min(200, value.StepCount))
	value.UpdatedAt = max(int64(0), value.UpdatedAt)
	value.LastUsedAt = max(int64(0), value.LastUsedAt)
	return value, true
}

func (s *Store) SyncLearnedSkillMetadata(ctx context.Context, userID, deviceID, workspaceID, projectID string, items []LearnedSkillMetadata) error {
	if s == nil || s.DB == nil {
		return nil
	}
	projectID = strings.TrimSpace(projectID)
	if len(items) > 128 {
		items = items[:128]
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	ids := []string{}
	seen := map[string]struct{}{}
	for _, raw := range items {
		item, ok := sanitizeLearnedSkillMetadata(raw)
		if !ok {
			continue
		}
		if _, exists := seen[item.ID]; exists {
			continue
		}
		seen[item.ID] = struct{}{}
		ids = append(ids, item.ID)
		if _, err := tx.Exec(ctx, `
INSERT INTO codelocal_learned_skill_metadata(
 user_id,device_id,workspace_id,skill_id,intent,task_kind,status,confidence,success_count,failure_count,step_count,updated_at,last_used_at)
VALUES($1,$2,$3,$4,$5,NULLIF($6,''),$7,$8,$9,$10,$11,$12,$13)
ON CONFLICT(user_id,device_id,workspace_id,skill_id) DO UPDATE SET
 intent=EXCLUDED.intent,task_kind=EXCLUDED.task_kind,status=EXCLUDED.status,confidence=EXCLUDED.confidence,
 success_count=EXCLUDED.success_count,failure_count=EXCLUDED.failure_count,step_count=EXCLUDED.step_count,
 updated_at=EXCLUDED.updated_at,last_used_at=EXCLUDED.last_used_at`,
			userID, deviceID, workspaceID, item.ID, item.Intent, item.TaskKind, item.Status, item.Confidence,
			item.SuccessCount, item.FailureCount, item.StepCount, item.UpdatedAt, item.LastUsedAt); err != nil {
			return err
		}
		if projectID == "" || item.Portable == nil {
			continue
		}
		portable, portableOK := learnedskills.NormalizePortableRecipe(*item.Portable, projectID)
		if !portableOK {
			continue
		}
		stepsJSON, marshalErr := json.Marshal(portable.Steps)
		if marshalErr != nil {
			continue
		}
		contextJSON, marshalErr := json.Marshal(portable.Context)
		if marshalErr != nil {
			continue
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO codelocal_project_learned_skill_contributions(
 user_id,project_id,portable_skill_id,device_id,workspace_id,intent,task_kind,steps,context,context_hash,status,confidence,success_count,failure_count,updated_at,last_used_at)
VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8::jsonb,$9::jsonb,NULLIF($10,''),$11,$12,$13,$14,$15,$16)
ON CONFLICT(user_id,project_id,portable_skill_id,device_id,workspace_id) DO UPDATE SET
 intent=EXCLUDED.intent,task_kind=EXCLUDED.task_kind,steps=EXCLUDED.steps,context=EXCLUDED.context,context_hash=EXCLUDED.context_hash,
 status=EXCLUDED.status,confidence=EXCLUDED.confidence,success_count=EXCLUDED.success_count,failure_count=EXCLUDED.failure_count,
 updated_at=EXCLUDED.updated_at,last_used_at=EXCLUDED.last_used_at
WHERE EXCLUDED.updated_at >= codelocal_project_learned_skill_contributions.updated_at`,
			userID, projectID, portable.ID, deviceID, workspaceID, portable.Intent, portable.TaskKind,
			string(stepsJSON), string(contextJSON), portable.ContextHash, portable.Status, portable.Confidence,
			portable.SuccessCount, portable.FailureCount, portable.UpdatedAt, portable.LastUsedAt); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
DELETE FROM codelocal_learned_skill_metadata
WHERE user_id=$1 AND device_id=$2 AND workspace_id=$3 AND NOT(skill_id=ANY($4::text[]))`, userID, deviceID, workspaceID, ids); err != nil {
		return err
	}
	// Portable contributions intentionally are not globally deleted here. A
	// second device may still provide evidence for the same project-level skill;
	// contributor expiry/revocation is a separate consolidation concern.
	return tx.Commit(ctx)
}

func portableSkillEvidenceCutoff() int64 {
	days := envInt("CODELOCAL_PORTABLE_SKILL_EVIDENCE_MAX_AGE_DAYS", 180)
	return time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()
}

func portableSkillHealth(contributors, successes, failures int, averageConfidence float64) string {
	if contributors <= 0 {
		return "stale"
	}
	if contributors == 1 {
		return "single_source"
	}
	total := successes + failures
	failureRatio := float64(0)
	if total > 0 {
		failureRatio = float64(failures) / float64(total)
	}
	if failureRatio >= 0.35 || averageConfidence < 0.45 {
		return "degraded"
	}
	if contributors >= 2 && successes >= 4 && averageConfidence >= 0.75 && failureRatio <= 0.20 {
		return "corroborated"
	}
	return "collecting"
}

const projectPortableLearnedSkillsSQL = `
WITH per_device_all AS (
 SELECT DISTINCT ON(portable_skill_id,device_id)
  portable_skill_id,device_id,intent,task_kind,steps,context,context_hash,status,confidence,success_count,failure_count,updated_at,last_used_at
 FROM codelocal_project_learned_skill_contributions
 WHERE user_id=$1 AND project_id=$2
 ORDER BY portable_skill_id,device_id,(status='trusted') DESC,confidence DESC,success_count DESC,updated_at DESC
), fresh AS (
 SELECT * FROM per_device_all WHERE updated_at >= $3
), aggregates AS (
 SELECT portable_skill_id,
  COUNT(*)::int AS contributor_count,
  COUNT(*) FILTER (WHERE status='trusted')::int AS trusted_contributor_count,
  COALESCE(SUM(success_count),0)::int AS aggregate_success_count,
  COALESCE(SUM(failure_count),0)::int AS aggregate_failure_count,
  COALESCE(AVG(confidence),0)::double precision AS average_confidence
 FROM fresh
 GROUP BY portable_skill_id
), best AS (
 SELECT DISTINCT ON(portable_skill_id)
  portable_skill_id,intent,task_kind,steps,context,context_hash,status,confidence,success_count,failure_count,updated_at,last_used_at
 FROM per_device_all
 ORDER BY portable_skill_id,(status='trusted') DESC,confidence DESC,success_count DESC,updated_at DESC
)
SELECT best.portable_skill_id,best.intent,COALESCE(best.task_kind,''),best.steps,best.context,COALESCE(best.context_hash,''),
 best.status,best.confidence,best.success_count,best.failure_count,best.updated_at,best.last_used_at,
 COALESCE(aggregates.contributor_count,0),COALESCE(aggregates.trusted_contributor_count,0),
 COALESCE(aggregates.aggregate_success_count,0),COALESCE(aggregates.aggregate_failure_count,0),
 COALESCE(aggregates.average_confidence,0)
FROM best
LEFT JOIN aggregates USING(portable_skill_id)
ORDER BY best.updated_at DESC,best.portable_skill_id
LIMIT $4`

func (s *Store) ProjectPortableLearnedSkills(ctx context.Context, userID, projectID string, limit int) ([]learnedskills.PortableRecipe, error) {
	if s == nil || s.DB == nil || strings.TrimSpace(userID) == "" || strings.TrimSpace(projectID) == "" {
		return nil, nil
	}
	if limit < 1 {
		limit = 32
	}
	if limit > 128 {
		limit = 128
	}
	rows, err := s.DB.Query(ctx, projectPortableLearnedSkillsSQL, userID, projectID, portableSkillEvidenceCutoff(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]learnedskills.PortableRecipe, 0, limit)
	for rows.Next() {
		var item learnedskills.PortableRecipe
		var stepsJSON, contextJSON []byte
		var evidence learnedskills.PortableEvidence
		if err := rows.Scan(
			&item.ID, &item.Intent, &item.TaskKind, &stepsJSON, &contextJSON, &item.ContextHash,
			&item.Status, &item.Confidence, &item.SuccessCount, &item.FailureCount, &item.UpdatedAt, &item.LastUsedAt,
			&evidence.ContributorCount, &evidence.TrustedContributorCount, &evidence.SuccessCount,
			&evidence.FailureCount, &evidence.AverageConfidence,
		); err != nil {
			return nil, err
		}
		evidence.Health = portableSkillHealth(evidence.ContributorCount, evidence.SuccessCount, evidence.FailureCount, evidence.AverageConfidence)
		item.Version = learnedskills.PortableVersion
		item.ProjectID = projectID
		if json.Unmarshal(stepsJSON, &item.Steps) != nil {
			continue
		}
		if len(contextJSON) > 0 && string(contextJSON) != "null" && json.Unmarshal(contextJSON, &item.Context) != nil {
			continue
		}
		if normalized, ok := learnedskills.NormalizePortableRecipe(item, projectID); ok {
			normalized.Evidence = &evidence
			out = append(out, normalized)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
