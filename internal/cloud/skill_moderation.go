package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/skills"
	"github.com/jackc/pgx/v5"
)

const MinSkillEvaluationScore = 0.80

type SkillEvaluation struct {
	EvaluationID    string         `json:"evaluationId"`
	SkillID         string         `json:"skillId"`
	Version         string         `json:"version"`
	EvaluatorUserID string         `json:"evaluatorUserId,omitempty"`
	Decision        string         `json:"decision"`
	Score           float64        `json:"score"`
	Checks          map[string]any `json:"checks,omitempty"`
	CreatedAt       int64          `json:"createdAt"`
}

func (s *Store) StartSkillEvaluation(ctx context.Context, skillID, version string) (SkillVersionRecord, error) {
	if s == nil || s.DB == nil {
		return SkillVersionRecord{}, fmt.Errorf("cloud store is unavailable")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return SkillVersionRecord{}, err
	}
	defer tx.Rollback(ctx)
	record, err := sharedSkillVersionForUpdate(ctx, tx, skillID, version)
	if err != nil {
		return SkillVersionRecord{}, err
	}
	if !skillVersionTransitionAllowed(record.State, SkillVersionEvaluating) {
		return SkillVersionRecord{}, fmt.Errorf("skill %s@%s cannot transition from %s to evaluating", record.Manifest.ID, record.Manifest.Version, record.State)
	}
	now := time.Now().UnixMilli()
	if _, err := tx.Exec(ctx, `UPDATE codelocal_skill_versions SET state='evaluating', updated_at=$3 WHERE skill_id=$1 AND version=$2 AND tenant_user_id IS NULL`, record.Manifest.ID, record.Manifest.Version, now); err != nil {
		return SkillVersionRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SkillVersionRecord{}, err
	}
	record.State = SkillVersionEvaluating
	record.UpdatedAt = now
	return record, nil
}

