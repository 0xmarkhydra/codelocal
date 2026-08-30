package cloud

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Store) PublicBlogMediaVariant(ctx context.Context, assetID, variantName string) (MediaVariant, error) {
	assetID = NormalizeMediaAssetID(assetID)
	variantName = strings.TrimSpace(variantName)
	if assetID == "" {
		return MediaVariant{}, ErrMediaAssetNotFound
	}
	switch variantName {
	case "original", "thumb", "medium", "large":
	default:
		return MediaVariant{}, ErrMediaAssetNotFound
	}
	var variant MediaVariant
	err := s.DB.QueryRow(ctx, `
SELECT v.asset_id,v.variant,v.object_key,v.content_type,v.size,v.width,v.height,v.sha256,v.created_at
FROM codelocal_media_variants v
JOIN codelocal_media_assets a ON a.asset_id=v.asset_id
WHERE v.asset_id=$1 AND v.variant=$2
  AND a.status='ready' AND a.deleted_at=0
  AND EXISTS (
    SELECT 1
    FROM codelocal_media_asset_refs r
    JOIN codelocal_blog_posts p ON p.post_id=r.ref_id
    WHERE r.asset_id=a.asset_id
      AND r.ref_kind='blog_post'
      AND p.deleted_at=0
      AND p.status='published'
      AND p.visibility='public'
      AND p.moderation_status='clean'
  )
LIMIT 1`, assetID, variantName).Scan(
		&variant.AssetID, &variant.Variant, &variant.ObjectKey, &variant.ContentType, &variant.Size,
		&variant.Width, &variant.Height, &variant.SHA256, &variant.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return MediaVariant{}, ErrMediaAssetNotFound
	}
	return variant, err
}
