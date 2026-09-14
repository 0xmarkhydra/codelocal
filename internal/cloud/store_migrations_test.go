package cloud

import (
	"strings"
	"testing"
)

func TestKnowledgeV2MigrationTrainIsContiguousAndTransactional(t *testing.T) {
	migrations := knowledgeV2SchemaMigrations()
	if len(migrations) != 15 {
		t.Fatalf("knowledge v2 migration count=%d want 15", len(migrations))
	}
	for index, migration := range migrations {
		want := 26 + index
		if migration.version != want {
			t.Fatalf("migration[%d].version=%d want %d", index, migration.version, want)
		}
		if strings.TrimSpace(migration.sql) == "" {
			t.Fatalf("migration %d has empty SQL", migration.version)
		}
		if nonTransactionalMigrationVersions[migration.version] {
			t.Fatalf("new Project Brain migration %d must remain transactional", migration.version)
		}
	}
}

func TestSchemaMigrationPlanValidationRejectsGapsAndEmptySQL(t *testing.T) {
	valid := []schemaMigration{{1, "SELECT 1"}, {2, "SELECT 2"}, {3, "SELECT 3"}}
	if err := validateSchemaMigrationPlan(valid); err != nil {
		t.Fatalf("valid migration plan rejected: %v", err)
	}
	for _, tc := range []struct {
		name string
		plan []schemaMigration
	}{
		{name: "empty", plan: nil},
		{name: "starts late", plan: []schemaMigration{{2, "SELECT 2"}}},
		{name: "gap", plan: []schemaMigration{{1, "SELECT 1"}, {3, "SELECT 3"}}},
		{name: "duplicate", plan: []schemaMigration{{1, "SELECT 1"}, {1, "SELECT again"}}},
		{name: "empty sql", plan: []schemaMigration{{1, "  "}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateSchemaMigrationPlan(tc.plan); err == nil {
				t.Fatalf("invalid migration plan accepted: %#v", tc.plan)
			}
		})
	}
}

func TestKnowledgeV2MigrationTrainIsAdditive(t *testing.T) {
	for _, migration := range knowledgeV2SchemaMigrations() {
		lower := strings.ToLower(migration.sql)
		for _, forbidden := range []string{"drop table", "drop column", "truncate table"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("migration %d contains destructive operation %q", migration.version, forbidden)
			}
		}
	}
}

func TestKnowledgeV2MigrationDependenciesAreExplicit(t *testing.T) {
	required := map[int][]string{
		26: {"codelocal_durable_outbox", "user_id", "dedupe_key"},
		27: {"codelocal_promotion_candidates", "user_id", "project_id"},
		28: {"codelocal_knowledge_objects", "codelocal_knowledge_revisions", "codelocal_knowledge_provenance"},
		29: {"promotion", "status"},
		30: {"knowledge_health", "project_id"},
		31: {"promotion", "source"},
		32: {"repository", "alias"},
		33: {"codelocal_knowledge_shadow_metrics", "project_id", "references codelocal_projects"},
		34: {"codelocal_project_learned_skill_contributions", "references codelocal_projects", "references codelocal_workspaces"},
		35: {"codelocal_collective_preferences", "codelocal_collective_user_patterns", "codelocal_collective_event_ledger"},
		36: {"codelocal_knowledge_graph_nodes", "codelocal_knowledge_graph_edges", "references codelocal_knowledge_objects", "references codelocal_knowledge_revisions"},
		37: {"codelocal_knowledge_graph_projection_state", "source_object_count", "source_revision_count", "projected_at", "references codelocal_projects"},
		38: {"codelocal_knowledge_embedding_projection_state", "model_version", "dimensions", "source_revision_count", "projected_revision_count", "references codelocal_projects"},
		39: {"codelocal_knowledge_embedding_shadow_metrics", "semantic_hits_total", "high_similarity_hits_total", "references codelocal_projects"},
		40: {"codelocal_knowledge_semantic_canary_metrics", "attempts_total", "applied_count", "deterministic_fallback_count", "references codelocal_projects"},
	}
	for _, migration := range knowledgeV2SchemaMigrations() {
		lower := strings.ToLower(migration.sql)
		for _, token := range required[migration.version] {
			if !strings.Contains(lower, strings.ToLower(token)) {
				t.Fatalf("migration %d missing dependency/contract token %q", migration.version, token)
			}
		}
	}
}