// CompleteSkillEvaluation records immutable moderation evidence and advances a
// shared candidate to canary only when the evaluation passes the minimum score.
// Failed evidence rejects the version; a new immutable version is required to
// retry publication after rejection.
func (s *Store) CompleteSkillEvaluation(ctx context.Context, evaluatorUserID, skillID, version string, passed bool, score float64, checks map[string]any) (SkillVersionRecord, SkillEvaluation, error) {
	if s == nil || s.DB == nil {
		return SkillVersionRecord{}, SkillEvaluation{}, fmt.Errorf("cloud store is unavailable")
	}
	evaluatorUserID = strings.TrimSpace(evaluatorUserID)
	if evaluatorUserID == "" {
		return SkillVersionRecord{}, SkillEvaluation{}, fmt.Errorf("skill evaluation requires evaluator")
	}
	if score < 0 || score > 1 {
		return SkillVersionRecord{}, SkillEvaluation{}, fmt.Errorf("skill evaluation score must be between 0 and 1")
	}
	if checks == nil {
		checks = map[string]any{}
	}
	checksJSON, err := json.Marshal(checks)
	if err != nil {
		return SkillVersionRecord{}, SkillEvaluation{}, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return SkillVersionRecord{}, SkillEvaluation{}, err
	}
	defer tx.Rollback(ctx)
	record, err := sharedSkillVersionForUpdate(ctx, tx, skillID, version)
	if err != nil {
		return SkillVersionRecord{}, SkillEvaluation{}, err
	}
	if record.State != SkillVersionEvaluating {
		return SkillVersionRecord{}, SkillEvaluation{}, fmt.Errorf("skill %s@%s is not evaluating", record.Manifest.ID, record.Manifest.Version)
	}
	decision := "failed"
	nextState := SkillVersionRejected
	if passed && score >= MinSkillEvaluationScore {
		decision = "passed"
		nextState = SkillVersionCanary
	}
	if !skillVersionTransitionAllowed(record.State, nextState) {
		return SkillVersionRecord{}, SkillEvaluation{}, fmt.Errorf("invalid skill evaluation transition %s -> %s", record.State, nextState)
	}
	now := time.Now().UnixMilli()
	evaluation := SkillEvaluation{
		EvaluationID: skillRegistryID("evaluation", record.Manifest.ID, record.Manifest.Version, evaluatorUserID, strconv.FormatInt(time.Now().UnixNano(), 10)),
		SkillID:      record.Manifest.ID, Version: record.Manifest.Version, EvaluatorUserID: evaluatorUserID,
		Decision: decision, Score: score, Checks: checks, CreatedAt: now,
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO codelocal_skill_evaluations(evaluation_id, skill_id, version, evaluator_user_id, decision, score, checks, created_at)
VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,$8)`,
		evaluation.EvaluationID, evaluation.SkillID, evaluation.Version, evaluation.EvaluatorUserID,
		evaluation.Decision, evaluation.Score, string(checksJSON), evaluation.CreatedAt); err != nil {
		return SkillVersionRecord{}, SkillEvaluation{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE codelocal_skill_versions SET state=$3, updated_at=$4 WHERE skill_id=$1 AND version=$2 AND tenant_user_id IS NULL`, record.Manifest.ID, record.Manifest.Version, nextState, now); err != nil {
		return SkillVersionRecord{}, SkillEvaluation{}, err
	}
	if nextState == SkillVersionCanary {
		channelID := skillRegistryID("channel", "", record.Manifest.ID, "canary")
		if _, err := tx.Exec(ctx, `
INSERT INTO codelocal_skill_channels(channel_id, skill_id, tenant_user_id, channel, version, updated_at)
VALUES($1,$2,NULL,'canary',$3,$4)
ON CONFLICT (channel_id) DO UPDATE SET version=EXCLUDED.version, updated_at=EXCLUDED.updated_at`,
			channelID, record.Manifest.ID, record.Manifest.Version, now); err != nil {
			return SkillVersionRecord{}, SkillEvaluation{}, err
		}
	} else {
		if _, err := tx.Exec(ctx, `DELETE FROM codelocal_skill_channels WHERE skill_id=$1 AND channel='canary' AND tenant_user_id IS NULL AND version=$2`, record.Manifest.ID, record.Manifest.Version); err != nil {
			return SkillVersionRecord{}, SkillEvaluation{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return SkillVersionRecord{}, SkillEvaluation{}, err
	}
	record.State = nextState
	record.UpdatedAt = now
	return record, evaluation, nil
}

// PromoteSkillVersion atomically moves a canary (or restores a rolled-back
// version) to stable and rolls the previous promoted version back. The stable
// channel can therefore never point at candidate/evaluating/rejected content.
func (s *Store) PromoteSkillVersion(ctx context.Context, skillID, version string) (SkillVersionRecord, error) {
	if s == nil || s.DB == nil {
		return SkillVersionRecord{}, fmt.Errorf("cloud store is unavailable")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return SkillVersionRecord{}, err
	}
	defer tx.Rollback(ctx)
	target, err := sharedSkillVersionForUpdate(ctx, tx, skillID, version)
	if err != nil {
		return SkillVersionRecord{}, err
	}
	if target.State != SkillVersionCanary && target.State != SkillVersionRolledBack {
		return SkillVersionRecord{}, fmt.Errorf("skill %s@%s must be canary or rolled_back before promotion", target.Manifest.ID, target.Manifest.Version)
	}
	if target.State == SkillVersionCanary {
		var score float64
		err := tx.QueryRow(ctx, `
SELECT score FROM codelocal_skill_evaluations
WHERE skill_id=$1 AND version=$2 AND decision='passed'
ORDER BY created_at DESC LIMIT 1`, target.Manifest.ID, target.Manifest.Version).Scan(&score)
		if err == pgx.ErrNoRows {
			return SkillVersionRecord{}, fmt.Errorf("skill %s@%s has no passed evaluation evidence", target.Manifest.ID, target.Manifest.Version)
		}
		if err != nil {
			return SkillVersionRecord{}, err
		}
		if score < MinSkillEvaluationScore {
			return SkillVersionRecord{}, fmt.Errorf("skill %s@%s evaluation score %.3f is below %.2f", target.Manifest.ID, target.Manifest.Version, score, MinSkillEvaluationScore)
		}
	}

	var previousVersion string
	err = tx.QueryRow(ctx, `
SELECT version FROM codelocal_skill_channels
WHERE skill_id=$1 AND channel='stable' AND tenant_user_id IS NULL
FOR UPDATE`, target.Manifest.ID).Scan(&previousVersion)
	if err != nil && err != pgx.ErrNoRows {
		return SkillVersionRecord{}, err
	}
	now := time.Now().UnixMilli()
	if previousVersion != "" && previousVersion != target.Manifest.Version {
		if _, err := tx.Exec(ctx, `
UPDATE codelocal_skill_versions
SET state='rolled_back', updated_at=$3
WHERE skill_id=$1 AND version=$2 AND tenant_user_id IS NULL AND state='promoted'`, target.Manifest.ID, previousVersion, now); err != nil {
			return SkillVersionRecord{}, err
		}
	}
	if _, err := tx.Exec(ctx, `
UPDATE codelocal_skill_versions
SET state='promoted', updated_at=$3, promoted_at=$3
WHERE skill_id=$1 AND version=$2 AND tenant_user_id IS NULL`, target.Manifest.ID, target.Manifest.Version, now); err != nil {
		return SkillVersionRecord{}, err
	}
	channelID := skillRegistryID("channel", "", target.Manifest.ID, "stable")
	if _, err := tx.Exec(ctx, `
INSERT INTO codelocal_skill_channels(channel_id, skill_id, tenant_user_id, channel, version, updated_at)
VALUES($1,$2,NULL,'stable',$3,$4)
ON CONFLICT (channel_id) DO UPDATE SET version=EXCLUDED.version, updated_at=EXCLUDED.updated_at`, channelID, target.Manifest.ID, target.Manifest.Version, now); err != nil {
		return SkillVersionRecord{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM codelocal_skill_channels WHERE skill_id=$1 AND channel='canary' AND tenant_user_id IS NULL`, target.Manifest.ID); err != nil {
		return SkillVersionRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SkillVersionRecord{}, err
	}
	target.State = SkillVersionPromoted
	target.UpdatedAt = now
	target.PromotedAt = now
	return target, nil
}

func sharedSkillVersionForUpdate(ctx context.Context, tx pgx.Tx, skillID, version string) (SkillVersionRecord, error) {
	row := tx.QueryRow(ctx, `
SELECT record_id, tenant_user_id, creator_user_id, publisher, manifest,
       state, package_hash, artifact_hash, artifact_uri,
       created_at, updated_at, promoted_at
FROM codelocal_skill_versions
WHERE skill_id=$1 AND version=$2 AND tenant_user_id IS NULL
FOR UPDATE`, strings.TrimSpace(skillID), strings.TrimSpace(version))
	record, err := scanSkillVersionRecord(row)
	if err == pgx.ErrNoRows {
		return SkillVersionRecord{}, fmt.Errorf("shared skill %s@%s does not exist", strings.TrimSpace(skillID), strings.TrimSpace(version))
	}
	if err != nil {
		return SkillVersionRecord{}, err
	}
	if record.Manifest.Scope != skills.ScopeSystem && record.Manifest.Scope != skills.ScopeCommunity {
		return SkillVersionRecord{}, fmt.Errorf("skill %s@%s is not shared", record.Manifest.ID, record.Manifest.Version)
	}
	return record, nil
}

func skillVersionTransitionAllowed(from, to SkillVersionState) bool {
	switch from {
	case SkillVersionCandidate:
		return to == SkillVersionEvaluating || to == SkillVersionRejected || to == SkillVersionBlocked
	case SkillVersionEvaluating:
		return to == SkillVersionCanary || to == SkillVersionRejected || to == SkillVersionBlocked
	case SkillVersionCanary:
		return to == SkillVersionPromoted || to == SkillVersionRejected || to == SkillVersionBlocked
	case SkillVersionPromoted:
		return to == SkillVersionRolledBack || to == SkillVersionDeprecated || to == SkillVersionBlocked
	case SkillVersionRolledBack:
		return to == SkillVersionPromoted || to == SkillVersionDeprecated || to == SkillVersionBlocked
	case SkillVersionActive:
		return to == SkillVersionDeprecated || to == SkillVersionBlocked
	default:
		return false
	}
}
