package cloud

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrScreenshotShareNotFound  = errors.New("screenshot share not found")
	ErrScreenshotShareForbidden = errors.New("screenshot share forbidden")
	ErrScreenshotShareInvalid   = errors.New("invalid screenshot share")
)

type ScreenshotShare struct {
	ID          string
	OwnerUserID string
	AssetID     string
	ContentType string
	Size        int64
	Width       int
	Height      int
	CreatedAt   int64
}

func NormalizeScreenshotShareID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 12 || len(value) > 64 {
		return ""
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			continue
		}
		return ""
	}
	return value
}

func randomScreenshotShareID() string {
	buffer := make([]byte, 9)
	if _, err := rand.Read(buffer); err == nil {
		return base64.RawURLEncoding.EncodeToString(buffer)
	}
	return RandomHex(6)
}

type screenshotShareRowScanner interface {
	Scan(dest ...any) error
}

const screenshotShareSelect = `
SELECT s.share_id,s.owner_user_id,s.asset_id,a.source_content_type,a.source_size,a.width,a.height,s.created_at
FROM codelocal_screenshot_shares s
JOIN codelocal_media_assets a ON a.asset_id=s.asset_id`

func scanScreenshotShare(row screenshotShareRowScanner) (ScreenshotShare, error) {
	var share ScreenshotShare
	err := row.Scan(&share.ID, &share.OwnerUserID, &share.AssetID, &share.ContentType, &share.Size, &share.Width, &share.Height, &share.CreatedAt)
	return share, err
}

func (s *Store) CreateScreenshotShare(ctx context.Context, ownerUserID, assetID string) (ScreenshotShare, error) {
	ownerUserID = strings.TrimSpace(ownerUserID)
	assetID = NormalizeMediaAssetID(assetID)
	if ownerUserID == "" || assetID == "" {
		return ScreenshotShare{}, ErrScreenshotShareInvalid
	}
	asset, err := s.MediaAssetByID(ctx, assetID)
	if err != nil {
		return ScreenshotShare{}, ErrScreenshotShareInvalid
	}
	if asset.OwnerUserID != ownerUserID {
		return ScreenshotShare{}, ErrScreenshotShareForbidden
	}
	if asset.Status != "ready" || asset.DeletedAt != 0 {
		return ScreenshotShare{}, ErrScreenshotShareInvalid
	}

	now := time.Now().UnixMilli()
	for attempt := 0; attempt < 8; attempt++ {
		shareID := randomScreenshotShareID()
		command, err := s.DB.Exec(ctx, `
INSERT INTO codelocal_screenshot_shares(share_id,owner_user_id,asset_id,created_at)
VALUES($1,$2,$3,$4)
ON CONFLICT(share_id) DO NOTHING`, shareID, ownerUserID, assetID, now)
		if err != nil {
			return ScreenshotShare{}, err
		}
		if command.RowsAffected() == 1 {
			return s.ScreenshotShareByID(ctx, shareID)
		}
	}
	return ScreenshotShare{}, errors.New("could not allocate screenshot share id")
}

func (s *Store) ScreenshotShareByID(ctx context.Context, shareID string) (ScreenshotShare, error) {
	shareID = NormalizeScreenshotShareID(shareID)
	if shareID == "" {
		return ScreenshotShare{}, ErrScreenshotShareNotFound
	}
	share, err := scanScreenshotShare(s.DB.QueryRow(ctx, screenshotShareSelect+`
WHERE s.share_id=$1 AND s.deleted_at=0 AND a.status='ready' AND a.deleted_at=0`, shareID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ScreenshotShare{}, ErrScreenshotShareNotFound
	}
	return share, err
}

func (s *Store) ListScreenshotShares(ctx context.Context, ownerUserID string, limit int) ([]ScreenshotShare, error) {
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		return nil, ErrScreenshotShareInvalid
	}
	if limit < 1 {
		limit = 30
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := s.DB.Query(ctx, screenshotShareSelect+`
WHERE s.owner_user_id=$1 AND s.deleted_at=0 AND a.status='ready' AND a.deleted_at=0
ORDER BY s.created_at DESC
LIMIT $2`, ownerUserID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	shares := make([]ScreenshotShare, 0)
	for rows.Next() {
		share, scanErr := scanScreenshotShare(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		shares = append(shares, share)
	}
	return shares, rows.Err()
}

func (s *Store) DeleteScreenshotShare(ctx context.Context, ownerUserID, shareID string) error {
	ownerUserID = strings.TrimSpace(ownerUserID)
	shareID = NormalizeScreenshotShareID(shareID)
	if ownerUserID == "" || shareID == "" {
		return ErrScreenshotShareInvalid
	}
	command, err := s.DB.Exec(ctx, `
UPDATE codelocal_screenshot_shares
SET deleted_at=$1
WHERE share_id=$2 AND owner_user_id=$3 AND deleted_at=0`, time.Now().UnixMilli(), shareID, ownerUserID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 1 {
		return nil
	}
	if _, err := s.ScreenshotShareByID(ctx, shareID); err == nil {
		return ErrScreenshotShareForbidden
	}
	return ErrScreenshotShareNotFound
}

func (s *Store) ScreenshotShareVariant(ctx context.Context, shareID, variantName string) (MediaVariant, error) {
	shareID = NormalizeScreenshotShareID(shareID)
	variantName = strings.TrimSpace(variantName)
	if shareID == "" {
		return MediaVariant{}, ErrScreenshotShareNotFound
	}
	switch variantName {
	case "original", "thumb", "medium", "large":
	default:
		return MediaVariant{}, ErrScreenshotShareNotFound
	}
	var variant MediaVariant
	err := s.DB.QueryRow(ctx, `
SELECT v.asset_id,v.variant,v.object_key,v.content_type,v.size,v.width,v.height,v.sha256,v.created_at
FROM codelocal_screenshot_shares s
JOIN codelocal_media_assets a ON a.asset_id=s.asset_id
JOIN codelocal_media_variants v ON v.asset_id=a.asset_id
WHERE s.share_id=$1 AND s.deleted_at=0
  AND a.status='ready' AND a.deleted_at=0
  AND v.variant=$2
LIMIT 1`, shareID, variantName).Scan(
		&variant.AssetID, &variant.Variant, &variant.ObjectKey, &variant.ContentType, &variant.Size,
		&variant.Width, &variant.Height, &variant.SHA256, &variant.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return MediaVariant{}, ErrScreenshotShareNotFound
	}
	return variant, err
}
