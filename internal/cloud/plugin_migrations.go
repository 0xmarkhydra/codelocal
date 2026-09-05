package cloud

const pluginInstallationsMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_plugin_installations (
  user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
  plugin_id TEXT NOT NULL,
  version TEXT NOT NULL,
  manifest_hash TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'installed' CHECK (state IN ('installed', 'disabled')),
  installed_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (user_id, plugin_id),
  CHECK (BTRIM(plugin_id) <> ''),
  CHECK (BTRIM(version) <> ''),
  CHECK (BTRIM(manifest_hash) <> '')
);
CREATE INDEX IF NOT EXISTS idx_codelocal_plugin_installations_user_state
 ON codelocal_plugin_installations(user_id, state, updated_at DESC);
`

func pluginSchemaMigrations() []schemaMigration {
	return []schemaMigration{{61, pluginInstallationsMigrationSQL}}
}