func TestProductMigrationTrainFollowsProjectBrainTrain(t *testing.T) {
	migrations := accountSchemaMigrations()
	if len(migrations) != 27 {
		t.Fatalf("unexpected account migration train: %#v", migrations)
	}
	for index, version := range []int{41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63, 64, 65, 66, 67} {
		if migrations[index].version != version {
			t.Fatalf("migration[%d].version=%d want %d", index, migrations[index].version, version)
		}
	}
	if !strings.Contains(strings.ToLower(migrations[0].sql), "password_changed_at") {
		t.Fatal("account migration 41 must add password_changed_at")
	}
	if !strings.Contains(strings.ToLower(migrations[1].sql), "security_version") {
		t.Fatal("account migration 42 must add security_version")
	}
	if !strings.Contains(strings.ToLower(migrations[2].sql), "public_key") {
		t.Fatal("account migration 43 must add device public_key")
	}
	if !strings.Contains(strings.ToLower(migrations[3].sql), "codelocal_dashboard_chat") {
		t.Fatal("dashboard migration 44 must create dashboard chat storage")
	}
	if !strings.Contains(strings.ToLower(migrations[4].sql), "image") {
		t.Fatal("dashboard migration 45 must preserve the image column")
	}
	if !strings.Contains(strings.ToLower(migrations[5].sql), "codelocal_runtime_config") {
		t.Fatal("runtime migration 46 must create config storage")
	}
	if !strings.Contains(strings.ToLower(migrations[6].sql), "codelocal_runtime_secrets") {
		t.Fatal("runtime migration 47 must create encrypted secret storage")
	}
	if !strings.Contains(strings.ToLower(migrations[7].sql), "skill_affinity") || !strings.Contains(strings.ToLower(migrations[7].sql), "skill_id") {
		t.Fatal("skill migration 48 must index tenant-private affinity evidence")
	}
	if !strings.Contains(strings.ToLower(migrations[8].sql), "skills jsonb") {
		t.Fatal("skill migration 49 must persist versioned chat skill metadata")
	}
	registry := strings.ToLower(migrations[9].sql)
	for _, token := range []string{"codelocal_skill_versions", "codelocal_skill_channels", "package_hash", "artifact_uri", "tenant_user_id"} {
		if !strings.Contains(registry, token) {
			t.Fatalf("skill migration 50 missing registry token %q", token)
		}
	}
	userState := strings.ToLower(migrations[10].sql)
	for _, token := range []string{"codelocal_skill_user_states", "pinned_version", "disabled", "prefer"} {
		if !strings.Contains(userState, token) {
			t.Fatalf("skill migration 51 missing user-state token %q", token)
		}
	}
	evaluations := strings.ToLower(migrations[11].sql)
	for _, token := range []string{"codelocal_skill_evaluations", "decision", "score", "checks"} {
		if !strings.Contains(evaluations, token) {
			t.Fatalf("skill migration 52 missing evaluation token %q", token)
		}
	}
	ratings := strings.ToLower(migrations[12].sql)
	for _, token := range []string{"codelocal_skill_ratings", "rating", "user_id", "skill_id"} {
		if !strings.Contains(ratings, token) {
			t.Fatalf("skill migration 53 missing rating token %q", token)
		}
	}
	blog := strings.ToLower(migrations[13].sql)
	for _, token := range []string{"codelocal_blog_posts", "codelocal_blog_series", "codelocal_blog_post_revisions", "codelocal_blog_slug_redirects", "cover_asset_id", "show_on_landing"} {
		if !strings.Contains(blog, token) {
			t.Fatalf("blog migration 54 missing platform token %q", token)
		}
	}
	media := strings.ToLower(migrations[14].sql)
	for _, token := range []string{"codelocal_media_assets", "codelocal_media_variants", "codelocal_media_understanding", "codelocal_media_asset_refs", "source_sha256", "preserve_original"} {
		if !strings.Contains(media, token) {
			t.Fatalf("media migration 55 missing platform token %q", token)
		}
	}
	seriesRedirects := strings.ToLower(migrations[15].sql)
	for _, token := range []string{"codelocal_blog_series_slug_redirects", "old_slug", "series_id"} {
		if !strings.Contains(seriesRedirects, token) {
			t.Fatalf("blog series migration 56 missing platform token %q", token)
		}
	}
	redirectGuards := strings.ToLower(migrations[16].sql)
	for _, token := range []string{"codelocal_guard_blog_post_redirect_slug", "codelocal_guard_blog_series_redirect_slug", "before insert or update of slug", "23505"} {
		if !strings.Contains(redirectGuards, token) {
			t.Fatalf("blog redirect namespace migration 57 missing token %q", token)
		}
	}
	chatThreads := strings.ToLower(migrations[17].sql)
	for _, token := range []string{"codelocal_dashboard_chat_thread", "thread_id", "on delete cascade", "where thread_id is null"} {
		if !strings.Contains(chatThreads, token) {
			t.Fatalf("dashboard chat thread migration 58 missing token %q", token)
		}
	}
	shares := strings.ToLower(migrations[18].sql)
	for _, token := range []string{"codelocal_screenshot_shares", "share_id", "owner_user_id", "asset_id", "deleted_at"} {
		if !strings.Contains(shares, token) {
			t.Fatalf("screenshot share migration 59 missing token %q", token)
		}
	}
	plugins := strings.ToLower(migrations[20].sql)
	for _, token := range []string{"codelocal_plugin_installations", "plugin_id", "manifest_hash", "installed_at", "references codelocal_users"} {
		if !strings.Contains(plugins, token) {
			t.Fatalf("plugin migration 61 missing token %q", token)
		}
	}
	connections := strings.ToLower(migrations[21].sql)
	for _, token := range []string{"codelocal_plugin_connections", "workspace_key", "credential_ref", "tool_count", "references codelocal_plugin_installations"} {
		if !strings.Contains(connections, token) {
			t.Fatalf("plugin migration 62 missing token %q", token)
		}
	}
	subagents := strings.ToLower(migrations[22].sql)
	for _, token := range []string{"subagent_id", "subagent_verified", "subagent_affinity"} {
		if !strings.Contains(subagents, token) {
			t.Fatalf("subagent routing migration 63 missing token %q", token)
		}
	}
	forum := strings.ToLower(migrations[24].sql)
	for _, token := range []string{"codelocal_forum_topics", "codelocal_forum_comments", "codelocal_forum_votes", "github_issue_url", "under_review"} {
		if !strings.Contains(forum, token) {
			t.Fatalf("forum migration 65 missing token %q", token)
		}
	}
	mcpConnections := strings.ToLower(migrations[25].sql)
	for _, token := range []string{"codelocal_mcp_connections", "target", "device_id", "workspace_id", "credential_ciphertext", "pending", "ready"} {
		if !strings.Contains(mcpConnections, token) {
			t.Fatalf("MCP connection migration 66 missing token %q", token)
		}
	}
}

