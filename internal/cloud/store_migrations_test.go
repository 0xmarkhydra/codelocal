package cloud

import (
	"strings"
	"testing"
)

func TestKnowledgeV2MigrationTrainIsContiguousAndTransactional(t *testing.T) {
	migrations := knowledgeV2SchemaMigrations()
	if len(migrations) != 13 {
		t.Fatalf("knowledge v2 migration count=%d want 13", len(migrations))
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
		38: {"codelocal_knowledge_embeddings", "embedding vector", "model_version", "dimensions", "content_hash", "codelocal_knowledge_embedding_projection_state"},
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

func TestMigrationAdvisoryLockIdentityIsStableAndNonZero(t *testing.T) {
	if schemaMigrationAdvisoryLockID == 0 {
		t.Fatal("schema migration advisory lock id must be non-zero")
	}
	if nonTransactionalMigrationVersions[26] || nonTransactionalMigrationVersions[38] {
		t.Fatal("new migration train unexpectedly bypasses transactional runner")
	}
}

func TestSchemaMigrationStatusRequiresContiguousAppliedVersions(t *testing.T) {
	if got := LatestSchemaMigrationVersion(); got != 38 {
		t.Fatalf("latest schema version=%d want 38", got)
	}
	ready := schemaMigrationStatus(38, 38)
	if !ready.UpToDate || ready.TargetVersion != 38 || ready.AppliedCount != 38 || len(ready.ProjectBrainPlanHash) != 64 {
		t.Fatalf("unexpected ready schema status: %#v", ready)
	}
	for _, tc := range []struct {
		current int
		count   int
	}{
		{current: 37, count: 37},
		{current: 38, count: 37},
		{current: 39, count: 39},
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
	if migrations[0].version != 26 || migrations[len(migrations)-1].version != LatestSchemaMigrationVersion() {
		t.Fatalf("migration hash train boundaries drifted: %#v", migrations)
	}
}

func TestDatabaseSchemaHistoryRejectsForwardBinaryAndGaps(t *testing.T) {
	if err := validateDatabaseSchemaHistory(25, 25, 38); err != nil {
		t.Fatalf("valid older contiguous schema rejected: %v", err)
	}
	if err := validateDatabaseSchemaHistory(38, 38, 38); err != nil {
		t.Fatalf("target schema rejected: %v", err)
	}
	for _, tc := range []struct {
		name                   string
		current, count, target int
	}{
		{name: "newer database", current: 39, count: 39, target: 38},
		{name: "missing history row", current: 38, count: 37, target: 38},
		{name: "corrupt sparse history", current: 20, count: 19, target: 38},
		{name: "invalid negative", current: -1, count: 0, target: 38},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateDatabaseSchemaHistory(tc.current, tc.count, tc.target); err == nil {
				t.Fatalf("invalid database schema history accepted: current=%d count=%d target=%d", tc.current, tc.count, tc.target)
			}
		})
	}
}
