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

const (
	durableOutboxExperienceRecordV1        = "experience.record.v1"
	durableOutboxExplicitMemoryPromotionV1 = "promotion.explicit-memory.v1"
)

const durableOutboxMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_durable_outbox (
 user_id TEXT NOT NULL,
 outbox_id TEXT NOT NULL,
 event_type TEXT NOT NULL,
 dedupe_key TEXT NOT NULL,
 payload JSONB NOT NULL,
 status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','processed','dead')),
 attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
 available_at BIGINT NOT NULL,
 lease_until BIGINT,
 worker_id TEXT,
 last_error TEXT,
 created_at BIGINT NOT NULL,
 updated_at BIGINT NOT NULL,
 processed_at BIGINT,
 PRIMARY KEY(user_id,outbox_id),
 UNIQUE(user_id,event_type,dedupe_key),
 FOREIGN KEY(user_id) REFERENCES codelocal_users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_durable_outbox_claim
 ON codelocal_durable_outbox(status,available_at,created_at)
 WHERE status IN ('pending','processing');
CREATE INDEX IF NOT EXISTS idx_codelocal_durable_outbox_processed
 ON codelocal_durable_outbox(processed_at)
 WHERE status='processed';`

const claimDurableOutboxSQL = `
WITH candidates AS (
 SELECT user_id,outbox_id
 FROM codelocal_durable_outbox
 WHERE status IN ('pending','processing')
  AND available_at <= $1
  AND (status='pending' OR lease_until IS NULL OR lease_until <= $1)
 ORDER BY available_at ASC,created_at ASC,outbox_id ASC
 FOR UPDATE SKIP LOCKED
 LIMIT $2
)
UPDATE codelocal_durable_outbox o
SET status='processing',attempts=o.attempts+1,lease_until=$3,worker_id=$4,updated_at=$1
FROM candidates c
WHERE o.user_id=c.user_id AND o.outbox_id=c.outbox_id
RETURNING o.user_id,o.outbox_id,o.event_type,o.payload,o.attempts`

type durableOutboxItem struct {
	UserID    string
	OutboxID  string
	EventType string
	Payload   []byte
	Attempts  int
}

type DurableOutboxHealth struct {
	Status            string `json:"status"`
	PendingCount      int    `json:"pendingCount"`
	ProcessingCount   int    `json:"processingCount"`
	RetryingCount     int    `json:"retryingCount"`
	DeadCount         int    `json:"deadCount"`
	ProcessedLastHour int    `json:"processedLastHour"`
	OldestActiveAgeMS int64  `json:"oldestActiveAgeMs"`
	MaxAttempts       int    `json:"maxAttempts"`
}

type durableOutboxHealthSample struct {
	PendingCount      int
	ProcessingCount   int
	RetryingCount     int
	DeadCount         int
	ProcessedLastHour int
	OldestCreatedAt   int64
	MaxAttempts       int
}

type experienceRepositoryBinding struct {
	ID           string
	RelativePath string
}

const durableOutboxHealthSelect = `
SELECT
 COUNT(*) FILTER (WHERE status='pending')::int,
 COUNT(*) FILTER (WHERE status='processing')::int,
 COUNT(*) FILTER (WHERE status='pending' AND attempts>0)::int,
 COUNT(*) FILTER (WHERE status='dead')::int,
 COUNT(*) FILTER (WHERE status='processed' AND processed_at >= $1)::int,
 COALESCE(MIN(created_at) FILTER (WHERE status IN ('pending','processing')),0)::bigint,
 COALESCE(MAX(attempts) FILTER (WHERE status IN ('pending','processing','dead')),0)::int
FROM codelocal_durable_outbox`

const durableOutboxHealthGlobalSQL = durableOutboxHealthSelect
const durableOutboxHealthUserSQL = durableOutboxHealthSelect + ` WHERE user_id=$2`

func durableOutboxHealthFromSample(sample durableOutboxHealthSample, now int64) DurableOutboxHealth {
	age := int64(0)
	if sample.OldestCreatedAt > 0 && now > sample.OldestCreatedAt {
		age = now - sample.OldestCreatedAt
	}
	degradedAge := int64(maxInt(envInt("CODELOCAL_OUTBOX_DEGRADED_AGE_MS", 120_000), 10_000))
	criticalAge := int64(maxInt(envInt("CODELOCAL_OUTBOX_CRITICAL_AGE_MS", 1_800_000), int(degradedAge)))
	criticalDead := maxInt(envInt("CODELOCAL_OUTBOX_CRITICAL_DEAD_COUNT", 10), 1)
	maxAttempts := maxInt(envInt("CODELOCAL_OUTBOX_MAX_ATTEMPTS", 12), 1)
	retryWarningAttempts := maxInt(maxAttempts/2, 2)
	status := "healthy"
	switch {
	case sample.DeadCount >= criticalDead || age >= criticalAge:
		status = "critical"
	case sample.DeadCount > 0 || age >= degradedAge || (sample.RetryingCount > 0 && sample.MaxAttempts >= retryWarningAttempts):
		status = "degraded"
	}
	return DurableOutboxHealth{
		Status: status, PendingCount: sample.PendingCount, ProcessingCount: sample.ProcessingCount,
		RetryingCount: sample.RetryingCount, DeadCount: sample.DeadCount, ProcessedLastHour: sample.ProcessedLastHour,
		OldestActiveAgeMS: age, MaxAttempts: sample.MaxAttempts,
	}
}

func (s *Store) DurableOutboxHealth(ctx context.Context, userID string) (DurableOutboxHealth, error) {
	if s == nil || s.DB == nil {
		return DurableOutboxHealth{Status: "unavailable"}, errors.New("durable outbox unavailable")
	}
	now := time.Now().UnixMilli()
	since := now - time.Hour.Milliseconds()
	var sample durableOutboxHealthSample
	var err error
	if userID = strings.TrimSpace(userID); userID == "" {
		err = s.DB.QueryRow(ctx, durableOutboxHealthGlobalSQL, since).Scan(
			&sample.PendingCount, &sample.ProcessingCount, &sample.RetryingCount, &sample.DeadCount,
			&sample.ProcessedLastHour, &sample.OldestCreatedAt, &sample.MaxAttempts,
		)
	} else {
		err = s.DB.QueryRow(ctx, durableOutboxHealthUserSQL, since, userID).Scan(
			&sample.PendingCount, &sample.ProcessingCount, &sample.RetryingCount, &sample.DeadCount,
			&sample.ProcessedLastHour, &sample.OldestCreatedAt, &sample.MaxAttempts,
		)
	}
	if err != nil {
		return DurableOutboxHealth{Status: "unavailable"}, err
	}
	return durableOutboxHealthFromSample(sample, now), nil
}

func durableOutboxBackoff(attempt int, base, maximum time.Duration) time.Duration {
	if base <= 0 {
		base = 500 * time.Millisecond
	}
	if maximum < base {
		maximum = base
	}
	if attempt < 1 {
		attempt = 1
	}
	delay := base
	for i := 1; i < attempt && delay < maximum; i++ {
		if delay > maximum/2 {
			return maximum
		}
		delay *= 2
	}
	if delay > maximum {
		return maximum
	}
	return delay
}

func chooseExperienceRepository(files []string, bindings []experienceRepositoryBinding) string {
	seen := map[string]struct{}{}
	for _, rawFile := range files {
		if strings.TrimSpace(rawFile) == "" {
			continue
		}
		file := safeRepositoryRelativePath(rawFile)
		if file == "" || file == "." {
			continue
		}
		bestID := ""
		bestLen := -1
		for _, binding := range bindings {
			id := strings.TrimSpace(binding.ID)
			rel := safeRepositoryRelativePath(binding.RelativePath)
			if id == "" || rel == "" {
				continue
			}
			matches := rel == "." || file == rel || strings.HasPrefix(file, rel+"/")
			if !matches {
				continue
			}
			length := len(rel)
			if rel == "." {
				length = 0
			}
			if length > bestLen {
				bestID = id
				bestLen = length
			}
		}
		if bestID != "" {
			seen[bestID] = struct{}{}
		}
	}
	if len(seen) != 1 {
		return ""
	}
	for id := range seen {
		return id
	}
	return ""
}

func (s *Store) resolveExperienceScope(ctx context.Context, input ExperienceInput) (ExperienceInput, error) {
	if s == nil || s.DB == nil {
		return input, errors.New("experience scope resolver unavailable")
	}
	if input.ProjectID == "" && input.UserID != "" && input.DeviceID != "" && input.WorkspaceID != "" {
		binding, err := s.WorkspaceProject(ctx, input.UserID, input.DeviceID, input.WorkspaceID)
		if err != nil {
			return input, err
		}
		if binding != nil {
			input.ProjectID = binding.ProjectID
		}
	}
	if input.RepositoryID != "" || input.ProjectID == "" || input.UserID == "" || input.DeviceID == "" || input.WorkspaceID == "" || len(input.Files) == 0 {
		return input, nil
	}
	rows, err := s.DB.Query(ctx, `
SELECT wr.repository_id,wr.relative_path
FROM codelocal_workspace_repositories wr
JOIN codelocal_workspace_projects wp
 ON wp.user_id=wr.user_id AND wp.device_id=wr.device_id AND wp.workspace_id=wr.workspace_id
WHERE wr.user_id=$1 AND wr.device_id=$2 AND wr.workspace_id=$3 AND wp.project_id=$4`, input.UserID, input.DeviceID, input.WorkspaceID, input.ProjectID)
	if err != nil {
		return input, err
	}
	defer rows.Close()
	bindings := []experienceRepositoryBinding{}
	for rows.Next() {
		var binding experienceRepositoryBinding
		if err := rows.Scan(&binding.ID, &binding.RelativePath); err != nil {
			return input, err
		}
		bindings = append(bindings, binding)
	}
	if err := rows.Err(); err != nil {
		return input, err
	}
	input.RepositoryID = chooseExperienceRepository(input.Files, bindings)
	return input, nil
}

func (s *Store) EnqueueExperience(ctx context.Context, input ExperienceInput) (string, error) {
	if s == nil || s.DB == nil {
		return "", errors.New("durable outbox unavailable")
	}
	normalized, err := normalizeExperienceInput(input)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	dedupeKey := experienceID(normalized)
	outboxID := knowledgeStableID("out_", normalized.UserID, durableOutboxExperienceRecordV1, dedupeKey)
	now := time.Now().UnixMilli()
	_, err = s.DB.Exec(ctx, `
INSERT INTO codelocal_durable_outbox(
 user_id,outbox_id,event_type,dedupe_key,payload,status,attempts,available_at,created_at,updated_at)
VALUES($1,$2,$3,$4,$5::jsonb,'pending',0,$6,$6,$6)
ON CONFLICT(user_id,event_type,dedupe_key) DO NOTHING`, normalized.UserID, outboxID, durableOutboxExperienceRecordV1, dedupeKey, payload, now)
	if err != nil {
		return "", err
	}
	select {
	case s.outboxWake <- struct{}{}:
	default:
	}
	return outboxID, nil
}

func (s *Store) EnqueueExplicitMemoryPromotion(ctx context.Context, input ExplicitMemoryPromotionInput) (string, error) {
	if s == nil || s.DB == nil {
		return "", errors.New("durable outbox unavailable")
	}
	input.UserID = strings.TrimSpace(input.UserID)
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.MemoryID = strings.TrimSpace(input.MemoryID)
	input.SourceKey = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(input.SourceKey)), " "))
	input.StableKey = normalizePromotionStableKey(input.StableKey)
	input.RevisionToken = strings.TrimSpace(input.RevisionToken)
	if input.UserID == "" || input.ProjectID == "" || input.MemoryID == "" || input.SourceKey == "" || input.StableKey == "" || input.RevisionToken == "" {
		return "", errors.New("explicit memory promotion requires durable source identity")
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	dedupeKey := knowledgeStableID("emp_", input.UserID, input.ProjectID, input.MemoryID, input.RevisionToken)
	outboxID := knowledgeStableID("out_", input.UserID, durableOutboxExplicitMemoryPromotionV1, dedupeKey)
	now := time.Now().UnixMilli()
	_, err = s.DB.Exec(ctx, `
INSERT INTO codelocal_durable_outbox(
 user_id,outbox_id,event_type,dedupe_key,payload,status,attempts,available_at,created_at,updated_at)
VALUES($1,$2,$3,$4,$5::jsonb,'pending',0,$6,$6,$6)
ON CONFLICT(user_id,event_type,dedupe_key) DO NOTHING`, input.UserID, outboxID, durableOutboxExplicitMemoryPromotionV1, dedupeKey, payload, now)
	if err != nil {
		return "", err
	}
	select {
	case s.outboxWake <- struct{}{}:
	default:
	}
	return outboxID, nil
}

func (s *Store) claimDurableOutbox(ctx context.Context, limit int) ([]durableOutboxItem, error) {
	if limit <= 0 {
		limit = 16
	}
	if limit > 100 {
		limit = 100
	}
	now := time.Now().UnixMilli()
	// The worker gives each item up to 10s to process. Keep the lease safely
	// above that bound even if an operator configures an accidentally tiny value.
	leaseMs := maxInt(envInt("CODELOCAL_OUTBOX_LEASE_MS", 30_000), 15_000)
	lease := time.Duration(leaseMs) * time.Millisecond
	rows, err := s.DB.Query(ctx, claimDurableOutboxSQL, now, limit, now+lease.Milliseconds(), s.outboxWorkerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []durableOutboxItem{}
	for rows.Next() {
		var item durableOutboxItem
		if err := rows.Scan(&item.UserID, &item.OutboxID, &item.EventType, &item.Payload, &item.Attempts); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) processDurableOutboxItem(ctx context.Context, item durableOutboxItem) error {
	switch item.EventType {
	case durableOutboxExperienceRecordV1:
		var input ExperienceInput
		if err := json.Unmarshal(item.Payload, &input); err != nil {
			return err
		}
		resolved, err := s.resolveExperienceScope(ctx, input)
		if err != nil {
			return err
		}
		experience, err := s.RecordExperience(ctx, resolved)
		if err != nil {
			return err
		}
		candidate, err := s.recordPromotionCandidateFromExperience(ctx, experience)
		if err != nil {
			return err
		}
		return s.evaluateAndPromoteCandidate(ctx, candidate)
	case durableOutboxExplicitMemoryPromotionV1:
		var input ExplicitMemoryPromotionInput
		if err := json.Unmarshal(item.Payload, &input); err != nil {
			return err
		}
		candidate, err := s.StageExplicitMemoryCandidate(ctx, input)
		if err != nil || candidate == nil {
			return err
		}
		return s.evaluateAndPromoteCandidate(ctx, candidate)
	default:
		return errors.New("unsupported durable outbox event type: " + item.EventType)
	}
}

func (s *Store) markDurableOutboxProcessed(ctx context.Context, item durableOutboxItem) error {
	now := time.Now().UnixMilli()
	_, err := s.DB.Exec(ctx, `
UPDATE codelocal_durable_outbox
SET status='processed',lease_until=NULL,worker_id=NULL,last_error=NULL,processed_at=$1,updated_at=$1
WHERE user_id=$2 AND outbox_id=$3 AND status='processing' AND worker_id=$4`, now, item.UserID, item.OutboxID, s.outboxWorkerID)
	return err
}

func (s *Store) retryDurableOutbox(ctx context.Context, item durableOutboxItem, processErr error) error {
	// Allow enough time for eventual project/repository bindings or short cloud
	// outages to recover before an item requires operator attention.
	maxAttempts := envInt("CODELOCAL_OUTBOX_MAX_ATTEMPTS", 12)
	now := time.Now().UnixMilli()
	status := "pending"
	availableAt := now
	if item.Attempts >= maxAttempts {
		status = "dead"
	} else {
		base := time.Duration(envInt("CODELOCAL_OUTBOX_RETRY_BASE_MS", 500)) * time.Millisecond
		maximum := time.Duration(envInt("CODELOCAL_OUTBOX_RETRY_MAX_MS", 300_000)) * time.Millisecond
		availableAt = now + durableOutboxBackoff(item.Attempts, base, maximum).Milliseconds()
	}
	message := longmemory.SanitizeText(processErr.Error(), 800)
	_, err := s.DB.Exec(ctx, `
UPDATE codelocal_durable_outbox
SET status=$1,available_at=$2,lease_until=NULL,worker_id=NULL,last_error=$3,updated_at=$4
WHERE user_id=$5 AND outbox_id=$6 AND status='processing' AND worker_id=$7`, status, availableAt, message, now, item.UserID, item.OutboxID, s.outboxWorkerID)
	return err
}

func (s *Store) drainDurableOutbox() {
	batchSize := envInt("CODELOCAL_OUTBOX_BATCH_SIZE", 16)
	for batch := 0; batch < 4; batch++ {
		if s.ctx.Err() != nil {
			return
		}
		claimCtx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		items, err := s.claimDurableOutbox(claimCtx, batchSize)
		cancel()
		if err != nil {
			slog.Warn("durable outbox claim failed", "error", err)
			return
		}
		if len(items) == 0 {
			return
		}
		for _, item := range items {
			processCtx, processCancel := context.WithTimeout(s.ctx, 10*time.Second)
			err := s.processDurableOutboxItem(processCtx, item)
			processCancel()
			if err == nil {
				markCtx, markCancel := context.WithTimeout(s.ctx, 3*time.Second)
				markErr := s.markDurableOutboxProcessed(markCtx, item)
				markCancel()
				if markErr != nil {
					slog.Warn("durable outbox completion mark failed; lease will replay item", "eventType", item.EventType, "error", markErr)
				}
				continue
			}
			retryCtx, retryCancel := context.WithTimeout(s.ctx, 3*time.Second)
			retryErr := s.retryDurableOutbox(retryCtx, item, err)
			retryCancel()
			if retryErr != nil {
				slog.Warn("durable outbox retry scheduling failed; lease will replay item", "eventType", item.EventType, "error", retryErr)
			}
		}
		if len(items) < batchSize {
			return
		}
	}
}

func (s *Store) durableOutboxWorker() {
	defer s.wg.Done()
	interval := time.Duration(envInt("CODELOCAL_OUTBOX_POLL_MS", 500)) * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	s.drainDurableOutbox()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.outboxWake:
			s.drainDurableOutbox()
		case <-ticker.C:
			s.drainDurableOutbox()
		}
	}
}