func TestMigrationAdvisoryLockIdentityIsStableAndNonZero(t *testing.T) {
	if schemaMigrationAdvisoryLockID == 0 {
		t.Fatal("schema migration advisory lock id must be non-zero")
	}
	if nonTransactionalMigrationVersions[26] || nonTransactionalMigrationVersions[40] || nonTransactionalMigrationVersions[54] || nonTransactionalMigrationVersions[55] || nonTransactionalMigrationVersions[56] || nonTransactionalMigrationVersions[57] || nonTransactionalMigrationVersions[58] || nonTransactionalMigrationVersions[59] || nonTransactionalMigrationVersions[60] || nonTransactionalMigrationVersions[61] || nonTransactionalMigrationVersions[62] || nonTransactionalMigrationVersions[63] || nonTransactionalMigrationVersions[64] || nonTransactionalMigrationVersions[65] || nonTransactionalMigrationVersions[66] || nonTransactionalMigrationVersions[67] {
		t.Fatal("new migration train unexpectedly bypasses transactional runner")
	}
}

func TestSchemaMigrationStatusRequiresContiguousAppliedVersions(t *testing.T) {
	if got := LatestSchemaMigrationVersion(); got != 67 {
		t.Fatalf("latest schema version=%d want 67", got)
	}
	ready := schemaMigrationStatus(67, 67)
	if !ready.UpToDate || ready.TargetVersion != 67 || ready.AppliedCount != 67 || len(ready.ProjectBrainPlanHash) != 64 {
		t.Fatalf("unexpected ready schema status: %#v", ready)
	}
	for _, tc := range []struct {
		current int
		count   int
	}{
		{current: 40, count: 40},
		{current: 41, count: 41},
		{current: 42, count: 42},
		{current: 43, count: 43},
		{current: 44, count: 44},
		{current: 45, count: 45},
		{current: 46, count: 46},
		{current: 47, count: 47},
		{current: 48, count: 48},
		{current: 49, count: 49},
		{current: 50, count: 50},
		{current: 51, count: 51},
		{current: 52, count: 52},
		{current: 53, count: 53},
		{current: 54, count: 54},
		{current: 55, count: 55},
		{current: 56, count: 56},
		{current: 57, count: 57},
		{current: 58, count: 58},
		{current: 59, count: 59},
		{current: 60, count: 60},
		{current: 61, count: 61},
		{current: 62, count: 62},
		{current: 65, count: 65},
		{current: 59, count: 58},
	} {
		if status := schemaMigrationStatus(tc.current, tc.count); status.UpToDate {
			t.Fatalf("non-target/non-contiguous schema reported ready: %#v", status)
		}
	}
}

