package cloud

const blogPlatformMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_blog_series (
  series_id TEXT PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  author_user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  cover_asset_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','complete','archived')),
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  deleted_at BIGINT NOT NULL DEFAULT 0,
  CHECK (BTRIM(slug) <> ''),
  CHECK (BTRIM(title) <> '')
);
CREATE INDEX IF NOT EXISTS idx_codelocal_blog_series_author
 ON codelocal_blog_series(author_user_id, updated_at DESC)
 WHERE deleted_at=0;

CREATE TABLE IF NOT EXISTS codelocal_blog_posts (
  post_id TEXT PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  author_user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  excerpt TEXT NOT NULL DEFAULT '',
  content JSONB NOT NULL DEFAULT '[]'::jsonb,
  cover_asset_id TEXT NOT NULL DEFAULT '',
  category TEXT NOT NULL DEFAULT '',
  tags JSONB NOT NULL DEFAULT '[]'::jsonb,
  series_id TEXT REFERENCES codelocal_blog_series(series_id) ON DELETE SET NULL,
  series_part INTEGER,
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','scheduled','published','archived')),
  visibility TEXT NOT NULL DEFAULT 'public' CHECK (visibility IN ('public','unlisted','private')),
  moderation_status TEXT NOT NULL DEFAULT 'clean' CHECK (moderation_status IN ('clean','pending','hidden')),
  featured BOOLEAN NOT NULL DEFAULT FALSE,
  show_on_landing BOOLEAN NOT NULL DEFAULT FALSE,
  published_at BIGINT NOT NULL DEFAULT 0,
  scheduled_at BIGINT NOT NULL DEFAULT 0,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  deleted_at BIGINT NOT NULL DEFAULT 0,
  CHECK (BTRIM(slug) <> ''),
  CHECK (BTRIM(title) <> ''),
  CHECK (series_part IS NULL OR series_part > 0),
  CHECK ((series_id IS NULL AND series_part IS NULL) OR (series_id IS NOT NULL AND series_part IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_codelocal_blog_posts_author
 ON codelocal_blog_posts(author_user_id, updated_at DESC)
 WHERE deleted_at=0;
CREATE INDEX IF NOT EXISTS idx_codelocal_blog_posts_public
 ON codelocal_blog_posts(published_at DESC, updated_at DESC)
 WHERE deleted_at=0 AND status='published' AND visibility='public' AND moderation_status='clean';
CREATE INDEX IF NOT EXISTS idx_codelocal_blog_posts_series
 ON codelocal_blog_posts(series_id, series_part)
 WHERE deleted_at=0 AND series_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_codelocal_blog_posts_series_part
 ON codelocal_blog_posts(series_id, series_part)
 WHERE deleted_at=0 AND series_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS codelocal_blog_post_revisions (
  revision_id TEXT PRIMARY KEY,
  post_id TEXT NOT NULL REFERENCES codelocal_blog_posts(post_id) ON DELETE CASCADE,
  editor_user_id TEXT REFERENCES codelocal_users(id) ON DELETE SET NULL,
  snapshot JSONB NOT NULL,
  created_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_codelocal_blog_post_revisions_post
 ON codelocal_blog_post_revisions(post_id, created_at DESC);

CREATE TABLE IF NOT EXISTS codelocal_blog_slug_redirects (
  old_slug TEXT PRIMARY KEY,
  post_id TEXT NOT NULL REFERENCES codelocal_blog_posts(post_id) ON DELETE CASCADE,
  created_at BIGINT NOT NULL,
  CHECK (BTRIM(old_slug) <> '')
);
CREATE INDEX IF NOT EXISTS idx_codelocal_blog_slug_redirects_post
 ON codelocal_blog_slug_redirects(post_id, created_at DESC);
`

func blogSchemaMigrations() []schemaMigration {
	return []schemaMigration{{54, blogPlatformMigrationSQL}}
}
