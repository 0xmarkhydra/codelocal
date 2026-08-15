package cloud

import (
	"context"
	"errors"
	"strings"
	"time"

	longmemory "github.com/0xmarkhydra/codelocal/internal/memory"
	"github.com/jackc/pgx/v5"
)

const revokeKnowledgeSourceSQL = `
UPDATE codelocal_knowledge_sources
SET status='revoked',valid_to=$3,last_seen_at=$3
WHERE user_id=$1 AND source_id=$2`

const setMemoryLifecycleSQL = `
UPDATE codelocal_memories
SET lifecycle_status=$3,updated_at=$4
WHERE user_id=$1 AND id=$2`

func validMemoryLifecycle(status longmemory.LifecycleStatus) bool {
	switch status {
	case longmemory.LifecycleObserved, longmemory.LifecycleConfirmed, longmemory.LifecycleActive, longmemory.LifecycleStale, longmemory.LifecycleSuperseded, longmemory.LifecycleInvalidated:
		return true
	default:
		return false
	}
}

func (s *Store) RevokeKnowledgeSource(ctx context.Context, userID, sourceID string) error {
	userID = strings.TrimSpace(userID)
	sourceID = strings.TrimSpace(sourceID)
	if s == nil || s.DB == nil || userID == "" || sourceID == "" {
		return ErrKnowledgeInvalidScope
	}
	tag, err := s.DB.Exec(ctx, revokeKnowledgeSourceSQL, userID, sourceID, time.Now().UnixMilli())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrKnowledgeSourceNotFound
	}
	return nil
}

func (s *Store) RevalidateKnowledgeSource(ctx context.Context, userID, sourceID string) error {
	userID = strings.TrimSpace(userID)
	sourceID = strings.TrimSpace(sourceID)
	if s == nil || s.DB == nil || userID == "" || sourceID == "" {
		return ErrKnowledgeInvalidScope
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var activeRevisionID string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(active_revision_id,'') FROM codelocal_knowledge_sources WHERE user_id=$1 AND source_id=$2 FOR UPDATE`, userID, sourceID).Scan(&activeRevisionID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrKnowledgeSourceNotFound
		}
		return err
	}
	if activeRevisionID == "" {
		return ErrKnowledgeSourceNotFound
	}
	var openConflicts int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM codelocal_knowledge_conflicts WHERE user_id=$1 AND source_id=$2 AND status='open'`, userID, sourceID).Scan(&openConflicts); err != nil {
		return err
	}
	if openConflicts > 0 {
		return ErrKnowledgeRevisionConflict
	}
	var tombstone bool
	if err := tx.QueryRow(ctx, `SELECT tombstone FROM codelocal_knowledge_source_revisions WHERE user_id=$1 AND source_id=$2 AND revision_id=$3`, userID, sourceID, activeRevisionID).Scan(&tombstone); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	status := KnowledgeStatusActive
	var validTo any
	if tombstone {
		status = KnowledgeStatusSuperseded
		validTo = now
	}
	if _, err := tx.Exec(ctx, `UPDATE codelocal_knowledge_sources SET status=$3,last_seen_at=$4,valid_to=$5 WHERE user_id=$1 AND source_id=$2`, userID, sourceID, status, now, validTo); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) SetMemoryLifecycle(ctx context.Context, userID, memoryID string, status longmemory.LifecycleStatus) error {
	userID = strings.TrimSpace(userID)
	memoryID = strings.TrimSpace(memoryID)
	if s == nil || s.DB == nil || userID == "" || memoryID == "" || !validMemoryLifecycle(status) {
		return ErrKnowledgeInvalidScope
	}
	tag, err := s.DB.Exec(ctx, setMemoryLifecycleSQL, userID, memoryID, status, time.Now().UnixMilli())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrKnowledgeSourceNotFound
	}
	return nil
}
