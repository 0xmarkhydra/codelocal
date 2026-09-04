package cloud

const projectOSMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_project_goals (
  goal_id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL,
  title TEXT NOT NULL,
  objective TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','PLAN_PROPOSED','WAITING_APPROVAL','APPROVED','EXECUTING','VERIFYING','DONE','BLOCKED','CANCELLED')),
  active_plan_id TEXT,
  created_by_actor TEXT NOT NULL DEFAULT 'user',
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  revision BIGINT NOT NULL DEFAULT 1,
  CHECK (BTRIM(goal_id) <> ''),
  CHECK (BTRIM(title) <> ''),
  FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_project_goals_user_project
 ON codelocal_project_goals(user_id,project_id,updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_codelocal_project_goals_status
 ON codelocal_project_goals(user_id,status,updated_at DESC);

CREATE TABLE IF NOT EXISTS codelocal_project_plans (
  plan_id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL,
  goal_id TEXT NOT NULL REFERENCES codelocal_project_goals(goal_id) ON DELETE CASCADE,
  summary TEXT NOT NULL DEFAULT '',
  flow_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  assumptions_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  acceptance_criteria_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  status TEXT NOT NULL DEFAULT 'PROPOSED' CHECK (status IN ('PROPOSED','WAITING_APPROVAL','APPROVED','REJECTED','STALE','CANCELLED')),
  approved_by TEXT,
  approved_at BIGINT NOT NULL DEFAULT 0,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  revision BIGINT NOT NULL DEFAULT 1,
  CHECK (BTRIM(plan_id) <> ''),
  FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_project_plans_goal
 ON codelocal_project_plans(user_id,goal_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_codelocal_project_plans_project
 ON codelocal_project_plans(user_id,project_id,updated_at DESC);

CREATE TABLE IF NOT EXISTS codelocal_project_tasks (
  task_id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL,
  goal_id TEXT NOT NULL REFERENCES codelocal_project_goals(goal_id) ON DELETE CASCADE,
  plan_id TEXT NOT NULL REFERENCES codelocal_project_plans(plan_id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'PLANNED' CHECK (status IN ('PLANNED','READY','ASSIGNED','RUNNING','REVIEWING','TESTING','DONE','BLOCKED','WAITING_HUMAN','WAITING_EXTERNAL','RETRYING','STALE','FAILED','CANCELLED')),
  task_kind TEXT NOT NULL DEFAULT 'coding' CHECK (task_kind IN ('coding','review','testing','docs','research','ops')),
  required_capabilities_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  assigned_agent_role TEXT NOT NULL DEFAULT '',
  execution_task_id TEXT NOT NULL DEFAULT '',
  priority INTEGER NOT NULL DEFAULT 0,
  retry_count INTEGER NOT NULL DEFAULT 0,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  revision BIGINT NOT NULL DEFAULT 1,
  CHECK (BTRIM(task_id) <> ''),
  CHECK (BTRIM(title) <> ''),
  FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_project_tasks_goal
 ON codelocal_project_tasks(user_id,goal_id,status);
CREATE INDEX IF NOT EXISTS idx_codelocal_project_tasks_plan
 ON codelocal_project_tasks(user_id,plan_id,status);
CREATE INDEX IF NOT EXISTS idx_codelocal_project_tasks_project_status
 ON codelocal_project_tasks(user_id,project_id,status,updated_at DESC);

CREATE TABLE IF NOT EXISTS codelocal_project_task_dependencies (
  user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL,
  task_id TEXT NOT NULL REFERENCES codelocal_project_tasks(task_id) ON DELETE CASCADE,
  depends_on_task_id TEXT NOT NULL REFERENCES codelocal_project_tasks(task_id) ON DELETE CASCADE,
  created_at BIGINT NOT NULL,
  PRIMARY KEY(user_id,task_id,depends_on_task_id),
  CHECK (task_id <> depends_on_task_id),
  FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_project_task_deps_blocked
 ON codelocal_project_task_dependencies(user_id,depends_on_task_id);

CREATE TABLE IF NOT EXISTS codelocal_project_task_events (
  event_id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL,
  goal_id TEXT REFERENCES codelocal_project_goals(goal_id) ON DELETE CASCADE,
  task_id TEXT REFERENCES codelocal_project_tasks(task_id) ON DELETE CASCADE,
  event_type TEXT NOT NULL,
  actor_type TEXT NOT NULL DEFAULT 'system' CHECK (actor_type IN ('user','chief','agent','tester','reviewer','system')),
  actor_id TEXT NOT NULL DEFAULT '',
  safe_payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at BIGINT NOT NULL,
  CHECK (BTRIM(event_id) <> ''),
  CHECK (BTRIM(event_type) <> ''),
  FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_project_task_events_task
 ON codelocal_project_task_events(user_id,task_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_codelocal_project_task_events_goal
 ON codelocal_project_task_events(user_id,goal_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_codelocal_project_task_events_project
 ON codelocal_project_task_events(user_id,project_id,created_at DESC);

CREATE TABLE IF NOT EXISTS codelocal_project_tester_verdicts (
  verdict_id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL,
  task_id TEXT NOT NULL REFERENCES codelocal_project_tasks(task_id) ON DELETE CASCADE,
  verdict TEXT NOT NULL CHECK (verdict IN ('DONE','NOT_DONE')),
  evidence_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_by TEXT NOT NULL DEFAULT 'tester',
  created_at BIGINT NOT NULL,
  CHECK (BTRIM(verdict_id) <> ''),
  FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_project_tester_verdicts_task
 ON codelocal_project_tester_verdicts(user_id,task_id,created_at DESC);
`

func projectOSSchemaMigrations() []schemaMigration {
	return []schemaMigration{{60, projectOSMigrationSQL}}
}