func TestProjectBrainMigrationPlanHashIsDeterministicAndCoversTrain(t *testing.T) {
	first := ProjectBrainMigrationPlanHash()
	second := ProjectBrainMigrationPlanHash()
	if first == "" || len(first) != 64 || first != second {
		t.Fatalf("unstable project brain migration hash: first=%q second=%q", first, second)
	}
	migrations := knowledgeV2SchemaMigrations()
	if migrations[0].version != 26 || migrations[len(migrations)-1].version != 40 {
		t.Fatalf("Project Brain migration hash train boundaries drifted: %#v", migrations)
	}
}

func TestDatabaseSchemaHistoryRejectsForwardBinaryAndGaps(t *testing.T) {
	if err := validateDatabaseSchemaHistory(25, 25, 45); err != nil {
		t.Fatalf("valid older contiguous schema rejected: %v", err)
	}
	if err := validateDatabaseSchemaHistory(45, 45, 45); err != nil {
		t.Fatalf("target schema rejected: %v", err)
	}
	for _, tc := range []struct {
		name                   string
		current, count, target int
	}{
		{name: "newer database", current: 46, count: 46, target: 45},
		{name: "missing history row", current: 45, count: 44, target: 45},
		{name: "corrupt sparse history", current: 20, count: 19, target: 45},
		{name: "invalid negative", current: -1, count: 0, target: 45},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateDatabaseSchemaHistory(tc.current, tc.count, tc.target); err == nil {
				t.Fatalf("invalid database schema history accepted: current=%d count=%d target=%d", tc.current, tc.count, tc.target)
			}
		})
	}
}
