package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/skills"
	"github.com/jackc/pgx/v5"
)

type SkillVersionState string

const (
	SkillVersionActive     SkillVersionState = "active"
	SkillVersionCandidate  SkillVersionState = "candidate"
	SkillVersionEvaluating SkillVersionState = "evaluating"
	SkillVersionCanary     SkillVersionState = "canary"
	SkillVersionPromoted   SkillVersionState = "promoted"
	SkillVersionRejected   SkillVersionState = "rejected"
	SkillVersionRolledBack SkillVersionState = "rolled_back"
	SkillVersionDeprecated SkillVersionState = "deprecated"
	SkillVersionBlocked    SkillVersionState = "blocked"
)

type SkillVersionRecord struct {
	RecordID      string            `json:"recordId"`
	TenantUserID  string            `json:"tenantUserId,omitempty"`
	CreatorUserID string            `json:"creatorUserId,omitempty"`
	Publisher     string            `json:"publisher"`
	Manifest      skills.Manifest   `json:"manifest"`
	State         SkillVersionState `json:"state"`
	PackageHash   string            `json:"packageHash"`
	ArtifactHash  string            `json:"artifactHash"`
	ArtifactURI   string            `json:"artifactUri"`
	CreatedAt     int64             `json:"createdAt"`
	UpdatedAt     int64             `json:"updatedAt"`
	PromotedAt    int64             `json:"promotedAt,omitempty"`
}

type SkillUserState struct {
	UserID        string `json:"userId"`
	SkillID       string `json:"skillId"`
	Mode          string `json:"mode"`
	PinnedVersion string `json:"pinnedVersion,omitempty"`
	UpdatedAt     int64  `json:"updatedAt"`
}

