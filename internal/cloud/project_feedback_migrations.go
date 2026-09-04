package cloud

const projectFeedbackMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_project_feedback (
  feedback_id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('widget','chat','manual')),
  kind TEXT NOT NULL DEFAULT 'other' CHECK (kind IN ('bug','feature','praise','other')),
  title TEXT NOT NULL,
  body TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'new' CHECK (status IN ('new','triaged','planned','resolved','dismissed')),
  dedupe_key TEXT NOT NULL DEFAULT '',
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  CHECK (BTRIM(feedback_id) <> ''),
  CHECK (BTRIM(title) <> ''),
  FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_project_feedback_project
 ON codelocal_project_feedback(user_id,project_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_codelocal_project_feedback_cluster
 ON codelocal_project_feedback(user_id,project_id,dedupe_key,created_at DESC)
 WHERE status NOT IN ('resolved','dismissed');
CREATE INDEX IF NOT EXISTS idx_codelocal_project_feedback_status
 ON codelocal_project_feedback(user_id,status,updated_at DESC);
`

func projectFeedbackSchemaMigrations() []schemaMigration {
	return []schemaMigration{{62, projectFeedbackMigrationSQL}}
}
