package cloud

import (
	"strings"
	"testing"

	longmemory "github.com/0xmarkhydra/codelocal/internal/memory"
)

func explicitMemoryRecord(kind, summary string) longmemory.Record {
	return longmemory.Record{
		ID: "memory-a", UserID: "user-a", ProjectID: "project-a", Scope: longmemory.ScopeProject,
		Kind: kind, SourceType: "conversation", Summary: summary, Symbols: []string{"memory-key:project database strategy"},
		Confidence: .95, Importance: .8,
	}
}

func TestExplicitMemoryPromotionRequiresProjectScopeStableKeyAndSafeKind(t *testing.T) {
	for _, kind := range []string{"decision", "project_fact", "constraint", "goal", "milestone", "problem"} {
		record := explicitMemoryRecord(kind, "Durable project knowledge")
		input, ok := ExplicitMemoryPromotionInputForRecord("user-a", "project-a", record, "project database strategy")
		if !ok {
			t.Fatalf("safe explicit memory kind %q was not stageable", kind)
		}
		if input.SourceKey != "project database strategy" || input.StableKey != "project-database-strategy" || input.RevisionToken == "" {
			t.Fatalf("explicit memory key normalization lost source/canonical identity: %#v", input)
		}
	}

	unsafeKinds := []string{"preference", "user_fact", "idea", "person", "company"}
	for _, kind := range unsafeKinds {
		if _, ok := ExplicitMemoryPromotionInputForRecord("user-a", "project-a", explicitMemoryRecord(kind, "legacy-only"), "stable key"); ok {
			t.Fatalf("unsupported canonical kind %q must remain legacy-only", kind)
		}
	}

	record := explicitMemoryRecord("decision", "Use PostgreSQL")
	if _, ok := ExplicitMemoryPromotionInputForRecord("user-a", "project-a", record, ""); ok {
		t.Fatal("explicit mutable canonical memory without stable key must remain legacy-only")
	}
	record.Scope = longmemory.ScopeWorkspace
	record.ProjectID = ""
	if _, ok := ExplicitMemoryPromotionInputForRecord("user-a", "project-a", record, "project.database.strategy"); ok {
		t.Fatal("workspace-scoped memory must not silently become project canonical knowledge")
	}
}

func TestExplicitMemoryRevisionTokenChangesWhenMutableFactChanges(t *testing.T) {
	record := explicitMemoryRecord("project_fact", "Database is PostgreSQL")
	first, ok := ExplicitMemoryPromotionInputForRecord("user-a", "project-a", record, "project.database.strategy")
	if !ok {
		t.Fatal("valid explicit memory rejected")
	}
	secondRecord := record
	secondRecord.Summary = "Database is CockroachDB"
	second, ok := ExplicitMemoryPromotionInputForRecord("user-a", "project-a", secondRecord, "project.database.strategy")
	if !ok {
		t.Fatal("updated explicit memory rejected")
	}
	if first.MemoryID != second.MemoryID || first.StableKey != second.StableKey {
		t.Fatal("mutable explicit memory lost stable source/canonical identity")
	}
	if first.RevisionToken == second.RevisionToken {
		t.Fatal("changed explicit memory value reused durable outbox revision token")
	}
}

func TestExplicitMemorySourceValidationUsesLegacySourceKeyAndCanonicalStableKey(t *testing.T) {
	record := explicitMemorySourceRecord{
		Kind: "decision", Summary: "Use PostgreSQL", Confidence: .95, Importance: .8,
		Symbols: []string{"memory-key:project database strategy"},
	}
	input := ExplicitMemoryPromotionInput{
		UserID: "user-a", ProjectID: "project-a", MemoryID: "memory-a", SourceKey: "project database strategy", StableKey: "project-database-strategy",
		RevisionToken: explicitMemoryRevisionToken("decision", "project-database-strategy", "Use PostgreSQL", .95, .8),
	}
	if !explicitMemorySourceMatches(input, record) {
		t.Fatal("legacy source key and canonical stable key should validate independently")
	}
	input.RevisionToken = "stale-token"
	if explicitMemorySourceMatches(input, record) {
		t.Fatal("stale explicit-memory outbox event survived revision-token validation")
	}
}

func TestExplicitMemoryCandidateUsesUserSuppliedKindWithoutSemanticGuessing(t *testing.T) {
	record := explicitMemorySourceRecord{
		Kind: "decision", Summary: "Project Brain uses server-first architecture", Confidence: .98, Importance: .9,
		Symbols: []string{"memory-key:project architecture strategy"},
	}
	input := ExplicitMemoryPromotionInput{
		UserID: "user-a", ProjectID: "project-a", MemoryID: "memory-a", SourceKey: "project architecture strategy", StableKey: "project-architecture-strategy",
		RevisionToken: explicitMemoryRevisionToken("decision", "project-architecture-strategy", record.Summary, record.Confidence, record.Importance),
	}
	candidate, _, ok := explicitMemoryCandidate(input, record)
	if !ok {
		t.Fatal("valid explicit decision rejected")
	}
	if candidate.KnowledgeType != "decision" || candidate.Predicate != "has-decision" {
		t.Fatalf("free text was semantically reclassified instead of honoring explicit kind: %#v", candidate)
	}
	if candidate.Cardinality != promotionCardinalityScalar || candidate.StableKey != "project-architecture-strategy" {
		t.Fatalf("explicit stable identity was not preserved: %#v", candidate)
	}
}

func TestPromotionSourceMigrationGeneralizesEvidenceWithoutFakeExperience(t *testing.T) {
	normalized := strings.ToLower(strings.Join(strings.Fields(promotionSourcesMigrationSQL), " "))
	for _, required := range []string{
		"create table if not exists codelocal_knowledge_promotion_sources",
		"source_type in ('verified_experience','explicit_user_memory')",
		"foreign key(user_id,memory_id) references codelocal_memories(user_id,id)",
		"insert into codelocal_knowledge_promotion_sources",
		"alter table codelocal_knowledge_provenance add column if not exists source_id",
		"alter table codelocal_knowledge_provenance add column if not exists memory_id",
		"source_type='explicit_user_memory' and memory_id is not null and experience_id is null",
		"knowledge_type in ('architecture','decision','project_fact','constraint','goal','milestone','problem')",
	} {
		if !strings.Contains(normalized, required) {
			t.Fatalf("source-aware migration lost invariant %q", required)
		}
	}
	if strings.Contains(insertPromotionMemorySourceSQL, "'verified_experience'") || !strings.Contains(insertPromotionMemorySourceSQL, "'explicit_user_memory'") {
		t.Fatal("explicit memory source must not masquerade as a verified Experience")
	}
}

func TestPromotionSourceQueriesRemainTenantScoped(t *testing.T) {
	for name, query := range map[string]string{
		"sources":           promotionSourcesSelectSQL,
		"memory":            explicitMemorySourceSelectSQL,
		"explicit-exists":   promotionHasExplicitMemorySourceSQL,
		"experience-insert": insertPromotionExperienceSourceSQL,
		"memory-insert":     insertPromotionMemorySourceSQL,
	} {
		normalized := strings.ToLower(strings.Join(strings.Fields(query), " "))
		if !strings.Contains(normalized, "user_id") {
			t.Fatalf("%s lost tenant key: %s", name, normalized)
		}
	}
}
