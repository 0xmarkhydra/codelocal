package cloud

import (
	"strings"
	"testing"
)

func TestCanonicalGraphProjectionBuildsProjectRepoRevisionAndSupersessionEdges(t *testing.T) {
	rows := []canonicalGraphProjectionRow{
		{
			KnowledgeID: "knowledge-a", RepositoryID: "repo-a", KnowledgeType: "decision", StableKey: "database-writes",
			Status: "active", Confidence: .98, Importance: .9, ObjectUpdatedAt: 300, ActiveRevision: "revision-2",
			RevisionID: "revision-1", RevisionNumber: 1, Summary: "Use direct writes.", RevConfidence: .8, RevImportance: .7, RevisionCreated: 100, RevisionValidTo: 200,
		},
		{
			KnowledgeID: "knowledge-a", RepositoryID: "repo-a", KnowledgeType: "decision", StableKey: "database-writes",
			Status: "active", Confidence: .98, Importance: .9, ObjectUpdatedAt: 300, ActiveRevision: "revision-2",
			RevisionID: "revision-2", RevisionNumber: 2, Summary: "Use transactional outbox for durable writes.", RevConfidence: .99, RevImportance: .95, RevisionCreated: 200,
		},
	}
	nodes, edges, stats := buildCanonicalGraphProjection("project-a", rows)
	if stats.KnowledgeCount != 1 || stats.RevisionCount != 2 || stats.NodeCount != 3 || stats.EdgeCount != 5 {
		t.Fatalf("unexpected projection stats: %#v nodes=%#v edges=%#v", stats, nodes, edges)
	}
	nodeByID := map[string]canonicalGraphNode{}
	for _, node := range nodes {
		nodeByID[node.NodeID] = node
	}
	if nodeByID["canonical-knowledge:knowledge-a"].Label != "decision: database-writes" {
		t.Fatalf("canonical knowledge label missing: %#v", nodeByID)
	}
	if nodeByID["canonical-revision:revision-1"].Status != "historical" || nodeByID["canonical-revision:revision-2"].Status != "active" {
		t.Fatalf("revision lifecycle missing: %#v", nodeByID)
	}
	edgeRelations := map[string]string{}
	for _, edge := range edges {
		edgeRelations[edge.EdgeID] = edge.Relation
	}
	for edgeID, relation := range map[string]string{
		"project-canonical:knowledge-a":     "HAS_CANONICAL_KNOWLEDGE",
		"repo-canonical:repo-a:knowledge-a": "SCOPES_KNOWLEDGE",
		"knowledge-revision:revision-1":     "HAS_REVISION",
		"knowledge-revision:revision-2":     "HAS_REVISION",
		"revision-superseded:revision-2":    "SUPERSEDED_BY",
	} {
		if edgeRelations[edgeID] != relation {
			t.Fatalf("edge %q relation=%q want %q: %#v", edgeID, edgeRelations[edgeID], relation, edges)
		}
	}
}

func TestCanonicalGraphProjectionRedactsSecretLikeHistoricalSummaryAndStableKey(t *testing.T) {
	rows := []canonicalGraphProjectionRow{{
		KnowledgeID: "knowledge-secret", KnowledgeType: "fact", StableKey: "api_key=super-secret-token",
		Status: "active", Confidence: .9, Importance: .8, ObjectUpdatedAt: 200, ActiveRevision: "revision-secret",
		RevisionID: "revision-secret", RevisionNumber: 1, Summary: "Authorization: Bearer super-secret-token", RevConfidence: .9, RevImportance: .8, RevisionCreated: 100,
	}}
	nodes, _, _ := buildCanonicalGraphProjection("project-a", rows)
	for _, node := range nodes {
		text := strings.ToLower(node.Label + " " + node.Summary)
		if strings.Contains(text, "super-secret-token") || strings.Contains(text, "api_key=") || strings.Contains(text, "authorization: bearer") {
			t.Fatalf("canonical graph projection leaked secret-like content: %#v", node)
		}
	}
}

func TestCanonicalGraphMigrationIsDerivedTenantScopedAndRebuildable(t *testing.T) {
	lower := strings.ToLower(canonicalKnowledgeGraphMigrationSQL)
	for _, required := range []string{
		"codelocal_knowledge_graph_nodes",
		"codelocal_knowledge_graph_edges",
		"primary key(user_id,project_id,node_id)",
		"primary key(user_id,project_id,edge_id)",
		"references codelocal_projects(user_id,project_id)",
		"references codelocal_knowledge_objects(user_id,knowledge_id)",
		"references codelocal_knowledge_revisions(user_id,revision_id)",
	} {
		if !strings.Contains(lower, required) {
			t.Fatalf("canonical graph migration missing %q", required)
		}
	}
	for _, forbidden := range []string{"drop table", "truncate table"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("canonical graph migration unexpectedly destructive: %q", forbidden)
		}
	}
}

func TestCanonicalGraphProjectionSelectsOnlyPrivateCurrentCanonicalObjects(t *testing.T) {
	lower := strings.ToLower(canonicalGraphProjectionSelectSQL)
	for _, required := range []string{
		"o.user_id=$1",
		"o.project_id=$2",
		"o.privacy_classification='private_project'",
		"o.status in ('active','stale','conflicted')",
		"limit $3",
	} {
		if !strings.Contains(lower, required) {
			t.Fatalf("canonical graph projection query missing %q: %s", required, lower)
		}
	}
}