func NewSkillVersionRecord(tenantUserID, creatorUserID string, pkg skills.Package, artifactURI string, state SkillVersionState) (SkillVersionRecord, error) {
	if err := skills.ValidatePackageIntegrity(pkg); err != nil {
		return SkillVersionRecord{}, err
	}
	tenantUserID = strings.TrimSpace(tenantUserID)
	creatorUserID = strings.TrimSpace(creatorUserID)
	artifactURI = strings.TrimSpace(artifactURI)
	if artifactURI == "" {
		return SkillVersionRecord{}, fmt.Errorf("skill artifact URI is required")
	}
	if pkg.Manifest.Scope == skills.ScopePersonal && tenantUserID == "" {
		return SkillVersionRecord{}, fmt.Errorf("personal skill requires tenant user")
	}
	if pkg.Manifest.Scope != skills.ScopePersonal && tenantUserID != "" {
		return SkillVersionRecord{}, fmt.Errorf("shared skill cannot be tenant-scoped")
	}
	publisher, err := authoritativeSkillPublisher(pkg.Manifest, creatorUserID)
	if err != nil {
		return SkillVersionRecord{}, err
	}
	if !validSkillVersionState(state) {
		return SkillVersionRecord{}, fmt.Errorf("invalid skill version state %q", state)
	}
	now := time.Now().UnixMilli()
	return SkillVersionRecord{
		RecordID:      skillRegistryID("version", tenantUserID, pkg.Manifest.ID, pkg.Manifest.Version),
		TenantUserID:  tenantUserID,
		CreatorUserID: creatorUserID,
		Publisher:     publisher,
		Manifest:      pkg.Manifest,
		State:         state,
		PackageHash:   pkg.PackageHash,
		ArtifactHash:  pkg.Artifact.Manifest.ContentHash,
		ArtifactURI:   artifactURI,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

func authoritativeSkillPublisher(manifest skills.Manifest, creatorUserID string) (string, error) {
	creatorUserID = strings.TrimSpace(creatorUserID)
	switch manifest.Scope {
	case skills.ScopePersonal, skills.ScopeCommunity:
		if creatorUserID == "" {
			return "", fmt.Errorf("%s skill requires authenticated creator", manifest.Scope)
		}
		// Never trust a user-supplied manifest publisher as account identity. The
		// package keeps the claimed publisher under its signed hash for provenance,
		// while catalog/market authority comes from the authenticated creator.
		return creatorUserID, nil
	case skills.ScopeSystem:
		publisher := strings.TrimSpace(manifest.Publisher)
		if publisher == "" {
			return "", fmt.Errorf("system skill publisher is required")
		}
		return publisher, nil
	default:
		return "", fmt.Errorf("unsupported skill scope %q", manifest.Scope)
	}
}

func (s *Store) CreateSkillVersion(ctx context.Context, record SkillVersionRecord) (bool, error) {
	if s == nil || s.DB == nil {
		return false, fmt.Errorf("cloud store is unavailable")
	}
	if strings.TrimSpace(record.RecordID) == "" || strings.TrimSpace(record.Manifest.ID) == "" || strings.TrimSpace(record.Manifest.Version) == "" || strings.TrimSpace(record.Publisher) == "" {
		return false, fmt.Errorf("skill version identity is required")
	}
	manifestJSON, err := json.Marshal(record.Manifest)
	if err != nil {
		return false, err
	}
	tag, err := s.DB.Exec(ctx, `
INSERT INTO codelocal_skill_versions (
  record_id, skill_id, version, tenant_user_id, creator_user_id, scope, kind,
  publisher, state, verified, manifest, package_hash, artifact_hash, artifact_uri,
  created_at, updated_at, promoted_at
) VALUES (
  $1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, $7,
  $8, $9, $10, $11::jsonb, $12, $13, $14, $15, $16, $17
)
ON CONFLICT (record_id) DO NOTHING`,
		record.RecordID, record.Manifest.ID, record.Manifest.Version, record.TenantUserID,
		record.CreatorUserID, record.Manifest.Scope, record.Manifest.Kind, record.Publisher,
		record.State, record.Manifest.Verified, string(manifestJSON), record.PackageHash,
		record.ArtifactHash, record.ArtifactURI, record.CreatedAt, record.UpdatedAt, record.PromotedAt,
	)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 1 {
		return true, nil
	}
	var packageHash, artifactHash, artifactURI, publisher string
	var creatorUserID *string
	err = s.DB.QueryRow(ctx, `
SELECT package_hash, artifact_hash, artifact_uri, publisher, creator_user_id
FROM codelocal_skill_versions
WHERE record_id = $1`, record.RecordID).Scan(&packageHash, &artifactHash, &artifactURI, &publisher, &creatorUserID)
	if err != nil {
		return false, err
	}
	existingCreator := ""
	if creatorUserID != nil {
		existingCreator = *creatorUserID
	}
	if packageHash != record.PackageHash || artifactHash != record.ArtifactHash || artifactURI != record.ArtifactURI || publisher != record.Publisher || existingCreator != record.CreatorUserID {
		return false, fmt.Errorf("immutable skill version %s@%s already exists with different content or authority", record.Manifest.ID, record.Manifest.Version)
	}
	return false, nil
}

func (s *Store) SetSkillChannel(ctx context.Context, tenantUserID, skillID, channel, version string) error {
	if s == nil || s.DB == nil {
		return fmt.Errorf("cloud store is unavailable")
	}
	tenantUserID = strings.TrimSpace(tenantUserID)
	skillID = strings.TrimSpace(skillID)
	channel = strings.TrimSpace(channel)
	version = strings.TrimSpace(version)
	if skillID == "" || version == "" || (channel != "stable" && channel != "canary") {
		return fmt.Errorf("invalid skill channel update")
	}
	var exists bool
	err := s.DB.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM codelocal_skill_versions
  WHERE skill_id = $1 AND version = $2
    AND tenant_user_id IS NOT DISTINCT FROM NULLIF($3, '')
)`, skillID, version, tenantUserID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("skill %s@%s does not exist for channel", skillID, version)
	}
	now := time.Now().UnixMilli()
	channelID := skillRegistryID("channel", tenantUserID, skillID, channel)
	_, err = s.DB.Exec(ctx, `
INSERT INTO codelocal_skill_channels (channel_id, skill_id, tenant_user_id, channel, version, updated_at)
VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6)
ON CONFLICT (channel_id) DO UPDATE SET
  version = EXCLUDED.version,
  updated_at = EXCLUDED.updated_at`,
		channelID, skillID, tenantUserID, channel, version, now,
	)
	return err
}

func (s *Store) SkillChannelVersion(ctx context.Context, tenantUserID, skillID, channel string) (string, bool, error) {
	if s == nil || s.DB == nil {
		return "", false, fmt.Errorf("cloud store is unavailable")
	}
	var version string
	err := s.DB.QueryRow(ctx, `
SELECT version FROM codelocal_skill_channels
WHERE skill_id = $1 AND channel = $2
  AND tenant_user_id IS NOT DISTINCT FROM NULLIF($3, '')`,
		strings.TrimSpace(skillID), strings.TrimSpace(channel), strings.TrimSpace(tenantUserID),
	).Scan(&version)
	if err == pgx.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return version, true, nil
}

func (s *Store) SetSkillUserState(ctx context.Context, state SkillUserState) error {
	if s == nil || s.DB == nil {
		return fmt.Errorf("cloud store is unavailable")
	}
	state.UserID = strings.TrimSpace(state.UserID)
	state.SkillID = strings.TrimSpace(state.SkillID)
	state.Mode = strings.TrimSpace(state.Mode)
	state.PinnedVersion = strings.TrimSpace(state.PinnedVersion)
	if state.UserID == "" || state.SkillID == "" {
		return fmt.Errorf("skill user state identity is required")
	}
	if state.Mode == "" {
		state.Mode = "auto"
	}
	if state.Mode != "auto" && state.Mode != "prefer" && state.Mode != "disabled" {
		return fmt.Errorf("invalid skill user state mode %q", state.Mode)
	}
	state.UpdatedAt = time.Now().UnixMilli()
	_, err := s.DB.Exec(ctx, `
INSERT INTO codelocal_skill_user_states (user_id, skill_id, mode, pinned_version, updated_at)
VALUES ($1, $2, $3, NULLIF($4, ''), $5)
ON CONFLICT (user_id, skill_id) DO UPDATE SET
  mode = EXCLUDED.mode,
  pinned_version = EXCLUDED.pinned_version,
  updated_at = EXCLUDED.updated_at`,
		state.UserID, state.SkillID, state.Mode, state.PinnedVersion, state.UpdatedAt,
	)
	return err
}

func (s *Store) GetSkillUserState(ctx context.Context, userID, skillID string) (SkillUserState, bool, error) {
	if s == nil || s.DB == nil {
		return SkillUserState{}, false, fmt.Errorf("cloud store is unavailable")
	}
	state := SkillUserState{UserID: strings.TrimSpace(userID), SkillID: strings.TrimSpace(skillID)}
	var pinned *string
	err := s.DB.QueryRow(ctx, `
SELECT mode, pinned_version, updated_at
FROM codelocal_skill_user_states
WHERE user_id = $1 AND skill_id = $2`, state.UserID, state.SkillID).Scan(&state.Mode, &pinned, &state.UpdatedAt)
	if err == pgx.ErrNoRows {
		return SkillUserState{}, false, nil
	}
	if err != nil {
		return SkillUserState{}, false, err
	}
	if pinned != nil {
		state.PinnedVersion = *pinned
	}
	return state, true, nil
}

func validSkillVersionState(state SkillVersionState) bool {
	switch state {
	case SkillVersionActive, SkillVersionCandidate, SkillVersionEvaluating, SkillVersionCanary,
		SkillVersionPromoted, SkillVersionRejected, SkillVersionRolledBack,
		SkillVersionDeprecated, SkillVersionBlocked:
		return true
	default:
		return false
	}
}

func skillRegistryID(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte(strings.TrimSpace(part)))
		_, _ = hash.Write([]byte{0})
	}
	return "skill_" + hex.EncodeToString(hash.Sum(nil)[:16])
}
