package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"time"

	skillintel "github.com/0xmarkhydra/codelocal/internal/skills"
	"github.com/jackc/pgx/v5"
)

const collectiveIntelligenceMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_collective_preferences (
 user_id TEXT PRIMARY KEY REFERENCES codelocal_users(id) ON DELETE CASCADE,
 contribution_enabled BOOLEAN NOT NULL DEFAULT FALSE,
 suggestions_enabled BOOLEAN NOT NULL DEFAULT FALSE,
 updated_at BIGINT NOT NULL
);
CREATE TABLE IF NOT EXISTS codelocal_collective_user_patterns (
 user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
 pattern_key TEXT NOT NULL,
 fingerprint JSONB NOT NULL,
 success_count BIGINT NOT NULL DEFAULT 0 CHECK (success_count >= 0),
 failure_count BIGINT NOT NULL DEFAULT 0 CHECK (failure_count >= 0),
 sample_count BIGINT NOT NULL DEFAULT 0 CHECK (sample_count >= 0),
 first_seen_at BIGINT NOT NULL,
 last_seen_at BIGINT NOT NULL,
 PRIMARY KEY(user_id,pattern_key),
 CHECK (BTRIM(pattern_key) <> ''),
 CHECK (jsonb_typeof(fingerprint) = 'object'),
 CHECK (sample_count = success_count + failure_count)
);
CREATE INDEX IF NOT EXISTS idx_codelocal_collective_patterns_cohort
 ON codelocal_collective_user_patterns(pattern_key,last_seen_at DESC);
CREATE TABLE IF NOT EXISTS codelocal_collective_event_ledger (
 user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
 event_token TEXT NOT NULL,
 pattern_key TEXT NOT NULL,
 created_at BIGINT NOT NULL,
 PRIMARY KEY(user_id,event_token),
 CHECK (BTRIM(event_token) <> ''),
 CHECK (BTRIM(pattern_key) <> '')
);
CREATE INDEX IF NOT EXISTS idx_codelocal_collective_event_pattern
 ON codelocal_collective_event_ledger(pattern_key,created_at DESC);
`

const collectiveRecommendationsSQL = `
SELECT pattern_key,fingerprint,COUNT(*)::int,COALESCE(SUM(sample_count),0)::bigint,
 COALESCE(AVG(CASE WHEN sample_count>0 THEN success_count::double precision/sample_count ELSE 0 END),0)::double precision,
 MAX(last_seen_at)::bigint
FROM codelocal_collective_user_patterns
WHERE last_seen_at >= $1 AND user_id <> $2
GROUP BY pattern_key,fingerprint
HAVING COUNT(*) >= $3
ORDER BY AVG(CASE WHEN sample_count>0 THEN success_count::double precision/sample_count ELSE 0 END) DESC,
 COUNT(*) DESC,MAX(last_seen_at) DESC,pattern_key ASC
