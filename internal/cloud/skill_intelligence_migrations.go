package cloud

const skillAffinityIndexMigrationSQL = `
CREATE INDEX IF NOT EXISTS idx_codelocal_experiences_skill_affinity
 ON codelocal_experiences(user_id, skill_id, created_at DESC)
 WHERE skill_id IS NOT NULL;
`

const dashboardChatSkillsMigrationSQL = `
ALTER TABLE codelocal_dashboard_chat
 ADD COLUMN IF NOT EXISTS skills JSONB NOT NULL DEFAULT '[]'::jsonb;
`

// skillIntelligenceSchemaMigrations is intentionally forward-only. Existing
// schema versions are immutable because Cloud/Desktop binaries can overlap
// during rolling releases.
func skillIntelligenceSchemaMigrations() []schemaMigration {
	return []schemaMigration{
		{48, skillAffinityIndexMigrationSQL},
		{49, dashboardChatSkillsMigrationSQL},
	}
}
