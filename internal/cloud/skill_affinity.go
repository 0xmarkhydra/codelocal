package cloud

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"
)

func skillAffinityFreshnessCutoff() int64 {
	days := 180
	if raw := strings.TrimSpace(os.Getenv("CODELOCAL_SKILL_AFFINITY_DAYS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			days = parsed
		}
	}
	if days < 30 {
		days = 30
	}
	if days > 365 {
		days = 365
	}
	return time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()
}

// skillAffinityScore uses a four-sample neutral prior so one lucky/failed run
// cannot dominate routing. The router contract intentionally caps affinity at
// ±0.25; global quality and task relevance still carry more weight.
func skillAffinityScore(successes, failures int64) float64 {
	if successes < 0 {
		successes = 0
	}
	if failures < 0 {
		failures = 0
	}
	total := successes + failures
	if total == 0 {
		return 0
	}
	score := 0.25 * float64(successes-failures) / float64(total+4)
	if score > 0.25 {
		return 0.25
	}
	if score < -0.25 {
		return -0.25
	}
	return score
}

// SkillAffinity derives tenant-private routing preference from already
// verified experiences. It never changes global skill knowledge or collective
// ranking and requires no additional persistent profile table.
func (s *Store) SkillAffinity(ctx context.Context, userID string, skillIDs []string) (map[string]float64, error) {
	out := map[string]float64{}
	if s == nil || s.DB == nil || strings.TrimSpace(userID) == "" || len(skillIDs) == 0 {
		return out, nil
	}
	seen := map[string]struct{}{}
	ids := make([]string, 0, len(skillIDs))
	for _, id := range skillIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
		if len(ids) >= 64 {
			break
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.DB.Query(ctx, `
SELECT skill_id,
 COUNT(*) FILTER (WHERE outcome='succeeded')::bigint,
 COUNT(*) FILTER (WHERE outcome='failed')::bigint
FROM codelocal_experiences
WHERE user_id=$1 AND skill_id=ANY($2::text[]) AND created_at >= $3
GROUP BY skill_id`, userID, ids, skillAffinityFreshnessCutoff())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var successes, failures int64
		if err := rows.Scan(&id, &successes, &failures); err != nil {
			return nil, err
		}
		if score := skillAffinityScore(successes, failures); score != 0 {
			out[id] = score
		}
	}
	return out, rows.Err()
}
