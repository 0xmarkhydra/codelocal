package cloud

import (
	"context"
	"fmt"
)

const defaultLegacyNoiseCleanupBatch = 500

const quarantineLegacyOperationalNoiseSQL = `
WITH noisy AS (
 SELECT user_id,id
 FROM codelocal_memories
 WHERE source_type='task'
   AND lifecycle_status NOT IN ('invalidated','superseded')
   AND (
    summary LIKE 'Files edited for task:%'
    OR summary LIKE 'Verification evidence refreshed;%'
    OR summary LIKE 'Fresh verification evidence reached the ready quality gate for task:%'
    OR summary LIKE 'Agent quality gate reached ready state for task:%'
    OR summary ~ '^[^[:space:]]+ failed while working on task:'
   )
 ORDER BY updated_at ASC,created_at ASC
 LIMIT $2
),
invalidated AS (
 UPDATE codelocal_memories m
 SET lifecycle_status='invalidated',updated_at=$1
 FROM noisy n
 WHERE m.user_id=n.user_id AND m.id=n.id
 RETURNING m.user_id,m.id
),
closed_nodes AS (
 UPDATE codelocal_memory_nodes n
 SET valid_to=$1,last_seen_at=$1
 FROM invalidated i
 WHERE n.user_id=i.user_id AND n.source_memory_id=i.id AND n.valid_to IS NULL
 RETURNING n.user_id,n.id
),
closed_edges AS (
 UPDATE codelocal_memory_edges e
 SET valid_to=$1,last_seen_at=$1
 WHERE e.valid_to IS NULL AND (
  EXISTS (SELECT 1 FROM invalidated i WHERE i.user_id=e.user_id AND i.id=e.source_memory_id)
  OR EXISTS (SELECT 1 FROM closed_nodes n WHERE n.user_id=e.user_id AND (n.id=e.from_node_id OR n.id=e.to_node_id))
 )
 RETURNING e.id
),
deleted_aliases AS (
 DELETE FROM codelocal_memory_node_aliases a
 USING closed_nodes n
 WHERE a.user_id=n.user_id AND a.node_id=n.id
 RETURNING a.id
),
deleted_sources AS (
 DELETE FROM codelocal_memory_sources s
 USING invalidated i
 WHERE s.user_id=i.user_id AND s.source_memory_id=i.id
 RETURNING s.id
)
SELECT COUNT(*)::int FROM invalidated`

func (s *Store) quarantineLegacyOperationalNoiseBatch(ctx context.Context, now int64, limit int) (int, error) {
	if s == nil || s.DB == nil {
		return 0, fmt.Errorf("legacy knowledge cleanup store unavailable")
	}
	if limit <= 0 {
		limit = defaultLegacyNoiseCleanupBatch
	}
	var count int
	if err := s.DB.QueryRow(ctx, quarantineLegacyOperationalNoiseSQL, now, limit).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
