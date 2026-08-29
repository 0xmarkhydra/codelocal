package cloud

import (
	"context"
	"strings"
)

// ListSharedSkillVersions returns global System/Community versions for creator
// and admin management surfaces. It never returns Personal tenant versions.
func (s *Store) ListSharedSkillVersions(ctx context.Context) ([]SkillVersionRecord, error) {
	rows, err := s.DB.Query(ctx, `
SELECT record_id, tenant_user_id, creator_user_id, publisher, manifest,
       state, package_hash, artifact_hash, artifact_uri,
       created_at, updated_at, promoted_at
FROM codelocal_skill_versions
WHERE tenant_user_id IS NULL
ORDER BY skill_id, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SkillVersionRecord{}
	for rows.Next() {
		record, scanErr := scanSkillVersionRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *Store) ListPersonalSkillVersions(ctx context.Context, userID string) ([]SkillVersionRecord, error) {
	rows, err := s.DB.Query(ctx, `
SELECT record_id, tenant_user_id, creator_user_id, publisher, manifest,
       state, package_hash, artifact_hash, artifact_uri,
       created_at, updated_at, promoted_at
FROM codelocal_skill_versions
WHERE tenant_user_id = $1
ORDER BY skill_id, created_at DESC`, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SkillVersionRecord{}
	for rows.Next() {
		record, scanErr := scanSkillVersionRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *Store) SkillRatingsForUser(ctx context.Context, userID string, skillIDs []string) (map[string]int, error) {
	out := map[string]int{}
	userID = strings.TrimSpace(userID)
	seen := map[string]struct{}{}
	ids := make([]string, 0, len(skillIDs))
	for _, skillID := range skillIDs {
		skillID = strings.TrimSpace(skillID)
		if skillID == "" {
			continue
		}
		if _, duplicate := seen[skillID]; duplicate {
			continue
		}
		seen[skillID] = struct{}{}
		ids = append(ids, skillID)
	}
	if userID == "" || len(ids) == 0 {
		return out, nil
	}
	rows, err := s.DB.Query(ctx, `
SELECT skill_id, rating
FROM codelocal_skill_ratings
WHERE user_id=$1 AND skill_id=ANY($2::text[])`, userID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var skillID string
		var rating int
		if err := rows.Scan(&skillID, &rating); err != nil {
			return nil, err
		}
		out[skillID] = rating
	}
	return out, rows.Err()
}

func (s *Store) GetSkillRating(ctx context.Context, userID, skillID string) (int, bool, error) {
	ratings, err := s.SkillRatingsForUser(ctx, userID, []string{skillID})
	if err != nil {
		return 0, false, err
	}
	rating, ok := ratings[strings.TrimSpace(skillID)]
	return rating, ok, nil
}
