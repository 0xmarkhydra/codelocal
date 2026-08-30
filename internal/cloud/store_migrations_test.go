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

func TestAccountSecurityMigrationFollowsProjectBrainTrain(t *testing.T) {
	migrations := accountSchemaMigrations()
	if len(migrations) != 14 {
		t.Fatalf("unexpected account migration train: %#v", migrations)
	}
	for index, version := range []int{41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54} {
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
	packages := strings.ToLower(migrations[13].sql)
	for _, token := range []string{"codelocal_skill_packages", "package_hash", "payload", "size_bytes"} {
		if !strings.Contains(packages, token) {
			t.Fatalf("skill migration 54 missing package fallback token %q", token)
		}
	}
}

func TestMigrationAdvisoryLockIdentityIsStableAndNonZero(t *testing.T) {
	if schemaMigrationAdvisoryLockID == 0 {
		t.Fatal("schema migration advisory lock id must be non-zero")
	}
	if nonTransactionalMigrationVersions[26] || nonTransactionalMigrationVersions[40] {
		t.Fatal("new migration train unexpectedly bypasses transactional runner")
	}
}

func TestSchemaMigrationStatusRequiresContiguousAppliedVersions(t *testing.T) {
	if got := LatestSchemaMigrationVersion(); got != 54 {
		t.Fatalf("latest schema version=%d want 54", got)
	}
	ready := schemaMigrationStatus(54, 54)
	if !ready.UpToDate || ready.TargetVersion != 54 || ready.AppliedCount != 54 || len(ready.ProjectBrainPlanHash) != 64 {
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
		{current: 54, count: 53},
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
