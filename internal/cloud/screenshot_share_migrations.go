package cloud

const screenshotShareMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_screenshot_shares (
  share_id TEXT PRIMARY KEY,
  owner_user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
  asset_id TEXT NOT NULL REFERENCES codelocal_media_assets(asset_id) ON DELETE CASCADE,
  created_at BIGINT NOT NULL,
  deleted_at BIGINT NOT NULL DEFAULT 0,
  CHECK (char_length(share_id) BETWEEN 12 AND 64),
  CHECK (BTRIM(share_id) <> '')
);
CREATE INDEX IF NOT EXISTS idx_codelocal_screenshot_shares_owner_created
 ON codelocal_screenshot_shares(owner_user_id, created_at DESC)
 WHERE deleted_at=0;
CREATE INDEX IF NOT EXISTS idx_codelocal_screenshot_shares_asset
 ON codelocal_screenshot_shares(asset_id)
 WHERE deleted_at=0;
`

func screenshotShareSchemaMigrations() []schemaMigration {
	return []schemaMigration{{59, screenshotShareMigrationSQL}}
}
