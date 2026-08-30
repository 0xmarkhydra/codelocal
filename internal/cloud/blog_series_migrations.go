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

func blogSeriesSchemaMigrations() []schemaMigration {
	return []schemaMigration{{56, blogSeriesSlugRedirectMigrationSQL}}
}
