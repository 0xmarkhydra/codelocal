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

const skillRegistryMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_skill_versions (
  record_id TEXT PRIMARY KEY,
  skill_id TEXT NOT NULL,
  version TEXT NOT NULL,
  tenant_user_id TEXT REFERENCES codelocal_users(id) ON DELETE CASCADE,
  creator_user_id TEXT REFERENCES codelocal_users(id) ON DELETE SET NULL,
  scope TEXT NOT NULL CHECK (scope IN ('system', 'personal', 'community')),
  kind TEXT NOT NULL CHECK (kind IN ('knowledge', 'workflow', 'runtime', 'hybrid')),
  publisher TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('active', 'candidate', 'evaluating', 'canary', 'promoted', 'rejected', 'rolled_back', 'deprecated', 'blocked')),
  verified BOOLEAN NOT NULL DEFAULT FALSE,
  manifest JSONB NOT NULL,
  package_hash TEXT NOT NULL,
  artifact_hash TEXT NOT NULL,
  artifact_uri TEXT NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  promoted_at BIGINT NOT NULL DEFAULT 0,
  CHECK (
    (scope = 'personal' AND tenant_user_id IS NOT NULL)
    OR (scope IN ('system', 'community') AND tenant_user_id IS NULL)
  )
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_codelocal_skill_versions_global_identity
 ON codelocal_skill_versions(skill_id, version)
 WHERE tenant_user_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_codelocal_skill_versions_personal_identity
 ON codelocal_skill_versions(tenant_user_id, skill_id, version)
 WHERE tenant_user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_codelocal_skill_versions_scope_state
 ON codelocal_skill_versions(scope, state, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_codelocal_skill_versions_creator
 ON codelocal_skill_versions(creator_user_id, updated_at DESC)
 WHERE creator_user_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS codelocal_skill_channels (
  channel_id TEXT PRIMARY KEY,
  skill_id TEXT NOT NULL,
  tenant_user_id TEXT REFERENCES codelocal_users(id) ON DELETE CASCADE,
  channel TEXT NOT NULL CHECK (channel IN ('stable', 'canary')),
  version TEXT NOT NULL,
  updated_at BIGINT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_codelocal_skill_channels_global
 ON codelocal_skill_channels(skill_id, channel)
 WHERE tenant_user_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_codelocal_skill_channels_personal
 ON codelocal_skill_channels(tenant_user_id, skill_id, channel)
 WHERE tenant_user_id IS NOT NULL;
`

const skillUserStateMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_skill_user_states (
  user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
  skill_id TEXT NOT NULL,
  mode TEXT NOT NULL DEFAULT 'auto' CHECK (mode IN ('auto', 'prefer', 'disabled')),
  pinned_version TEXT,
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (user_id, skill_id)
);
CREATE INDEX IF NOT EXISTS idx_codelocal_skill_user_states_mode
 ON codelocal_skill_user_states(user_id, mode, updated_at DESC);
`

const skillEvaluationMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_skill_evaluations (
  evaluation_id TEXT PRIMARY KEY,
  skill_id TEXT NOT NULL,
  version TEXT NOT NULL,
  evaluator_user_id TEXT REFERENCES codelocal_users(id) ON DELETE SET NULL,
  decision TEXT NOT NULL CHECK (decision IN ('passed', 'failed')),
  score DOUBLE PRECISION NOT NULL CHECK (score >= 0 AND score <= 1),
  checks JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_codelocal_skill_evaluations_version
 ON codelocal_skill_evaluations(skill_id, version, created_at DESC);
`

const skillRatingMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_skill_ratings (
  user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
  skill_id TEXT NOT NULL,
  rating SMALLINT NOT NULL CHECK (rating >= 1 AND rating <= 5),
  updated_at BIGINT NOT NULL,
  PRIMARY KEY (user_id, skill_id)
);
CREATE INDEX IF NOT EXISTS idx_codelocal_skill_ratings_skill
 ON codelocal_skill_ratings(skill_id, updated_at DESC);
`

const skillPackageFallbackMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_skill_packages (
  package_hash TEXT PRIMARY KEY,
  payload BYTEA NOT NULL,
  size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
  created_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_codelocal_skill_packages_created
 ON codelocal_skill_packages(created_at DESC);
`

// skillIntelligenceSchemaMigrations is intentionally forward-only. Existing
// schema versions are immutable because Cloud/Desktop binaries can overlap
// during rolling releases.
func skillIntelligenceSchemaMigrations() []schemaMigration {
	return []schemaMigration{
		{48, skillAffinityIndexMigrationSQL},
		{49, dashboardChatSkillsMigrationSQL},
		{50, skillRegistryMigrationSQL},
		{51, skillUserStateMigrationSQL},
		{52, skillEvaluationMigrationSQL},
		{53, skillRatingMigrationSQL},
		{54, skillPackageFallbackMigrationSQL},
	}
}
