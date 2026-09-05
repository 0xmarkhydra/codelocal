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

const pluginConnectionsMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_plugin_connections (
  user_id TEXT NOT NULL,
  plugin_id TEXT NOT NULL,
  device_id TEXT NOT NULL,
  workspace_key TEXT NOT NULL,
  server_name TEXT NOT NULL,
  endpoint TEXT NOT NULL,
  credential_ref TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL CHECK (state IN ('configured', 'ready', 'error')),
  tool_count INTEGER NOT NULL DEFAULT 0 CHECK (tool_count >= 0),
  last_error TEXT NOT NULL DEFAULT '',
  connected_at BIGINT NOT NULL DEFAULT 0,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (user_id, plugin_id, device_id),
  FOREIGN KEY (user_id, plugin_id)
    REFERENCES codelocal_plugin_installations(user_id, plugin_id) ON DELETE CASCADE,
  CHECK (BTRIM(device_id) <> ''),
  CHECK (BTRIM(workspace_key) <> ''),
  CHECK (BTRIM(server_name) <> ''),
  CHECK (BTRIM(endpoint) <> '')
);
CREATE INDEX IF NOT EXISTS idx_codelocal_plugin_connections_user_plugin
 ON codelocal_plugin_connections(user_id, plugin_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_codelocal_plugin_connections_device
 ON codelocal_plugin_connections(user_id, device_id, updated_at DESC);
`

func pluginSchemaMigrations() []schemaMigration {
	return []schemaMigration{
		{61, pluginInstallationsMigrationSQL},
		{62, pluginConnectionsMigrationSQL},
	}
}
