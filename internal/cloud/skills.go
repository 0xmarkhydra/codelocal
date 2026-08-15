package cloud

import (
	"context"
	"strings"

	longmemory "github.com/0xmarkhydra/codelocal/internal/memory"
)

type LearnedSkillMetadata struct {
	ID           string  `json:"id"`
	Intent       string  `json:"intent"`
	TaskKind     string  `json:"taskKind,omitempty"`
	Status       string  `json:"status"`
	Confidence   float64 `json:"confidence"`
	SuccessCount int     `json:"successCount"`
	FailureCount int     `json:"failureCount"`
	StepCount    int     `json:"stepCount"`
	UpdatedAt    int64   `json:"updatedAt"`
	LastUsedAt   int64   `json:"lastUsedAt"`
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

func (s *Store) SyncLearnedSkillMetadata(ctx context.Context, userID, deviceID, workspaceID string, items []LearnedSkillMetadata) error {
	if s == nil || s.DB == nil {
		return nil
	}
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
	}
	if _, err := tx.Exec(ctx, `
DELETE FROM codelocal_learned_skill_metadata
WHERE user_id=$1 AND device_id=$2 AND workspace_id=$3 AND NOT(skill_id=ANY($4::text[]))`, userID, deviceID, workspaceID, ids); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