func TestCanonicalGraphDashboardQueriesRemainTenantScoped(t *testing.T) {
	for name, query := range map[string]string{
		"nodes": canonicalGraphDashboardNodesSQL,
		"edges": canonicalGraphDashboardEdgesSQL,
	} {
		lower := strings.ToLower(query)
		if !strings.Contains(lower, "where user_id=$1") || !strings.Contains(lower, "limit $2") {
			t.Fatalf("dashboard %s query lost tenant/boundary scope: %s", name, lower)
		}
	}
}

func TestCanonicalGraphFreshnessClassifiesEmptyMissingStaleAndCurrent(t *testing.T) {
	now := int64(10_000)
	cases := []struct {
		name  string
		value CanonicalGraphFreshness
		want  string
		lag   int64
	}{
		{name: "empty", value: CanonicalGraphFreshness{}, want: "empty"},
		{name: "source removed but old graph remains", value: CanonicalGraphFreshness{ProjectedObjectCount: 1, ProjectedRevisionCount: 1, ProjectedAt: 8_000, NodeCount: 2, EdgeCount: 2}, want: "stale", lag: 2_000},
		{name: "missing", value: CanonicalGraphFreshness{SourceObjectCount: 1, SourceRevisionCount: 1, SourceUpdatedAt: 8_000, ProjectedObjectCount: -1, ProjectedRevisionCount: -1}, want: "missing", lag: 2_000},
		{name: "stale count", value: CanonicalGraphFreshness{SourceObjectCount: 2, SourceRevisionCount: 3, SourceUpdatedAt: 9_000, ProjectedObjectCount: 1, ProjectedRevisionCount: 2, ProjectedSourceUpdatedAt: 7_000, ProjectedAt: 8_000}, want: "stale", lag: 2_000},
		{name: "stale timestamp", value: CanonicalGraphFreshness{SourceObjectCount: 2, SourceRevisionCount: 3, SourceUpdatedAt: 9_000, ProjectedObjectCount: 2, ProjectedRevisionCount: 3, ProjectedSourceUpdatedAt: 8_500, ProjectedAt: 9_000}, want: "stale", lag: 500},
		{name: "current", value: CanonicalGraphFreshness{SourceObjectCount: 2, SourceRevisionCount: 3, SourceUpdatedAt: 9_000, ProjectedObjectCount: 2, ProjectedRevisionCount: 3, ProjectedSourceUpdatedAt: 9_000, ProjectedAt: 9_500}, want: "current"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyCanonicalGraphFreshness(tc.value, now)
			if got.Status != tc.want || got.LagMS != tc.lag {
				t.Fatalf("freshness=%#v want status=%q lag=%d", got, tc.want, tc.lag)
			}
		})
	}
}

func TestCanonicalGraphFreshnessSummaryDoesNotDegradeForEmptyProjects(t *testing.T) {
	summary := summarizeCanonicalGraphFreshness([]CanonicalGraphFreshness{
		{Status: "current"}, {Status: "empty"}, {Status: "stale", LagMS: 2000}, {Status: "missing", LagMS: 5000},
	})
	if summary.Status != "degraded" || summary.ProjectCount != 4 || summary.CurrentProjects != 1 || summary.EmptyProjects != 1 || summary.StaleProjects != 1 || summary.MissingProjects != 1 || summary.MaxLagMS != 5000 {
		t.Fatalf("unexpected graph freshness summary: %#v", summary)
	}
	clean := summarizeCanonicalGraphFreshness([]CanonicalGraphFreshness{{Status: "current"}, {Status: "empty"}})
	if clean.Status != "current" {
		t.Fatalf("empty derived graph should not degrade canonical health: %#v", clean)
	}
}

func TestCanonicalGraphStateMigrationAndFreshnessQueryAreDerivedAndScoped(t *testing.T) {
	lower := strings.ToLower(canonicalKnowledgeGraphStateMigrationSQL)
	for _, required := range []string{
		"codelocal_knowledge_graph_projection_state",
		"primary key(user_id,project_id)",
		"references codelocal_projects(user_id,project_id)",
		"source_object_count",
		"source_revision_count",
		"source_updated_at",
		"projected_at",
	} {
		if !strings.Contains(lower, required) {
			t.Fatalf("canonical graph state migration missing %q", required)
		}
	}
	query := strings.ToLower(canonicalGraphFreshnessSQL)
	for _, required := range []string{
		"privacy_classification='private_project'",
		"o.status in ('active','stale','conflicted')",
		"where ($1='' or p.user_id=$1)",
		"left join codelocal_knowledge_graph_projection_state",
	} {
		if !strings.Contains(query, required) {
			t.Fatalf("canonical graph freshness query missing %q: %s", required, query)
		}
	}
	for _, forbidden := range []string{"summary", "stable_key", "objective", "root_cause"} {
		if strings.Contains(query, forbidden) {
			t.Fatalf("canonical graph freshness query exposes content field %q", forbidden)
		}
	}
}
