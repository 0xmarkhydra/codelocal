package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	longmemory "github.com/0xmarkhydra/codelocal/internal/memory"
)

type Experience struct {
	UserID              string         `json:"userId"`
	ExperienceID        string         `json:"experienceId"`
	ProjectID           string         `json:"projectId,omitempty"`
	RepositoryID        string         `json:"repositoryId,omitempty"`
	WorkspaceID         string         `json:"workspaceId,omitempty"`
	DeviceID            string         `json:"deviceId,omitempty"`
	TaskID              string         `json:"taskId,omitempty"`
	TaskKind            string         `json:"taskKind,omitempty"`
	Objective           string         `json:"objective"`
	Branch              string         `json:"branch,omitempty"`
	Files               []string       `json:"files,omitempty"`
	Symbols             []string       `json:"symbols,omitempty"`
	Checks              []string       `json:"checks,omitempty"`
	Outcome             string         `json:"outcome"`
	RootCause           string         `json:"rootCause,omitempty"`
	SkillID             string         `json:"skillId,omitempty"`
	RulesHash           string         `json:"rulesHash,omitempty"`
	ContextHash         string         `json:"contextHash,omitempty"`
	VerificationSummary string         `json:"verificationSummary"`
	CreatedAt           int64          `json:"createdAt"`
	Metadata            map[string]any `json:"metadata,omitempty"`
}

type ExperienceInput struct {
	UserID              string
	ProjectID           string
	RepositoryID        string
	WorkspaceID         string
	DeviceID            string
	TaskID              string
	TaskKind            string
	Objective           string
	Branch              string
	Files               []string
	Symbols             []string
	Checks              []string
	Outcome             string
	RootCause           string
	SkillID             string
	RulesHash           string
	ContextHash         string
	VerificationSummary string
	Verified            bool
	IdempotencyKey      string
	Metadata            map[string]any
}

func normalizeExperienceInput(input ExperienceInput) (ExperienceInput, error) {
	input.UserID = strings.TrimSpace(input.UserID)
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.RepositoryID = strings.TrimSpace(input.RepositoryID)
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	input.TaskID = strings.TrimSpace(input.TaskID)
	input.TaskKind = longmemory.SanitizeText(input.TaskKind, 120)
	input.Objective = longmemory.SanitizeText(input.Objective, 1200)
	input.Branch = longmemory.SanitizeText(input.Branch, 200)
	input.Files = longmemory.SanitizeList(input.Files, 80)
	input.Symbols = longmemory.SanitizeList(input.Symbols, 80)
	input.Checks = longmemory.SanitizeList(input.Checks, 40)
	input.Outcome = strings.ToLower(strings.TrimSpace(input.Outcome))
	input.RootCause = longmemory.SanitizeText(input.RootCause, 900)
	input.SkillID = longmemory.SanitizeText(input.SkillID, 160)
	input.RulesHash = longmemory.SanitizeText(input.RulesHash, 160)
	input.ContextHash = longmemory.SanitizeText(input.ContextHash, 160)
	input.VerificationSummary = longmemory.SanitizeText(input.VerificationSummary, 1200)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.Metadata = sanitizePromotionCandidateMetadata(input.Metadata)
	if !input.Verified {
		return ExperienceInput{}, errors.New("experience requires verified evidence")
	}
	if input.UserID == "" || input.Objective == "" || input.VerificationSummary == "" {
		return ExperienceInput{}, errors.New("experience requires user, objective and verification summary")
	}
	if input.Outcome != "succeeded" && input.Outcome != "failed" {
		return ExperienceInput{}, errors.New("experience outcome must be succeeded or failed")
	}
	return input, nil
}

func experienceID(input ExperienceInput) string {
	key := input.IdempotencyKey
	if key == "" {
		key = longmemory.IdempotencyKey(input.UserID, input.ProjectID, input.RepositoryID, input.TaskID, input.Objective, input.Outcome, input.VerificationSummary)
	}
	return knowledgeStableID("exp_", input.UserID, key)
}

func experienceJSON(value any) []byte {
	raw, _ := json.Marshal(value)
	if len(raw) == 0 {
		return []byte("null")
	}
	return raw
}

func (s *Store) RecordExperience(ctx context.Context, input ExperienceInput) (Experience, error) {
	if s == nil || s.DB == nil {
		return Experience{}, errors.New("experience store unavailable")
	}
	normalized, err := normalizeExperienceInput(input)
	if err != nil {
		return Experience{}, err
	}
	createdAt := time.Now().UnixMilli()
	experience := Experience{
		UserID: normalized.UserID, ExperienceID: experienceID(normalized), ProjectID: normalized.ProjectID,
		RepositoryID: normalized.RepositoryID, WorkspaceID: normalized.WorkspaceID, DeviceID: normalized.DeviceID,
		TaskID: normalized.TaskID, TaskKind: normalized.TaskKind, Objective: normalized.Objective, Branch: normalized.Branch,
		Files: normalized.Files, Symbols: normalized.Symbols, Checks: normalized.Checks, Outcome: normalized.Outcome,
		RootCause: normalized.RootCause, SkillID: normalized.SkillID, RulesHash: normalized.RulesHash, ContextHash: normalized.ContextHash,
		VerificationSummary: normalized.VerificationSummary, CreatedAt: createdAt, Metadata: normalized.Metadata,
	}
	_, err = s.DB.Exec(ctx, `
INSERT INTO codelocal_experiences(
 user_id,experience_id,project_id,repository_id,workspace_id,device_id,task_id,task_kind,objective,branch,
 files,symbols,checks,outcome,root_cause,skill_id,rules_hash,context_hash,verification_summary,created_at,metadata)
VALUES($1,$2,NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),$9,NULLIF($10,''),$11,$12,$13,$14,NULLIF($15,''),NULLIF($16,''),NULLIF($17,''),NULLIF($18,''),$19,$20,$21)
ON CONFLICT(user_id,experience_id) DO NOTHING`,
		experience.UserID, experience.ExperienceID, experience.ProjectID, experience.RepositoryID, experience.WorkspaceID, experience.DeviceID,
		experience.TaskID, experience.TaskKind, experience.Objective, experience.Branch, experienceJSON(experience.Files), experienceJSON(experience.Symbols),
		experienceJSON(experience.Checks), experience.Outcome, experience.RootCause, experience.SkillID, experience.RulesHash, experience.ContextHash,
		experience.VerificationSummary, experience.CreatedAt, knowledgeMetadata(experience.Metadata))
	if err != nil {
		return Experience{}, err
	}
	// Collective learning is a privacy-gated derivative of verified Experience.
	// It is never allowed to fail or delay the private Experience path when the
	// feature is disabled, the user has not opted in, or the derivative fails.
	if recorded, collectiveErr := s.RecordCollectiveExperience(ctx, experience); collectiveErr != nil {
		slog.Debug("collective experience contribution skipped", "error", collectiveErr)
	} else if recorded {
		slog.Debug("collective experience contribution recorded")
	}
	return experience, nil
}
