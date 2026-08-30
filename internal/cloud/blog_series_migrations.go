package cloud

const blogSeriesSlugRedirectMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_blog_series_slug_redirects (
  old_slug TEXT PRIMARY KEY,
  series_id TEXT NOT NULL REFERENCES codelocal_blog_series(series_id) ON DELETE CASCADE,
  created_at BIGINT NOT NULL,
  CHECK (BTRIM(old_slug) <> '')
);
CREATE INDEX IF NOT EXISTS idx_codelocal_blog_series_slug_redirects_series
 ON codelocal_blog_series_slug_redirects(series_id, created_at DESC);
`

const blogRedirectNamespaceMigrationSQL = `
CREATE OR REPLACE FUNCTION codelocal_guard_blog_post_redirect_slug()
RETURNS TRIGGER AS $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM codelocal_blog_slug_redirects r
    WHERE r.old_slug = NEW.slug AND r.post_id <> NEW.post_id
  ) THEN
    RAISE EXCEPTION 'blog slug is reserved by redirect: %', NEW.slug USING ERRCODE = '23505';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_codelocal_guard_blog_post_redirect_slug ON codelocal_blog_posts;
CREATE TRIGGER trg_codelocal_guard_blog_post_redirect_slug
BEFORE INSERT OR UPDATE OF slug ON codelocal_blog_posts
FOR EACH ROW EXECUTE FUNCTION codelocal_guard_blog_post_redirect_slug();

CREATE OR REPLACE FUNCTION codelocal_guard_blog_series_redirect_slug()
RETURNS TRIGGER AS $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM codelocal_blog_series_slug_redirects r
    WHERE r.old_slug = NEW.slug AND r.series_id <> NEW.series_id
  ) THEN
    RAISE EXCEPTION 'blog series slug is reserved by redirect: %', NEW.slug USING ERRCODE = '23505';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_codelocal_guard_blog_series_redirect_slug ON codelocal_blog_series;
CREATE TRIGGER trg_codelocal_guard_blog_series_redirect_slug
BEFORE INSERT OR UPDATE OF slug ON codelocal_blog_series
FOR EACH ROW EXECUTE FUNCTION codelocal_guard_blog_series_redirect_slug();
`

func blogSeriesSchemaMigrations() []schemaMigration {
	return []schemaMigration{
		{56, blogSeriesSlugRedirectMigrationSQL},
		{57, blogRedirectNamespaceMigrationSQL},
	}
}
