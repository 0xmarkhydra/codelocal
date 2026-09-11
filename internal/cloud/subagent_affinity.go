package cloud

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"
)

// Subagent routing affinity mirrors the SkillAffinity contract: tenant-private
// preference learned only from VERIFIED team outcomes, with a neutral prior so
// a handful of anecdotal runs cannot dominate routing. Join-verified success
// raises affinity; evidenced blocked/failed outcomes lower it. Unverified
// completions never participate.
//
// Bounds: the resolver caps the learned boost at ±0.10 while semantic+constraint
// scores carry the primary weight, the same shape as the skills router
// (±0.25 affinity cap there covers a larger reusable catalog).

const (
	subagentAffinityMinSamplesDefault = 3
	subagentAffinityCapDefault        = 0.10
)

func subagentAffinityMinSamples() int {
	raw := strings.TrimSpace(os.Getenv("CODELOCAL_SUBAGENT_AFFINITY_MIN_SAMPLES"))
	if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
		if parsed > 32 {
			return 32
		}
		return parsed
	}
	return subagentAffinityMinSamplesDefault
}

func subagentAffinityFreshnessCutoff() int64 {
	days := 180
	if raw := strings.TrimSpace(os.Getenv("CODELOCAL_SUBAGENT_AFFINITY_DAYS")); raw != "" {
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

// subagentAffinityScore applies Laplace-style smoothing: 4 prior pseudo-samples
// keep one lucky run from dominating, and the ±0.10 cap keeps learned history
// strictly advisory next to semantic/constraint scores.
func subagentAffinityScore(successes, failures int64) float64 {
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
	score := subagentAffinityCapDefault * float64(successes-failures) / float64(total+4)
	if score > subagentAffinityCapDefault {
		return subagentAffinityCapDefault
	}
	if score < -subagentAffinityCapDefault {
		return -subagentAffinityCapDefault
	}
	return score
}

// SubagentAffinity returns per-subagent routing boosts keyed by subagent name.
// Unknown ids are absent (neutral 0). Tenant-private: no cross-user leakage,
// no change to global knowledge, works with zero rows (empty map).
func (s *Store) SubagentAffinity(ctx context.Context, userID string, subagentIDs []string) (map[string]float64, error) {
	out := map[string]float64{}
	if s == nil || s.DB == nil || strings.TrimSpace(userID) == "" || len(subagentIDs) == 0 {
		return out, nil
	}
	seen := map[string]struct{}{}
	ids := make([]string, 0, len(subagentIDs))
	for _, id := range subagentIDs {
		id = strings.ToLower(strings.TrimSpace(id))
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
	minSamples := int64(subagentAffinityMinSamples())
	rows, err := s.DB.Query(ctx, `
SELECT subagent_id,
 COUNT(*) FILTER (WHERE outcome='succeeded')::bigint,
 COUNT(*) FILTER (WHERE outcome='failed')::bigint
FROM codelocal_experiences
WHERE user_id=$1 AND subagent_verified AND subagent_id=ANY($2::text[]) AND created_at >= $3
GROUP BY subagent_id`, userID, ids, subagentAffinityFreshnessCutoff())
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
		if successes+failures < minSamples {
			continue
		}
		if score := subagentAffinityScore(successes, failures); score != 0 {
			out[strings.ToLower(strings.TrimSpace(id))] = score
		}
	}
	return out, rows.Err()
}
