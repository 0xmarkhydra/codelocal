package cloud

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

type SkillVersionRef struct {
	SkillID string
	Version string
}

type SkillQualitySignal struct {
	SkillID         string  `json:"skillId"`
	Version         string  `json:"version"`
	EvaluationScore float64 `json:"evaluationScore"`
	RatingAverage   float64 `json:"ratingAverage,omitempty"`
	RatingCount     int64   `json:"ratingCount,omitempty"`
	Quality         float64 `json:"quality"`
}

func (s *Store) SetSkillRating(ctx context.Context, userID, skillID string, rating int) error {
	if s == nil || s.DB == nil {
		return fmt.Errorf("cloud store is unavailable")
	}
	userID = strings.TrimSpace(userID)
	skillID = strings.TrimSpace(skillID)
	if userID == "" || skillID == "" {
		return fmt.Errorf("skill rating requires user and skill")
	}
	if rating < 1 || rating > 5 {
		return fmt.Errorf("skill rating must be between 1 and 5")
	}
	_, err := s.DB.Exec(ctx, `
INSERT INTO codelocal_skill_ratings(user_id, skill_id, rating, updated_at)
VALUES($1,$2,$3,$4)
ON CONFLICT(user_id, skill_id) DO UPDATE SET rating=EXCLUDED.rating, updated_at=EXCLUDED.updated_at`,
		userID, skillID, rating, time.Now().UnixMilli())
	return err
}

// SkillQualitySignals derives Community routing quality from trusted evaluation
// evidence plus explicit ratings. Creator-supplied Manifest.Quality is not used
// as the authoritative global ranking signal.
func (s *Store) SkillQualitySignals(ctx context.Context, refs []SkillVersionRef) (map[string]SkillQualitySignal, error) {
	out := map[string]SkillQualitySignal{}
	if s == nil || s.DB == nil || len(refs) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(refs))
	wanted := map[string]map[string]struct{}{}
	seenID := map[string]struct{}{}
	for _, ref := range refs {
		id := strings.TrimSpace(ref.SkillID)
		version := strings.TrimSpace(ref.Version)
		if id == "" || version == "" {
			continue
		}
		if wanted[id] == nil {
			wanted[id] = map[string]struct{}{}
		}
		wanted[id][version] = struct{}{}
		if _, seen := seenID[id]; !seen {
			seenID[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}

	rows, err := s.DB.Query(ctx, `
SELECT DISTINCT ON (skill_id, version) skill_id, version, score
FROM codelocal_skill_evaluations
WHERE skill_id=ANY($1::text[]) AND decision='passed'
ORDER BY skill_id, version, created_at DESC`, ids)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, version string
		var score float64
		if err := rows.Scan(&id, &version, &score); err != nil {
			rows.Close()
			return nil, err
		}
		if _, ok := wanted[id][version]; !ok {
			continue
		}
		key := skillQualityKey(id, version)
		out[key] = SkillQualitySignal{SkillID: id, Version: version, EvaluationScore: clampSkillScore(score)}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	ratingRows, err := s.DB.Query(ctx, `
SELECT skill_id, AVG(rating)::double precision, COUNT(*)::bigint
FROM codelocal_skill_ratings
WHERE skill_id=ANY($1::text[])
GROUP BY skill_id`, ids)
	if err != nil {
		return nil, err
	}
	ratings := map[string]struct {
		average float64
		count   int64
	}{}
	for ratingRows.Next() {
		var id string
		var average float64
		var count int64
		if err := ratingRows.Scan(&id, &average, &count); err != nil {
			ratingRows.Close()
			return nil, err
		}
		ratings[id] = struct {
			average float64
			count   int64
		}{average: average, count: count}
	}
	if err := ratingRows.Err(); err != nil {
		ratingRows.Close()
		return nil, err
	}
	ratingRows.Close()

	for key, signal := range out {
		if rating, ok := ratings[signal.SkillID]; ok {
			signal.RatingAverage = math.Round(rating.average*100) / 100
			signal.RatingCount = rating.count
		}
		ratingQuality := bayesianRatingQuality(signal.RatingAverage, signal.RatingCount)
		if signal.RatingCount == 0 {
			signal.Quality = signal.EvaluationScore
		} else {
			signal.Quality = clampSkillScore(signal.EvaluationScore*0.8 + ratingQuality*0.2)
		}
		out[key] = signal
	}
	return out, nil
}

func bayesianRatingQuality(average float64, count int64) float64 {
	if count <= 0 {
		return 0.7
	}
	const priorRating = 3.5
	const priorWeight = 5.0
	weighted := (average*float64(count) + priorRating*priorWeight) / (float64(count) + priorWeight)
	return clampSkillScore(weighted / 5.0)
}

func skillQualityKey(skillID, version string) string {
	return strings.TrimSpace(skillID) + "@" + strings.TrimSpace(version)
}

func clampSkillScore(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
