package cloud

import (
	"context"
	"encoding/json"
	"strings"
)

type skillVersionScanner interface {
	Scan(dest ...any) error
}

// StableSkillVersionRecords returns the globally promoted stable versions plus
// the caller's private Personal stable versions. Callers apply Personal-over-
// global precedence and user preference state in memory.
func (s *Store) StableSkillVersionRecords(ctx context.Context, userID string) ([]SkillVersionRecord, error) {
	rows, err := s.DB.Query(ctx, `
SELECT v.record_id, v.tenant_user_id, v.creator_user_id, v.publisher, v.manifest,
       v.state, v.package_hash, v.artifact_hash, v.artifact_uri,
       v.created_at, v.updated_at, v.promoted_at
FROM codelocal_skill_channels c
JOIN codelocal_skill_versions v
  ON v.skill_id = c.skill_id
 AND v.version = c.version
 AND v.tenant_user_id IS NOT DISTINCT FROM c.tenant_user_id
WHERE c.channel = 'stable'
  AND (c.tenant_user_id IS NULL OR c.tenant_user_id = $1)
  AND v.state IN ('active', 'promoted')
ORDER BY c.tenant_user_id NULLS FIRST, v.skill_id`, strings.TrimSpace(userID))
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

func (s *Store) SkillVersionRecordByIdentity(ctx context.Context, tenantUserID, skillID, version string) (SkillVersionRecord, bool, error) {
	row := s.DB.QueryRow(ctx, `
SELECT record_id, tenant_user_id, creator_user_id, publisher, manifest,
       state, package_hash, artifact_hash, artifact_uri,
       created_at, updated_at, promoted_at
FROM codelocal_skill_versions
WHERE skill_id = $1 AND version = $2
  AND tenant_user_id IS NOT DISTINCT FROM NULLIF($3, '')`,
		strings.TrimSpace(skillID), strings.TrimSpace(version), strings.TrimSpace(tenantUserID))
	record, err := scanSkillVersionRecord(row)
	if err != nil {
		if isNoRows(err) {
			return SkillVersionRecord{}, false, nil
		}
		return SkillVersionRecord{}, false, err
	}
	return record, true, nil
}

func (s *Store) ListSkillUserStates(ctx context.Context, userID string) ([]SkillUserState, error) {
	rows, err := s.DB.Query(ctx, `
SELECT user_id, skill_id, mode, pinned_version, updated_at
FROM codelocal_skill_user_states
WHERE user_id = $1
ORDER BY skill_id`, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SkillUserState{}
	for rows.Next() {
		var state SkillUserState
		var pinned *string
		if err := rows.Scan(&state.UserID, &state.SkillID, &state.Mode, &pinned, &state.UpdatedAt); err != nil {
			return nil, err
		}
		if pinned != nil {
			state.PinnedVersion = *pinned
		}
		out = append(out, state)
	}
	return out, rows.Err()
}

func scanSkillVersionRecord(row skillVersionScanner) (SkillVersionRecord, error) {
	var record SkillVersionRecord
	var tenantUserID, creatorUserID *string
	var manifestJSON []byte
	var state string
	if err := row.Scan(
		&record.RecordID, &tenantUserID, &creatorUserID, &record.Publisher, &manifestJSON,
		&state, &record.PackageHash, &record.ArtifactHash, &record.ArtifactURI,
		&record.CreatedAt, &record.UpdatedAt, &record.PromotedAt,
	); err != nil {
		return SkillVersionRecord{}, err
	}
	if tenantUserID != nil {
		record.TenantUserID = *tenantUserID
	}
	if creatorUserID != nil {
		record.CreatorUserID = *creatorUserID
	}
	record.State = SkillVersionState(state)
	if err := json.Unmarshal(manifestJSON, &record.Manifest); err != nil {
		return SkillVersionRecord{}, err
	}
	return record, nil
}
