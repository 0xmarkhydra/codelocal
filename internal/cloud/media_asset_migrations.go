package cloud

const durableMediaAssetMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_media_assets (
  asset_id TEXT PRIMARY KEY,
  owner_user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
  source_sha256 TEXT NOT NULL,
  source_content_type TEXT NOT NULL,
  source_size BIGINT NOT NULL,
  width INTEGER NOT NULL DEFAULT 0,
  height INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'processing' CHECK (status IN ('processing','ready','failed','deleted')),
  preserve_original BOOLEAN NOT NULL DEFAULT FALSE,
  error_code TEXT NOT NULL DEFAULT '',
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  deleted_at BIGINT NOT NULL DEFAULT 0,
  CHECK (source_size > 0),
  CHECK (BTRIM(source_sha256) <> ''),
  CHECK (BTRIM(source_content_type) <> '')
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_codelocal_media_assets_owner_hash
 ON codelocal_media_assets(owner_user_id, source_sha256)
 WHERE deleted_at=0;
CREATE INDEX IF NOT EXISTS idx_codelocal_media_assets_owner_updated
 ON codelocal_media_assets(owner_user_id, updated_at DESC)
 WHERE deleted_at=0;

CREATE TABLE IF NOT EXISTS codelocal_media_variants (
  asset_id TEXT NOT NULL REFERENCES codelocal_media_assets(asset_id) ON DELETE CASCADE,
  variant TEXT NOT NULL CHECK (variant IN ('original','thumb','medium','large')),
  object_key TEXT NOT NULL,
  content_type TEXT NOT NULL,
  size BIGINT NOT NULL,
  width INTEGER NOT NULL,
  height INTEGER NOT NULL,
  sha256 TEXT NOT NULL,
  created_at BIGINT NOT NULL,
  PRIMARY KEY(asset_id, variant),
  UNIQUE(object_key),
  CHECK (size > 0),
  CHECK (width > 0),
  CHECK (height > 0),
  CHECK (BTRIM(object_key) <> ''),
  CHECK (BTRIM(content_type) <> ''),
  CHECK (BTRIM(sha256) <> '')
);

CREATE TABLE IF NOT EXISTS codelocal_media_understanding (
  asset_id TEXT PRIMARY KEY REFERENCES codelocal_media_assets(asset_id) ON DELETE CASCADE,
  summary TEXT NOT NULL DEFAULT '',
  alt_text TEXT NOT NULL DEFAULT '',
  extracted_text TEXT NOT NULL DEFAULT '',
  tags JSONB NOT NULL DEFAULT '[]'::jsonb,
  language TEXT NOT NULL DEFAULT '',
  provider TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  analyzed_at BIGINT NOT NULL DEFAULT 0,
  updated_at BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS codelocal_media_asset_refs (
  asset_id TEXT NOT NULL REFERENCES codelocal_media_assets(asset_id) ON DELETE CASCADE,
  owner_user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
  ref_kind TEXT NOT NULL,
  ref_id TEXT NOT NULL,
  slot TEXT NOT NULL,
  created_at BIGINT NOT NULL,
  PRIMARY KEY(asset_id, ref_kind, ref_id, slot),
  CHECK (BTRIM(ref_kind) <> ''),
  CHECK (BTRIM(ref_id) <> ''),
  CHECK (BTRIM(slot) <> '')
);
CREATE INDEX IF NOT EXISTS idx_codelocal_media_asset_refs_reference
 ON codelocal_media_asset_refs(owner_user_id, ref_kind, ref_id);
`

func mediaAssetSchemaMigrations() []schemaMigration {
	return []schemaMigration{{55, durableMediaAssetMigrationSQL}}
}
