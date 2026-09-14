package cloud

const forumMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_forum_topics (
  topic_id TEXT PRIMARY KEY,
  author_user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('question','bug','idea')),
  title TEXT NOT NULL,
  body TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','under_review','planned','in_progress','resolved','closed')),
  severity TEXT NOT NULL DEFAULT '' CHECK (severity IN ('','low','medium','high','critical')),
  version TEXT NOT NULL DEFAULT '',
  environment TEXT NOT NULL DEFAULT '',
  reproduction_steps TEXT NOT NULL DEFAULT '',
  expected_behavior TEXT NOT NULL DEFAULT '',
  actual_behavior TEXT NOT NULL DEFAULT '',
  tags JSONB NOT NULL DEFAULT '[]'::jsonb,
  github_issue_url TEXT NOT NULL DEFAULT '',
  github_issue_number BIGINT NOT NULL DEFAULT 0,
  github_pr_url TEXT NOT NULL DEFAULT '',
  resolution_note TEXT NOT NULL DEFAULT '',
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  resolved_at BIGINT NOT NULL DEFAULT 0,
  deleted_at BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_codelocal_forum_topics_status_updated
 ON codelocal_forum_topics(status, updated_at DESC) WHERE deleted_at=0;
CREATE INDEX IF NOT EXISTS idx_codelocal_forum_topics_kind_updated
 ON codelocal_forum_topics(kind, updated_at DESC) WHERE deleted_at=0;
CREATE INDEX IF NOT EXISTS idx_codelocal_forum_topics_author
 ON codelocal_forum_topics(author_user_id, updated_at DESC) WHERE deleted_at=0;

CREATE TABLE IF NOT EXISTS codelocal_forum_comments (
  comment_id TEXT PRIMARY KEY,
  topic_id TEXT NOT NULL REFERENCES codelocal_forum_topics(topic_id) ON DELETE CASCADE,
  author_user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
  body TEXT NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  deleted_at BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_codelocal_forum_comments_topic
 ON codelocal_forum_comments(topic_id, created_at ASC) WHERE deleted_at=0;

CREATE TABLE IF NOT EXISTS codelocal_forum_votes (
  topic_id TEXT NOT NULL REFERENCES codelocal_forum_topics(topic_id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
  created_at BIGINT NOT NULL,
  PRIMARY KEY(topic_id,user_id)
);
CREATE INDEX IF NOT EXISTS idx_codelocal_forum_votes_user
 ON codelocal_forum_votes(user_id, created_at DESC);
`

func forumSchemaMigrations() []schemaMigration {
	return []schemaMigration{{65, forumMigrationSQL}}
}

const forumMediaMigrationSQL = `
ALTER TABLE codelocal_forum_topics
 ADD COLUMN IF NOT EXISTS asset_ids JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE codelocal_forum_comments
 ADD COLUMN IF NOT EXISTS asset_ids JSONB NOT NULL DEFAULT '[]'::jsonb;
`

func forumMediaSchemaMigrations() []schemaMigration {
	return []schemaMigration{{67, forumMediaMigrationSQL}}
}
