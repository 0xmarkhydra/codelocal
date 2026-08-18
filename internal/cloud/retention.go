package cloud

import (
	"context"
	"log/slog"
	"time"
)

func (s *Store) retentionWorker() {
	defer s.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.runRetentionSweep()
		}
	}
}

func (s *Store) runRetentionSweep() {
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	now := time.Now()

	s.retentionExec(ctx, "pairings", `DELETE FROM codelocal_pairings WHERE expires_at < $1 OR (claimed_at IS NOT NULL AND claimed_at < $2)`, now.UnixMilli(), now.Add(-24*time.Hour).UnixMilli())
	s.retentionExec(ctx, "oauth_codes", `DELETE FROM codelocal_oauth_codes WHERE expires_at < $1`, now.UnixMilli())
	s.retentionExec(ctx, "mcp_usage_batches", `DELETE FROM codelocal_mcp_usage_batches WHERE processed_at < $1`, now.Add(-35*24*time.Hour).UnixMilli())

	if days := envInt("CODELOCAL_OUTBOX_RETENTION_DAYS", 14); days > 0 {
		s.retentionExec(ctx, "durable_outbox", `DELETE FROM codelocal_durable_outbox WHERE status IN ('processed','dead') AND updated_at < $1`, now.Add(-time.Duration(days)*24*time.Hour).UnixMilli())
	}
	if days := envInt("CODELOCAL_KNOWLEDGE_V2_SHADOW_RETENTION_DAYS", 30); days > 0 {
		s.retentionExec(ctx, "knowledge_shadow_metrics", `DELETE FROM codelocal_knowledge_shadow_metrics WHERE last_sample_at < $1`, now.Add(-time.Duration(days)*24*time.Hour).UnixMilli())
	}
	if days := envInt("CODELOCAL_AUDIT_RETENTION_DAYS", 90); days > 0 {
		s.retentionExec(ctx, "audit_logs", `DELETE FROM codelocal_audit_logs WHERE created_at < $1`, now.Add(-time.Duration(days)*24*time.Hour).UnixMilli())
	}
	if !s.legacyNoiseCleanupDone.Load() {
		count, err := s.quarantineLegacyOperationalNoiseBatch(ctx, now.UnixMilli(), defaultLegacyNoiseCleanupBatch)
		if err != nil {
			slog.Warn("legacy operational knowledge quarantine failed", "error", err)
		} else if count == 0 {
			s.legacyNoiseCleanupDone.Store(true)
		} else {
			slog.Info("legacy operational knowledge quarantined", "count", count)
		}
	}
}

func (s *Store) retentionExec(ctx context.Context, target, query string, args ...any) {
	if _, err := s.DB.Exec(ctx, query, args...); err != nil {
		slog.Warn("retention cleanup failed", "target", target, "error", err)
	}
}
