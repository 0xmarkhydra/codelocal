package cloud

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func syncMediaAssetRefsTx(ctx context.Context, tx pgx.Tx, ownerUserID, refKind, refID string, slots map[string]string) error {
	ownerUserID = strings.TrimSpace(ownerUserID)
	refKind = strings.TrimSpace(refKind)
	refID = strings.TrimSpace(refID)
	if ownerUserID == "" || refKind == "" || refID == "" {
		return ErrMediaAssetInvalid
	}
	if _, err := tx.Exec(ctx, `DELETE FROM codelocal_media_asset_refs WHERE owner_user_id=$1 AND ref_kind=$2 AND ref_id=$3`, ownerUserID, refKind, refID); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	for slot, rawAssetID := range slots {
		slot = strings.TrimSpace(slot)
		assetID := NormalizeMediaAssetID(rawAssetID)
		if slot == "" || assetID == "" {
			return ErrMediaAssetInvalid
		}
		command, err := tx.Exec(ctx, `
INSERT INTO codelocal_media_asset_refs(asset_id,owner_user_id,ref_kind,ref_id,slot,created_at)
SELECT a.asset_id,$2,$3,$4,$5,$6
FROM codelocal_media_assets a
WHERE a.asset_id=$1 AND a.owner_user_id=$2 AND a.status='ready' AND a.deleted_at=0`, assetID, ownerUserID, refKind, refID, slot, now)
		if err != nil {
			return err
		}
		if command.RowsAffected() != 1 {
			return ErrMediaAssetForbidden
		}
	}
	return nil
}

func (s *Store) SyncMediaAssetRefs(ctx context.Context, ownerUserID, refKind, refID string, slots map[string]string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := syncMediaAssetRefsTx(ctx, tx, ownerUserID, refKind, refID, slots); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
