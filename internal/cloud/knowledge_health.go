package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"time"

	longmemory "github.com/0xmarkhydra/codelocal/internal/memory"
)

const knowledgeHealthMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_knowledge_health (
 user_id TEXT NOT NULL,
 project_id TEXT NOT NULL,
 status TEXT NOT NULL CHECK (status IN ('healthy','degraded','critical')),
 auto_promotion_enabled BOOLEAN NOT NULL,
 canonical_integrity_violations INTEGER NOT NULL DEFAULT 0 CHECK (canonical_integrity_violations >= 0),
 operational_noise_violations INTEGER NOT NULL DEFAULT 0 CHECK (operational_noise_violations >= 0),
 secret_violations INTEGER NOT NULL DEFAULT 0 CHECK (secret_violations >= 0),
 privacy_violations INTEGER NOT NULL DEFAULT 0 CHECK (privacy_violations >= 0),
 conflicted_candidates INTEGER NOT NULL DEFAULT 0 CHECK (conflicted_candidates >= 0),
 stale_pending_candidates INTEGER NOT NULL DEFAULT 0 CHECK (stale_pending_candidates >= 0),
 scan_truncated BOOLEAN NOT NULL DEFAULT FALSE,
 reason_codes TEXT[] NOT NULL DEFAULT '{}'::text[],
 evaluated_at BIGINT NOT NULL,
 PRIMARY KEY(user_id,project_id),
 FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_knowledge_health_status
 ON codelocal_knowledge_health(status,evaluated_at DESC);
`

const knowledgeHealthCanonicalScanSQL = `
SELECT
 o.knowledge_id,COALESCE(o.active_revision_id,''),o.status,o.privacy_classification,COALESCE(o.valid_until,0),
 COALESCE(r.revision_id,''),COALESCE(r.valid_until,0),COALESCE(r.summary,''),COALESCE(r.predicate,''),
 COALESCE(r.subject,'{}'::jsonb),COALESCE(r.object,'{}'::jsonb),
 (SELECT COUNT(*) FROM codelocal_knowledge_provenance p
   WHERE p.user_id=o.user_id AND p.knowledge_id=o.knowledge_id AND p.revision_id=o.active_revision_id),
 (SELECT COUNT(*) FROM codelocal_knowledge_revisions ar
   WHERE ar.user_id=o.user_id AND ar.knowledge_id=o.knowledge_id AND ar.valid_until IS NULL)
FROM codelocal_knowledge_objects o
LEFT JOIN codelocal_knowledge_revisions r
 ON r.user_id=o.user_id AND r.knowledge_id=o.knowledge_id AND r.revision_id=o.active_revision_id
WHERE o.user_id=$1 AND o.project_id=$2 AND o.status='active'
ORDER BY o.updated_at DESC,o.knowledge_id ASC
LIMIT $3`

const knowledgeHealthPromotionStatsSQL = `
SELECT
 COUNT(*) FILTER (WHERE status='conflicted'),
 COUNT(*) FILTER (WHERE status='pending' AND updated_at < $3)
FROM codelocal_knowledge_promotion_candidates
WHERE user_id=$1 AND project_id=$2`

const recentKnowledgeHealthProjectsSQL = `
SELECT user_id,project_id
FROM codelocal_projects
ORDER BY last_seen_at DESC,user_id ASC,project_id ASC
LIMIT $1`

const pendingPromotionCandidatesSQL = `
SELECT candidate_id
FROM codelocal_knowledge_promotion_candidates
WHERE user_id=$1 AND project_id=$2 AND status='pending'
ORDER BY updated_at ASC,candidate_id ASC
LIMIT $3`

const upsertKnowledgeHealthSQL = `
INSERT INTO codelocal_knowledge_health(
 user_id,project_id,status,auto_promotion_enabled,canonical_integrity_violations,operational_noise_violations,
 secret_violations,privacy_violations,conflicted_candidates,stale_pending_candidates,scan_truncated,reason_codes,evaluated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
ON CONFLICT(user_id,project_id) DO UPDATE SET
 status=EXCLUDED.status,
 auto_promotion_enabled=EXCLUDED.auto_promotion_enabled,
 canonical_integrity_violations=EXCLUDED.canonical_integrity_violations,
 operational_noise_violations=EXCLUDED.operational_noise_violations,
 secret_violations=EXCLUDED.secret_violations,
 privacy_violations=EXCLUDED.privacy_violations,
 conflicted_candidates=EXCLUDED.conflicted_candidates,
 stale_pending_candidates=EXCLUDED.stale_pending_candidates,
 scan_truncated=EXCLUDED.scan_truncated,
 reason_codes=EXCLUDED.reason_codes,
 evaluated_at=EXCLUDED.evaluated_at`

const (
	knowledgeHealthHealthy  = "healthy"
	knowledgeHealthDegraded = "degraded"
	knowledgeHealthCritical = "critical"
)

type KnowledgeHealth struct {
	UserID                       string   `json:"userId"`
	ProjectID                    string   `json:"projectId"`
	Status                       string   `json:"status"`
	AutoPromotionEnabled         bool     `json:"autoPromotionEnabled"`
	CanonicalIntegrityViolations int      `json:"canonicalIntegrityViolations"`
	OperationalNoiseViolations   int      `json:"operationalNoiseViolations"`
	SecretViolations             int      `json:"secretViolations"`
	PrivacyViolations            int      `json:"privacyViolations"`
	ConflictedCandidates         int      `json:"conflictedCandidates"`
	StalePendingCandidates       int      `json:"stalePendingCandidates"`
	ScanTruncated                bool     `json:"scanTruncated"`
	ReasonCodes                  []string `json:"reasonCodes"`
	EvaluatedAt                  int64    `json:"evaluatedAt"`
}

type knowledgeHealthCanonicalRow struct {
	KnowledgeID         string
	ActiveRevisionID    string
	Status              string
	Privacy             string
	ObjectValidUntil    int64
	RevisionID          string
	RevisionValidUntil  int64
	Summary             string
	Predicate           string
	Subject             map[string]any
	Object              map[string]any
	ProvenanceCount     int
	ActiveRevisionCount int
}

type knowledgeHealthSignals struct {
	CanonicalIntegrityViolations int
	OperationalNoiseViolations   int
	SecretViolations             int
	PrivacyViolations            int
	ConflictedCandidates         int
	StalePendingCandidates       int
	ScanTruncated                bool
	DurableLearningDegraded      bool
	DurableLearningCritical      bool
}

func knowledgeHealthScanLimit() int {
	value := envInt("CODELOCAL_KNOWLEDGE_HEALTH_SCAN_LIMIT", 1000)
	if value < 100 {
		return 100
	}
	if value > 5000 {
		return 5000
	}
	return value
}

func knowledgeHealthProjectLimit() int {
	value := envInt("CODELOCAL_KNOWLEDGE_HEALTH_PROJECT_LIMIT", 100)
	if value < 1 {
		return 1
	}
	if value > 500 {
		return 500
	}
	return value
}

func knowledgeHealthPendingLimit() int {
	value := envInt("CODELOCAL_KNOWLEDGE_HEALTH_PENDING_LIMIT", 20)
	if value < 1 {
		return 1
	}
	if value > 100 {
		return 100
	}
	return value
}

func knowledgeHealthInterval() time.Duration {
	value := time.Duration(envInt("CODELOCAL_KNOWLEDGE_HEALTH_INTERVAL_MS", 300_000)) * time.Millisecond
	if value < 30*time.Second {
		return 30 * time.Second
	}
	if value > time.Hour {
		return time.Hour
	}
	return value
}

func knowledgeHealthStalePendingCutoff(now time.Time) int64 {
	days := envInt("CODELOCAL_KNOWLEDGE_HEALTH_STALE_PENDING_DAYS", 7)
	if days < 1 {
		days = 1
	}
	if days > 90 {
		days = 90
	}
	return now.Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()
}

func normalizeHealthSecretKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("_", "", "-", "", ".", "", " ", "")
	return replacer.Replace(value)
}

func healthSecretLikeKey(value string) bool {
	normalized := normalizeHealthSecretKey(value)
	for _, key := range []string{"apikey", "accesstoken", "refreshtoken", "token", "secret", "clientsecret", "password", "passwd", "privatekey"} {
		if normalized == key || strings.HasSuffix(normalized, key) {
			return true
		}
	}
	return false
}

func healthStringContainsSecret(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.Contains(trimmed, "[REDACTED") {
		return false
	}
	return longmemory.SanitizeText(trimmed, len([]rune(trimmed))+32) != trimmed
}

func healthValueContainsSecret(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if healthSecretLikeKey(key) {
				if text, ok := child.(string); ok && strings.TrimSpace(text) != "" && !strings.Contains(text, "[REDACTED") {
					return true
				}
			}
			if healthValueContainsSecret(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if healthValueContainsSecret(child) {
				return true
			}
		}
	case string:
		return healthStringContainsSecret(typed)
	}
	return false
}

func healthRowContainsSecret(row knowledgeHealthCanonicalRow) bool {
	return healthStringContainsSecret(row.Summary) || healthStringContainsSecret(row.Predicate) || healthValueContainsSecret(row.Subject) || healthValueContainsSecret(row.Object)
}

func evaluateKnowledgeHealthSignals(rows []knowledgeHealthCanonicalRow, conflictedCandidates, stalePendingCandidates int, scanTruncated bool) knowledgeHealthSignals {
	signals := knowledgeHealthSignals{
		ConflictedCandidates:   conflictedCandidates,
		StalePendingCandidates: stalePendingCandidates,
		ScanTruncated:          scanTruncated,
	}
	for _, row := range rows {
		if row.ActiveRevisionID == "" || row.RevisionID == "" || row.ActiveRevisionID != row.RevisionID || row.ObjectValidUntil > 0 || row.RevisionValidUntil > 0 || row.ActiveRevisionCount != 1 || row.ProvenanceCount < 1 {
			signals.CanonicalIntegrityViolations++
		}
		if row.Privacy != KnowledgeClassPrivateProject {
			signals.PrivacyViolations++
		}
		if promotionOperationalNoise(row.Predicate, row.Summary) {
			signals.OperationalNoiseViolations++
		}
		if healthRowContainsSecret(row) {
			signals.SecretViolations++
		}
	}
	return signals
}

func knowledgeHealthFromSignals(userID, projectID string, signals knowledgeHealthSignals, evaluatedAt int64) KnowledgeHealth {
	reasons := []string{}
	critical := false
	addCritical := func(condition bool, code string) {
		if condition {
			critical = true
			reasons = append(reasons, code)
		}
	}
	addCritical(signals.CanonicalIntegrityViolations > 0, "canonical_integrity_violation")
	addCritical(signals.OperationalNoiseViolations > 0, "operational_noise_leakage")
	addCritical(signals.SecretViolations > 0, "secret_payload_detected")
	addCritical(signals.PrivacyViolations > 0, "privacy_boundary_violation")
	addCritical(signals.ScanTruncated, "health_scan_truncated")
	addCritical(signals.DurableLearningCritical, "durable_learning_pipeline_critical")
	status := knowledgeHealthHealthy
	if critical {
		status = knowledgeHealthCritical
	} else {
		if signals.ConflictedCandidates > 0 {
			reasons = append(reasons, "promotion_conflicts")
		}
		if signals.StalePendingCandidates > 0 {
			reasons = append(reasons, "stale_pending_candidates")
		}
		if signals.DurableLearningDegraded {
			reasons = append(reasons, "durable_learning_pipeline_degraded")
		}
		if len(reasons) > 0 {
			status = knowledgeHealthDegraded
		}
	}
	sort.Strings(reasons)
	return KnowledgeHealth{
		UserID: userID, ProjectID: projectID, Status: status, AutoPromotionEnabled: status != knowledgeHealthCritical,
		CanonicalIntegrityViolations: signals.CanonicalIntegrityViolations,
		OperationalNoiseViolations:   signals.OperationalNoiseViolations,
		SecretViolations:             signals.SecretViolations,
		PrivacyViolations:            signals.PrivacyViolations,
		ConflictedCandidates:         signals.ConflictedCandidates,
		StalePendingCandidates:       signals.StalePendingCandidates,
		ScanTruncated:                signals.ScanTruncated,
		ReasonCodes:                  reasons, EvaluatedAt: evaluatedAt,
	}
}

func scanKnowledgeHealthCanonicalRow(row interface{ Scan(...any) error }) (knowledgeHealthCanonicalRow, error) {
	var value knowledgeHealthCanonicalRow
	var subjectJSON, objectJSON []byte
	if err := row.Scan(
		&value.KnowledgeID, &value.ActiveRevisionID, &value.Status, &value.Privacy, &value.ObjectValidUntil,
		&value.RevisionID, &value.RevisionValidUntil, &value.Summary, &value.Predicate, &subjectJSON, &objectJSON,
		&value.ProvenanceCount, &value.ActiveRevisionCount,
	); err != nil {
		return knowledgeHealthCanonicalRow{}, err
	}
	if err := json.Unmarshal(subjectJSON, &value.Subject); err != nil {
		return knowledgeHealthCanonicalRow{}, err
	}
	if err := json.Unmarshal(objectJSON, &value.Object); err != nil {
		return knowledgeHealthCanonicalRow{}, err
	}
	return value, nil
}

func (s *Store) EvaluateKnowledgeHealth(ctx context.Context, userID, projectID string) (KnowledgeHealth, error) {
	if s == nil || s.DB == nil {
		return KnowledgeHealth{}, errors.New("knowledge health store unavailable")
	}
	userID = strings.TrimSpace(userID)
	projectID = strings.TrimSpace(projectID)
	if userID == "" || projectID == "" {
		return KnowledgeHealth{}, errors.New("knowledge health requires user and project")
	}
	limit := knowledgeHealthScanLimit()
	rows, err := s.DB.Query(ctx, knowledgeHealthCanonicalScanSQL, userID, projectID, limit+1)
	if err != nil {
		return KnowledgeHealth{}, err
	}
	canonicalRows := []knowledgeHealthCanonicalRow{}
	for rows.Next() {
		value, scanErr := scanKnowledgeHealthCanonicalRow(rows)
		if scanErr != nil {
			rows.Close()
			return KnowledgeHealth{}, scanErr
		}
		canonicalRows = append(canonicalRows, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return KnowledgeHealth{}, err
	}
	rows.Close()
	scanTruncated := len(canonicalRows) > limit
	if scanTruncated {
		canonicalRows = canonicalRows[:limit]
	}
	var conflictedCandidates, stalePendingCandidates int
	now := time.Now()
	if err := s.DB.QueryRow(ctx, knowledgeHealthPromotionStatsSQL, userID, projectID, knowledgeHealthStalePendingCutoff(now)).Scan(&conflictedCandidates, &stalePendingCandidates); err != nil {
		return KnowledgeHealth{}, err
	}
	signals := evaluateKnowledgeHealthSignals(canonicalRows, conflictedCandidates, stalePendingCandidates, scanTruncated)
	durableLearning, err := s.DurableOutboxHealth(ctx, userID)
	if err != nil {
		return KnowledgeHealth{}, err
	}
	switch durableLearning.Status {
	case "critical":
		signals.DurableLearningCritical = true
	case "degraded":
		signals.DurableLearningDegraded = true
	}
	health := knowledgeHealthFromSignals(userID, projectID, signals, now.UnixMilli())
	if _, err := s.DB.Exec(ctx, upsertKnowledgeHealthSQL,
		health.UserID, health.ProjectID, health.Status, health.AutoPromotionEnabled,
		health.CanonicalIntegrityViolations, health.OperationalNoiseViolations, health.SecretViolations, health.PrivacyViolations,
		health.ConflictedCandidates, health.StalePendingCandidates, health.ScanTruncated, health.ReasonCodes, health.EvaluatedAt,
	); err != nil {
		return KnowledgeHealth{}, err
	}
	return health, nil
}

func (s *Store) deferPromotionForKnowledgeHealth(ctx context.Context, userID, candidateID string) error {
	_, err := s.DB.Exec(ctx, `
UPDATE codelocal_knowledge_promotion_candidates
SET approval_reason='health_circuit_breaker',evaluated_at=$1,updated_at=$1
WHERE user_id=$2 AND candidate_id=$3 AND status='pending'`, time.Now().UnixMilli(), userID, candidateID)
	return err
}

func (s *Store) evaluateAndPromoteCandidateRef(ctx context.Context, userID, projectID, candidateID string) error {
	health, err := s.EvaluateKnowledgeHealth(ctx, userID, projectID)
	if err != nil {
		return err
	}
	if !health.AutoPromotionEnabled {
		return s.deferPromotionForKnowledgeHealth(ctx, userID, candidateID)
	}
	evaluation, err := s.EvaluatePromotionCandidate(ctx, userID, candidateID)
	if err != nil {
		return err
	}
	if evaluation.Status != "approved" {
		return nil
	}
	_, _, err = s.PromoteApprovedCandidate(ctx, userID, candidateID)
	return err
}

func (s *Store) reevaluateProjectPendingCandidates(ctx context.Context, userID, projectID string) error {
	limit := knowledgeHealthPendingLimit()
	rows, err := s.DB.Query(ctx, pendingPromotionCandidatesSQL, userID, projectID, limit)
	if err != nil {
		return err
	}
	candidateIDs := []string{}
	for rows.Next() {
		var candidateID string
		if err := rows.Scan(&candidateID); err != nil {
			rows.Close()
			return err
		}
		candidateIDs = append(candidateIDs, candidateID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, candidateID := range candidateIDs {
		evaluation, err := s.EvaluatePromotionCandidate(ctx, userID, candidateID)
		if err != nil {
			return err
		}
		if evaluation.Status == "approved" {
			if _, _, err := s.PromoteApprovedCandidate(ctx, userID, candidateID); err != nil {
				return err
			}
		}
	}
	return nil
}

const (
	knowledgeMaintenanceReconcile          = "reconcile_explicit_memory"
	knowledgeMaintenanceRecheck            = "recheck_health"
	knowledgeMaintenancePending            = "reevaluate_pending"
	knowledgeMaintenancePostPendingRecheck = "recheck_health_after_pending"
	knowledgeMaintenanceProjection         = "rebuild_canonical_graph"
)

func knowledgeMaintenanceStages(health KnowledgeHealth) []string {
	if !health.AutoPromotionEnabled {
		return nil
	}
	return []string{
		knowledgeMaintenanceReconcile,
		knowledgeMaintenanceRecheck,
		knowledgeMaintenancePending,
		knowledgeMaintenancePostPendingRecheck,
		knowledgeMaintenanceProjection,
	}
}

func (s *Store) evaluateRecentlyActiveKnowledgeHealth(ctx context.Context) error {
	rows, err := s.DB.Query(ctx, recentKnowledgeHealthProjectsSQL, knowledgeHealthProjectLimit())
	if err != nil {
		return err
	}
	type projectRef struct{ userID, projectID string }
	projects := []projectRef{}
	for rows.Next() {
		var ref projectRef
		if err := rows.Scan(&ref.userID, &ref.projectID); err != nil {
			rows.Close()
			return err
		}
		projects = append(projects, ref)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, project := range projects {
		projectCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		health, evalErr := s.EvaluateKnowledgeHealth(projectCtx, project.userID, project.projectID)
		for _, stage := range knowledgeMaintenanceStages(health) {
			if evalErr != nil || !health.AutoPromotionEnabled {
				break
			}
			switch stage {
			case knowledgeMaintenanceReconcile:
				_, evalErr = s.reconcileExplicitProjectMemories(projectCtx, project.userID, project.projectID)
			case knowledgeMaintenanceRecheck:
				// Newly canonicalized explicit memory must pass the hard breaker
				// before any older pending candidate proceeds in this sweep.
				health, evalErr = s.EvaluateKnowledgeHealth(projectCtx, project.userID, project.projectID)
			case knowledgeMaintenancePending:
				evalErr = s.reevaluateProjectPendingCandidates(projectCtx, project.userID, project.projectID)
			case knowledgeMaintenancePostPendingRecheck:
				// Pending promotion may itself change canonical state. Re-evaluate
				// before publishing any derived projection from the same sweep.
				health, evalErr = s.EvaluateKnowledgeHealth(projectCtx, project.userID, project.projectID)
			case knowledgeMaintenanceProjection:
				if _, projectionErr := s.RebuildCanonicalKnowledgeGraphProject(projectCtx, project.userID, project.projectID); projectionErr != nil {
					slog.Debug("canonical graph projection skipped", "error", projectionErr)
				}
				if s.canonicalEmbeddingProvider != nil {
					embeddingCtx, embeddingCancel := context.WithTimeout(ctx, canonicalEmbeddingMaintenanceTimeout())
					_, embeddingErr := s.RebuildCanonicalKnowledgeEmbeddingsProject(embeddingCtx, s.canonicalEmbeddingProvider, project.userID, project.projectID)
					embeddingCancel()
					if embeddingErr != nil {
						slog.Debug("canonical embedding projection skipped", "error", embeddingErr)
					}
				}
			}
		}
		cancel()
		if evalErr != nil {
			slog.Debug("knowledge health project evaluation skipped", "error", evalErr)
		}
	}
	return nil
}

func (s *Store) knowledgeHealthWorker() {
	defer s.wg.Done()
	ticker := time.NewTicker(knowledgeHealthInterval())
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
			if err := s.evaluateRecentlyActiveKnowledgeHealth(ctx); err != nil {
				slog.Debug("knowledge health sweep skipped", "error", err)
			}
			cancel()
		}
	}
}
