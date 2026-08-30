package cloud

import (
	"context"
	"strings"
	"time"
)

func (s *Store) ResetMediaAssetProcessing(ctx context.Context, ownerUserID, assetID string, preserveOriginal bool) (MediaAsset, error) {
	assetID = NormalizeMediaAssetID(assetID)
	ownerUserID = strings.TrimSpace(ownerUserID)
	if assetID == "" || ownerUserID == "" {
		return MediaAsset{}, ErrMediaAssetInvalid
	}
	command, err := s.DB.Exec(ctx, `
UPDATE codelocal_media_assets
SET status='processing',preserve_original=$1,error_code='',updated_at=$2
WHERE asset_id=$3 AND owner_user_id=$4 AND deleted_at=0 AND status='failed'`, preserveOriginal, time.Now().UnixMilli(), assetID, ownerUserID)
	if err != nil {
		return MediaAsset{}, err
	}
	if command.RowsAffected() != 1 {
		return MediaAsset{}, ErrMediaAssetForbidden
	}
	return s.MediaAssetByID(ctx, assetID)
}
