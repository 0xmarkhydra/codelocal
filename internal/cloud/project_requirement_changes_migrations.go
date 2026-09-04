package cloud

const projectRequirementChangeMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_project_requirement_changes (
  change_id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('docs','chat','feedback','manual')),
  summary TEXT NOT NULL DEFAULT '',
  changed_refs_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  created_at BIGINT NOT NULL,
  CHECK (BTRIM(change_id) <> ''),
  CHECK (BTRIM(summary) <> ''),
  FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_project_requirement_changes_project
 ON codelocal_project_requirement_changes(user_id,project_id,created_at DESC);
`

func projectRequirementChangeSchemaMigrations() []schemaMigration {
	return []schemaMigration{{61, projectRequirementChangeMigrationSQL}}
}
