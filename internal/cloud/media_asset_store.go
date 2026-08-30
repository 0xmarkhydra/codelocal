package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type mediaAssetRowScanner interface { Scan(dest ...any) error }

const mediaAssetSelect = `
SELECT asset_id,owner_user_id,source_sha256,source_content_type,source_size,width,height,status,
       preserve_original,error_code,created_at,updated_at,deleted_at
FROM codelocal_media_assets`

func scanMediaAsset(row mediaAssetRowScanner) (MediaAsset, error) {
	var asset MediaAsset
	err := row.Scan(&asset.ID,&asset.OwnerUserID,&asset.SourceSHA256,&asset.SourceContentType,&asset.SourceSize,
		&asset.Width,&asset.Height,&asset.Status,&asset.PreserveOriginal,&asset.ErrorCode,&asset.CreatedAt,&asset.UpdatedAt,&asset.DeletedAt)
	asset.Variants = []MediaVariant{}
	return asset, err
}

func (s *Store) EnsureMediaAsset(ctx context.Context, ownerUserID, sha256, contentType string, size int64, preserveOriginal bool) (MediaAsset, bool, error) {
	ownerUserID = strings.TrimSpace(ownerUserID)
	sha256 = strings.ToLower(strings.TrimSpace(sha256))
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if ownerUserID == "" || len(sha256) != 64 || size <= 0 || size > 25<<20 || !strings.HasPrefix(contentType, "image/") {
		return MediaAsset{}, false, ErrMediaAssetInvalid
	}
	if existing, err := s.MediaAssetByOwnerHash(ctx, ownerUserID, sha256); err == nil {
		return existing, true, nil
	} else if !errors.Is(err, ErrMediaAssetNotFound) {
		return MediaAsset{}, false, err
	}
	assetID := "media_" + RandomHex(12)
	now := time.Now().UnixMilli()
	_, err := s.DB.Exec(ctx, `
INSERT INTO codelocal_media_assets(asset_id,owner_user_id,source_sha256,source_content_type,source_size,status,preserve_original,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,'processing',$6,$7,$7)
ON CONFLICT(owner_user_id,source_sha256) WHERE deleted_at=0 DO NOTHING`, assetID, ownerUserID, sha256, contentType, size, preserveOriginal, now)
	if err != nil { return MediaAsset{}, false, err }
	asset, err := s.MediaAssetByOwnerHash(ctx, ownerUserID, sha256)
	return asset, asset.ID != assetID, err
}

func (s *Store) MediaAssetByOwnerHash(ctx context.Context, ownerUserID, sha256 string) (MediaAsset, error) {
	asset, err := scanMediaAsset(s.DB.QueryRow(ctx, mediaAssetSelect+` WHERE owner_user_id=$1 AND source_sha256=$2 AND deleted_at=0`, strings.TrimSpace(ownerUserID), strings.ToLower(strings.TrimSpace(sha256))))
	if errors.Is(err, pgx.ErrNoRows) { return MediaAsset{}, ErrMediaAssetNotFound }
	if err != nil { return MediaAsset{}, err }
	return s.loadMediaVariants(ctx, asset)
}

func (s *Store) MediaAssetByID(ctx context.Context, assetID string) (MediaAsset, error) {
	assetID = NormalizeMediaAssetID(assetID)
	if assetID == "" { return MediaAsset{}, ErrMediaAssetNotFound }
	asset, err := scanMediaAsset(s.DB.QueryRow(ctx, mediaAssetSelect+` WHERE asset_id=$1 AND deleted_at=0`, assetID))
	if errors.Is(err, pgx.ErrNoRows) { return MediaAsset{}, ErrMediaAssetNotFound }
	if err != nil { return MediaAsset{}, err }
	return s.loadMediaVariants(ctx, asset)
}

func (s *Store) loadMediaVariants(ctx context.Context, asset MediaAsset) (MediaAsset, error) {
	rows, err := s.DB.Query(ctx, `SELECT asset_id,variant,object_key,content_type,size,width,height,sha256,created_at FROM codelocal_media_variants WHERE asset_id=$1 ORDER BY variant`, asset.ID)
	if err != nil { return MediaAsset{}, err }
	defer rows.Close()
	for rows.Next() {
		var variant MediaVariant
		if err := rows.Scan(&variant.AssetID,&variant.Variant,&variant.ObjectKey,&variant.ContentType,&variant.Size,&variant.Width,&variant.Height,&variant.SHA256,&variant.CreatedAt); err != nil { return MediaAsset{}, err }
		asset.Variants = append(asset.Variants, variant)
	}
	return asset, rows.Err()
}

