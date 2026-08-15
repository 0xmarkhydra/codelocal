package cloud

import (
	"strings"
	"testing"
)

func TestSingleMemoryKeySymbolFailsClosedUnlessExactlyOneKeyExists(t *testing.T) {
	for name, symbols := range map[string][]string{
		"none":     {"task:abc", "file:a.go"},
		"empty":    {"memory-key:   "},
		"multiple": {"memory-key:project database", "memory-key:project cache"},
	} {
		t.Run(name, func(t *testing.T) {
			if key, ok := singleMemoryKeySymbol(symbols); ok || key != "" {
				t.Fatalf("ambiguous memory key survived %s gate: key=%q ok=%t", name, key, ok)
			}
		})
	}
	key, ok := singleMemoryKeySymbol([]string{"file:a.go", "memory-key:Project   Database Strategy"})
	if !ok || key != "project database strategy" {
		t.Fatalf("single durable key normalization failed: key=%q ok=%t", key, ok)
	}
}

func TestReconcileRowBuildsCurrentRevisionAwarePromotionInput(t *testing.T) {
	row := explicitMemoryReconcileRow{
		MemoryID: "memory-a", Kind: "project_fact", Summary: "Database is PostgreSQL", Confidence: .95, Importance: .8,
		Symbols: []string{"memory-key:project database strategy"},
	}
	first, ok := explicitMemoryPromotionInputFromReconcileRow("user-a", "project-a", row)
	if !ok {
		t.Fatal("valid reconcile row rejected")
	}
	second, ok := explicitMemoryPromotionInputFromReconcileRow("user-a", "project-a", row)
	if !ok || first != second {
		t.Fatalf("unchanged durable memory did not produce idempotent input: first=%#v second=%#v", first, second)
	}
	row.Summary = "Database is CockroachDB"
	updated, ok := explicitMemoryPromotionInputFromReconcileRow("user-a", "project-a", row)
	if !ok {
		t.Fatal("updated durable memory rejected")
	}
	if updated.MemoryID != first.MemoryID || updated.StableKey != first.StableKey || updated.RevisionToken == first.RevisionToken {
		t.Fatalf("mutable memory revision semantics broken: first=%#v updated=%#v", first, updated)
	}
}

func TestReconcileRowKeepsUnsafeOrAmbiguousMemoryLegacyOnly(t *testing.T) {
	base := explicitMemoryReconcileRow{
		MemoryID: "memory-a", Kind: "decision", Summary: "Use PostgreSQL", Confidence: .95, Importance: .8,
		Symbols: []string{"memory-key:project database strategy"},
	}
	for name, mutate := range map[string]func(*explicitMemoryReconcileRow){
		"unsupported-kind": func(r *explicitMemoryReconcileRow) { r.Kind = "idea" },
		"low-confidence":   func(r *explicitMemoryReconcileRow) { r.Confidence = .5 },
		"low-importance":   func(r *explicitMemoryReconcileRow) { r.Importance = .4 },
		"no-key":           func(r *explicitMemoryReconcileRow) { r.Symbols = nil },
		"multiple-keys": func(r *explicitMemoryReconcileRow) {
			r.Symbols = []string{"memory-key:a", "memory-key:b"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			row := base
			mutate(&row)
			if _, ok := explicitMemoryPromotionInputFromReconcileRow("user-a", "project-a", row); ok {
				t.Fatalf("unsafe reconcile row survived %s gate: %#v", name, row)
			}
		})
	}
}

func TestReconcileQueryIsTenantProjectScopedBoundedAndProgressive(t *testing.T) {
	normalized := strings.ToLower(strings.Join(strings.Fields(reconcileExplicitProjectMemoriesSQL), " "))
	for _, required := range []string{
		"m.user_id=$1",
		"m.project_id=$2",
		"m.scope='project'",
		"m.source_type='conversation'",
		"m.kind in ('decision','project_fact','constraint','goal','milestone','problem')",
		"m.confidence >= 0.9",
		"m.importance >= 0.65",
		"ps.memory_id=m.id",
		"ps.source_type='explicit_user_memory'",
		"pc.summary=m.summary",
		"pc.confidence >= m.confidence",
		"pc.importance >= m.importance",
		"limit $3",
	} {
		if !strings.Contains(normalized, required) {
			t.Fatalf("reconcile query lost invariant %q: %s", required, normalized)
		}
	}
	if !strings.Contains(normalized, "not exists") {
		t.Fatal("reconciliation would repeatedly consume already-current rows instead of progressing through backlog")
	}
}

func TestReconcileLimitIsBounded(t *testing.T) {
	t.Setenv("CODELOCAL_KNOWLEDGE_RECONCILE_LIMIT", "99999")
	if got := explicitMemoryReconcileLimit(); got != 200 {
		t.Fatalf("reconcile max bound=%d want 200", got)
	}
	t.Setenv("CODELOCAL_KNOWLEDGE_RECONCILE_LIMIT", "0")
	if got := explicitMemoryReconcileLimit(); got != 50 {
		t.Fatalf("non-positive reconcile limit should use safe default, got=%d want 50", got)
	}
	t.Setenv("CODELOCAL_KNOWLEDGE_RECONCILE_LIMIT", "1")
	if got := explicitMemoryReconcileLimit(); got != 1 {
		t.Fatalf("small positive reconcile limit=%d want 1", got)
	}
}

func TestKnowledgeMaintenanceLifecycleReconcilesThenRechecksBeforePending(t *testing.T) {
	healthy := KnowledgeHealth{AutoPromotionEnabled: true, Status: knowledgeHealthHealthy}
	stages := knowledgeMaintenanceStages(healthy)
	want := []string{
		knowledgeMaintenanceReconcile,
		knowledgeMaintenanceRecheck,
		knowledgeMaintenancePending,
		knowledgeMaintenancePostPendingRecheck,
		knowledgeMaintenanceProjection,
	}
	if strings.Join(stages, ",") != strings.Join(want, ",") {
		t.Fatalf("unsafe maintenance ordering: got=%#v want=%#v", stages, want)
	}
	critical := KnowledgeHealth{AutoPromotionEnabled: false, Status: knowledgeHealthCritical}
	if stages := knowledgeMaintenanceStages(critical); len(stages) != 0 {
		t.Fatalf("critical health should skip reconciliation/promotion stages: %#v", stages)
	}
}
