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

func (s *Store) GetSkillRating(ctx context.Context, userID, skillID string) (int, bool, error) {
	var rating int
	err := s.DB.QueryRow(ctx, `SELECT rating FROM codelocal_skill_ratings WHERE user_id=$1 AND skill_id=$2`, strings.TrimSpace(userID), strings.TrimSpace(skillID)).Scan(&rating)
	if err != nil {
		if isPGXNoRows(err) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return rating, true, nil
}