func (s *Store) CompleteMediaAsset(ctx context.Context, ownerUserID, assetID string, width, height int, variants []MediaVariant) (MediaAsset, error) {
	asset, err := s.MediaAssetByID(ctx, assetID)
	if err != nil { return MediaAsset{}, err }
	if asset.OwnerUserID != strings.TrimSpace(ownerUserID) { return MediaAsset{}, ErrMediaAssetForbidden }
	if width <= 0 || height <= 0 || len(variants) == 0 { return MediaAsset{}, ErrMediaAssetInvalid }
	tx, err := s.DB.Begin(ctx)
	if err != nil { return MediaAsset{}, err }
	defer func(){ _ = tx.Rollback(ctx) }()
	now := time.Now().UnixMilli()
	if _, err = tx.Exec(ctx, `DELETE FROM codelocal_media_variants WHERE asset_id=$1`, asset.ID); err != nil { return MediaAsset{}, err }
	for _, variant := range variants {
		if variant.Variant != "original" && variant.Variant != "thumb" && variant.Variant != "medium" && variant.Variant != "large" { return MediaAsset{}, ErrMediaAssetInvalid }
		if variant.ObjectKey == "" || variant.ContentType == "" || variant.Size <= 0 || variant.Width <= 0 || variant.Height <= 0 || variant.SHA256 == "" { return MediaAsset{}, ErrMediaAssetInvalid }
		if _, err = tx.Exec(ctx, `INSERT INTO codelocal_media_variants(asset_id,variant,object_key,content_type,size,width,height,sha256,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, asset.ID, variant.Variant, variant.ObjectKey, variant.ContentType, variant.Size, variant.Width, variant.Height, variant.SHA256, now); err != nil { return MediaAsset{}, err }
	}
	if _, err = tx.Exec(ctx, `UPDATE codelocal_media_assets SET width=$1,height=$2,status='ready',error_code='',updated_at=$3 WHERE asset_id=$4`, width,height,now,asset.ID); err != nil { return MediaAsset{}, err }
	if err := tx.Commit(ctx); err != nil { return MediaAsset{}, err }
	return s.MediaAssetByID(ctx, asset.ID)
}

func (s *Store) FailMediaAsset(ctx context.Context, ownerUserID, assetID, code string) error {
	command, err := s.DB.Exec(ctx, `UPDATE codelocal_media_assets SET status='failed',error_code=$1,updated_at=$2 WHERE asset_id=$3 AND owner_user_id=$4 AND deleted_at=0`, strings.TrimSpace(code), time.Now().UnixMilli(), NormalizeMediaAssetID(assetID), strings.TrimSpace(ownerUserID))
	if err != nil { return err }
	if command.RowsAffected() != 1 { return ErrMediaAssetNotFound }
	return nil
}

func (s *Store) MediaAssetOwnedReady(ctx context.Context, ownerUserID, assetID string) (bool, error) {
	if strings.TrimSpace(assetID) == "" { return true, nil }
	var ready bool
	err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM codelocal_media_assets WHERE asset_id=$1 AND owner_user_id=$2 AND status='ready' AND deleted_at=0)`, NormalizeMediaAssetID(assetID), strings.TrimSpace(ownerUserID)).Scan(&ready)
	return ready, err
}

func (s *Store) SyncMediaAssetRefs(ctx context.Context, ownerUserID, refKind, refID string, slots map[string]string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil { return err }
	defer func(){ _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `DELETE FROM codelocal_media_asset_refs WHERE owner_user_id=$1 AND ref_kind=$2 AND ref_id=$3`, ownerUserID, refKind, refID); err != nil { return err }
	now := time.Now().UnixMilli()
	for slot, assetID := range slots {
		if strings.TrimSpace(assetID) == "" { continue }
		if _, err = tx.Exec(ctx, `INSERT INTO codelocal_media_asset_refs(asset_id,owner_user_id,ref_kind,ref_id,slot,created_at) VALUES($1,$2,$3,$4,$5,$6)`, assetID,ownerUserID,refKind,refID,slot,now); err != nil { return err }
	}
	return tx.Commit(ctx)
}

func (s *Store) UpsertMediaUnderstanding(ctx context.Context, ownerUserID string, input MediaUnderstanding) (MediaUnderstanding, error) {
	asset, err := s.MediaAssetByID(ctx, input.AssetID)
	if err != nil { return MediaUnderstanding{}, err }
	if asset.OwnerUserID != strings.TrimSpace(ownerUserID) { return MediaUnderstanding{}, ErrMediaAssetForbidden }
	tags, _ := json.Marshal(normalizeBlogTags(input.Tags))
	now := time.Now().UnixMilli()
	_, err = s.DB.Exec(ctx, `INSERT INTO codelocal_media_understanding(asset_id,summary,alt_text,extracted_text,tags,language,provider,model,analyzed_at,updated_at) VALUES($1,$2,$3,$4,$5::jsonb,$6,$7,$8,$9,$10) ON CONFLICT(asset_id) DO UPDATE SET summary=EXCLUDED.summary,alt_text=EXCLUDED.alt_text,extracted_text=EXCLUDED.extracted_text,tags=EXCLUDED.tags,language=EXCLUDED.language,provider=EXCLUDED.provider,model=EXCLUDED.model,analyzed_at=EXCLUDED.analyzed_at,updated_at=EXCLUDED.updated_at`, input.AssetID,input.Summary,input.AltText,input.ExtractedText,string(tags),input.Language,input.Provider,input.Model,input.AnalyzedAt,now)
	input.UpdatedAt = now
	return input, err
}