LIMIT $4`

type CollectivePreference struct {
	ContributionEnabled bool  `json:"contributionEnabled"`
	SuggestionsEnabled  bool  `json:"suggestionsEnabled"`
	UpdatedAt           int64 `json:"updatedAt"`
}

type CollectiveFingerprint struct {
	SchemaVersion   int      `json:"schemaVersion"`
	TaskKind        string   `json:"taskKind"`
	CheckProfile    []string `json:"checkProfile"`
	FileCountBucket string   `json:"fileCountBucket"`
	SymbolBucket    string   `json:"symbolCountBucket"`
	ExecutionTool   string   `json:"executionTool"`
	QualityBucket   string   `json:"qualityBucket"`
	SkillUsed       bool     `json:"skillUsed"`
	SkillID         string   `json:"skillId,omitempty"`
	DiffObserved    bool     `json:"diffObserved"`
}

type CollectiveRecommendation struct {
	PatternKey          string                `json:"patternKey"`
	Fingerprint         CollectiveFingerprint `json:"fingerprint"`
	ContributorCount    int                   `json:"contributorCount"`
	SampleCount         int64                 `json:"sampleCount"`
	MeanUserSuccessRate float64               `json:"meanUserSuccessRate"`
	LastSeenAt          int64                 `json:"lastSeenAt"`
}

func collectiveEnvEnabled(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}

func CollectiveContributionAvailable() bool {
	return collectiveEnvEnabled("CODELOCAL_COLLECTIVE_CONTRIBUTION")
}

func CollectiveSuggestionsAvailable() bool {
	return collectiveEnvEnabled("CODELOCAL_COLLECTIVE_INTELLIGENCE")
}

func collectiveTaskKind(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "review", "refactor", "migration", "deployment", "bugfix", "feature", "test", "code", "general":
		return value
	default:
		return "other"
	}
}

func collectiveCheckProfile(values []string) []string {
	allowed := map[string]bool{"diff-check": true, "test": true, "lint": true, "typecheck": true, "build": true}
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
		category := strings.ToLower(strings.TrimSpace(value))
		if index := strings.Index(category, ":"); index >= 0 {
			category = category[:index]
		}
		if !allowed[category] {
			continue
		}
		if _, duplicate := seen[category]; duplicate {
			continue
		}
		seen[category] = struct{}{}
		out = append(out, category)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return []string{"verified"}
	}
	return out
}

func collectiveCountBucket(count int) string {
	switch {
	case count <= 0:
		return "0"
	case count == 1:
		return "1"
	case count <= 3:
		return "2-3"
	case count <= 8:
		return "4-8"
	default:
		return "9+"
	}
}

func collectiveExecutionTool(metadata map[string]any) string {
	value := strings.ToLower(strings.TrimSpace(anyString(metadata["executionTool"])))
	switch value {
	case "edit", "verify", "terminal", "git", "browser", "computer", "agent":
		return value
	default:
		return "other"
	}
}

func anyString(value any) string {
	text, _ := value.(string)
	return text
}

func collectiveQualityBucket(metadata map[string]any) string {
	var score int
	switch value := metadata["qualityScore"].(type) {
	case int:
		score = value
	case int64:
		score = int(value)
	case float64:
		score = int(value)
	default:
		return "unknown"
	}
	switch {
	case score < 0 || score > 100:
		return "unknown"
	case score < 60:
		return "0-59"
	case score < 80:
		return "60-79"
	case score < 95:
		return "80-94"
	default:
		return "95-100"
	}
}

// collectiveSkillID only exposes canonical reusable Skill IDs. Arbitrary
// local/project learned-skill IDs can still contribute the coarse SkillUsed
// signal but never cross the collective privacy boundary by name.
func collectiveSkillID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	manifest, ok := skillintel.DefaultRegistry().Get(value)
	if !ok {
		return ""
	}
	if manifest.Scope != skillintel.ScopeSystem && manifest.Scope != skillintel.ScopeCommunity {
		return ""
	}
	return manifest.ID
}

func collectiveFingerprintForExperience(experience Experience) (CollectiveFingerprint, bool) {
	if experience.Outcome != "succeeded" && experience.Outcome != "failed" {
		return CollectiveFingerprint{}, false
	}
	fingerprint := CollectiveFingerprint{
		SchemaVersion:   2,
		TaskKind:        collectiveTaskKind(experience.TaskKind),
		CheckProfile:    collectiveCheckProfile(experience.Checks),
		FileCountBucket: collectiveCountBucket(len(experience.Files)),
		SymbolBucket:    collectiveCountBucket(len(experience.Symbols)),
		ExecutionTool:   collectiveExecutionTool(experience.Metadata),
		QualityBucket:   collectiveQualityBucket(experience.Metadata),
		SkillUsed:       strings.TrimSpace(experience.SkillID) != "",
		SkillID:         collectiveSkillID(experience.SkillID),
	}
	if value, ok := experience.Metadata["diffObserved"].(bool); ok {
		fingerprint.DiffObserved = value
	}
	return fingerprint, true
}

func collectivePatternKey(fingerprint CollectiveFingerprint) string {
	raw, _ := json.Marshal(fingerprint)
	sum := sha256.Sum256(raw)
	return "pattern_" + hex.EncodeToString(sum[:12])
}

func collectiveEventToken(experience Experience) string {
	sum := sha256.Sum256([]byte(experience.UserID + "\x00" + experience.ExperienceID))
	return "event_" + hex.EncodeToString(sum[:16])
}

func (s *Store) CollectivePreference(ctx context.Context, userID string) (CollectivePreference, error) {
	if s == nil || s.DB == nil || strings.TrimSpace(userID) == "" {
		return CollectivePreference{}, errors.New("collective preference requires user")
	}
	var preference CollectivePreference
	err := s.DB.QueryRow(ctx, `SELECT contribution_enabled,suggestions_enabled,updated_at FROM codelocal_collective_preferences WHERE user_id=$1`, userID).Scan(
		&preference.ContributionEnabled, &preference.SuggestionsEnabled, &preference.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return CollectivePreference{}, nil
	}
	return preference, err
}

func (s *Store) SetCollectivePreference(ctx context.Context, userID string, contributionEnabled, suggestionsEnabled bool) (CollectivePreference, error) {
	if s == nil || s.DB == nil || strings.TrimSpace(userID) == "" {
		return CollectivePreference{}, errors.New("collective preference requires user")
	}
	now := time.Now().UnixMilli()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return CollectivePreference{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
INSERT INTO codelocal_collective_preferences(user_id,contribution_enabled,suggestions_enabled,updated_at)
VALUES($1,$2,$3,$4)
ON CONFLICT(user_id) DO UPDATE SET contribution_enabled=EXCLUDED.contribution_enabled,suggestions_enabled=EXCLUDED.suggestions_enabled,updated_at=EXCLUDED.updated_at`,
		userID, contributionEnabled, suggestionsEnabled, now); err != nil {
		return CollectivePreference{}, err
	}
	if !contributionEnabled {
		if _, err := tx.Exec(ctx, `DELETE FROM codelocal_collective_event_ledger WHERE user_id=$1`, userID); err != nil {
			return CollectivePreference{}, err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM codelocal_collective_user_patterns WHERE user_id=$1`, userID); err != nil {
			return CollectivePreference{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return CollectivePreference{}, err
	}
	return CollectivePreference{ContributionEnabled: contributionEnabled, SuggestionsEnabled: suggestionsEnabled, UpdatedAt: now}, nil
}

func (s *Store) RecordCollectiveExperience(ctx context.Context, experience Experience) (bool, error) {
	if !collectiveEnvEnabled("CODELOCAL_COLLECTIVE_CONTRIBUTION") {
		return false, nil
	}
	preference, err := s.CollectivePreference(ctx, experience.UserID)
	if err != nil || !preference.ContributionEnabled {
		return false, err
	}
	fingerprint, ok := collectiveFingerprintForExperience(experience)
	if !ok {
		return false, nil
	}
	patternKey := collectivePatternKey(fingerprint)
	eventToken := collectiveEventToken(experience)
	fingerprintJSON, err := json.Marshal(fingerprint)
	if err != nil {
		return false, err
	}
	now := time.Now().UnixMilli()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
INSERT INTO codelocal_collective_event_ledger(user_id,event_token,pattern_key,created_at)
VALUES($1,$2,$3,$4)
ON CONFLICT(user_id,event_token) DO NOTHING`, experience.UserID, eventToken, patternKey, now)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	succeeded := int64(0)
	failed := int64(0)
	if experience.Outcome == "succeeded" {
		succeeded = 1
	} else {
		failed = 1
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO codelocal_collective_user_patterns(user_id,pattern_key,fingerprint,success_count,failure_count,sample_count,first_seen_at,last_seen_at)
VALUES($1,$2,$3::jsonb,$4,$5,1,$6,$6)
ON CONFLICT(user_id,pattern_key) DO UPDATE SET
 fingerprint=EXCLUDED.fingerprint,
 success_count=codelocal_collective_user_patterns.success_count+EXCLUDED.success_count,
 failure_count=codelocal_collective_user_patterns.failure_count+EXCLUDED.failure_count,
 sample_count=codelocal_collective_user_patterns.sample_count+1,
 last_seen_at=GREATEST(codelocal_collective_user_patterns.last_seen_at,EXCLUDED.last_seen_at)`,
		experience.UserID, patternKey, string(fingerprintJSON), succeeded, failed, now); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func collectiveMinContributors() int {
	value := envInt("CODELOCAL_COLLECTIVE_MIN_CONTRIBUTORS", 20)
	if value < 5 {
		value = 5
	}
	if value > 1000 {
		value = 1000
	}
	return value
}

func CollectiveMinimumContributors() int {
	return collectiveMinContributors()
}

func collectiveFreshnessCutoff() int64 {
	days := envInt("CODELOCAL_COLLECTIVE_FRESH_DAYS", 90)
	if days < 30 {
		days = 30
	}
	if days > 365 {
		days = 365
	}
	return time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()
}

func (s *Store) CollectiveRecommendations(ctx context.Context, userID string, limit int) ([]CollectiveRecommendation, error) {
	if !collectiveEnvEnabled("CODELOCAL_COLLECTIVE_INTELLIGENCE") {
		return nil, nil
	}
	preference, err := s.CollectivePreference(ctx, userID)
	if err != nil || !preference.SuggestionsEnabled {
		return nil, err
	}
	if limit < 1 {
		limit = 8
	}
	if limit > 32 {
		limit = 32
	}
	rows, err := s.DB.Query(ctx, collectiveRecommendationsSQL, collectiveFreshnessCutoff(), userID, collectiveMinContributors(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CollectiveRecommendation{}
	for rows.Next() {
		var item CollectiveRecommendation
		var fingerprintJSON []byte
		if err := rows.Scan(&item.PatternKey, &fingerprintJSON, &item.ContributorCount, &item.SampleCount, &item.MeanUserSuccessRate, &item.LastSeenAt); err != nil {
			return nil, err
		}
		if json.Unmarshal(fingerprintJSON, &item.Fingerprint) != nil {
			continue
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
