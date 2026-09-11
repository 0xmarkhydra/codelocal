package cloud

const subagentRoutingAffinityMigrationSQL = `
ALTER TABLE codelocal_experiences
 ADD COLUMN IF NOT EXISTS subagent_id TEXT,
 ADD COLUMN IF NOT EXISTS subagent_role TEXT,
 ADD COLUMN IF NOT EXISTS subagent_scope TEXT,
 ADD COLUMN IF NOT EXISTS subagent_verified BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX IF NOT EXISTS idx_codelocal_experiences_subagent_affinity
 ON codelocal_experiences(user_id, subagent_id, created_at DESC)
 WHERE subagent_id IS NOT NULL AND subagent_verified;
`

func subagentRoutingAffinitySchemaMigrations() []schemaMigration {
	return []schemaMigration{{63, subagentRoutingAffinityMigrationSQL}}
}
