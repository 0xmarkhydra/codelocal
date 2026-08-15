package memory

import "testing"

func TestNormalizeScopeSeparatesGlobalProjectRepositoryAndWorkspaceMemory(t *testing.T) {
	scope, workspace, project, repository, err := normalizeScope("", "codex-mcp", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if scope != ScopeWorkspace || workspace != "codex-mcp" || project != "" || repository != "" {
		t.Fatalf("default scope = %q/%q/%q/%q, want workspace/codex-mcp/empty/empty", scope, workspace, project, repository)
	}

	scope, workspace, project, repository, err = normalizeScope(ScopeGlobal, "must-be-cleared", "project", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if scope != ScopeGlobal || workspace != "" || project != "" || repository != "" {
		t.Fatalf("global scope did not clear local identities: %q/%q/%q/%q", scope, workspace, project, repository)
	}

	scope, workspace, project, repository, err = normalizeScope(ScopeProject, "workspace", "project-1", "repo")
	if err != nil || scope != ScopeProject || workspace != "" || project != "project-1" || repository != "" {
		t.Fatalf("unexpected project scope normalization: %q/%q/%q/%q %v", scope, workspace, project, repository, err)
	}

	scope, workspace, project, repository, err = normalizeScope(ScopeRepository, "workspace", "project-1", "repo-1")
	if err != nil || scope != ScopeRepository || workspace != "" || project != "project-1" || repository != "repo-1" {
		t.Fatalf("unexpected repository scope normalization: %q/%q/%q/%q %v", scope, workspace, project, repository, err)
	}

	if _, _, _, _, err := normalizeScope(ScopeWorkspace, "", "", ""); err == nil {
		t.Fatal("workspace scope without workspace id should fail")
	}
	if _, _, _, _, err := normalizeScope(ScopeProject, "", "", ""); err == nil {
		t.Fatal("project scope without project id should fail")
	}
	if _, _, _, _, err := normalizeScope(ScopeRepository, "", "project-1", ""); err == nil {
		t.Fatal("repository scope without repository id should fail")
	}
	if _, _, _, _, err := normalizeScope(Scope("other"), "codex-mcp", "project-1", "repo-1"); err == nil {
		t.Fatal("unknown scope should fail")
	}
}

func TestGraphProjectionBuildsWorkspaceMemoryRelationships(t *testing.T) {
	record := Record{
		ID:          "mem-1",
		UserID:      "user-1",
		WorkspaceID: "codex-mcp",
		Scope:       ScopeWorkspace,
		TaskID:      "thread-1",
		Level:       LevelScenario,
		Summary:     "OAuth incident resolved",
		Files:       []string{"internal/oauth/server.go"},
		Symbols:     []string{"oauth.New"},
		Confidence:  .9,
		Importance:  .8,
		CreatedAt:   100,
		LastUsedAt:  200,
	}
	nodes, edges, err := graphProjection(record)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 5 {
		t.Fatalf("nodes=%d want=5: %#v", len(nodes), nodes)
	}
	if len(edges) != 4 {
		t.Fatalf("edges=%d want=4: %#v", len(edges), edges)
	}
	for index, node := range nodes {
		if node.UserID != record.UserID {
			t.Fatalf("node user=%q want=%q", node.UserID, record.UserID)
		}
		if index == 2 {
			if node.Kind != "user" || node.Scope != ScopeGlobal || node.WorkspaceID != "" {
				t.Fatalf("workspace projection should link a global user anchor safely: %#v", node)
			}
			continue
		}
		if node.Scope != ScopeWorkspace || node.WorkspaceID != record.WorkspaceID {
			t.Fatalf("node escaped workspace scope: %#v", node)
		}
	}
	if nodes[0].Kind != "workspace" || nodes[1].Kind != "scenario" {
		t.Fatalf("unexpected anchor/memory kinds: %q %q", nodes[0].Kind, nodes[1].Kind)
	}
	if edges[0].Relation != "PART_OF" {
		t.Fatalf("root relation=%q want PART_OF", edges[0].Relation)
	}
	if edges[1].Relation != "WORKS_ON" || edges[1].FromNodeID != nodes[2].ID || edges[1].ToNodeID != nodes[0].ID {
		t.Fatalf("workspace projection should connect user to workspace: %#v", edges[1])
	}
	for _, edge := range edges {
		if edge.UserID != record.UserID || edge.Scope != ScopeWorkspace || edge.WorkspaceID != record.WorkspaceID {
			t.Fatalf("edge escaped tenant/workspace scope: %#v", edge)
		}
	}

	nodesAgain, edgesAgain, err := graphProjection(record)
	if err != nil {
		t.Fatal(err)
	}
	for i := range nodes {
		if nodes[i].ID != nodesAgain[i].ID {
			t.Fatalf("node id not deterministic: %q != %q", nodes[i].ID, nodesAgain[i].ID)
		}
	}
	for i := range edges {
		if edges[i].ID != edgesAgain[i].ID {
			t.Fatalf("edge id not deterministic: %q != %q", edges[i].ID, edgesAgain[i].ID)
		}
	}
}

func TestGraphProjectionBuildsGlobalUserAnchor(t *testing.T) {
	record := Record{
		ID:         "mem-global",
		UserID:     "user-1",
		Scope:      ScopeGlobal,
		TaskID:     "thread-2",
		Level:      LevelWorkspace,
		Summary:    "Prefers concise execution-oriented answers",
		Confidence: .95,
		Importance: .9,
		CreatedAt:  300,
		LastUsedAt: 400,
	}
	nodes, edges, err := graphProjection(record)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || len(edges) != 1 {
		t.Fatalf("global projection nodes=%d edges=%d", len(nodes), len(edges))
	}
	if nodes[0].Kind != "user" || nodes[0].Scope != ScopeGlobal || nodes[0].WorkspaceID != "" {
		t.Fatalf("unexpected global anchor: %#v", nodes[0])
	}
	if nodes[1].Kind != "knowledge" || nodes[1].WorkspaceID != "" {
		t.Fatalf("unexpected global memory node: %#v", nodes[1])
	}
	if edges[0].Scope != ScopeGlobal || edges[0].WorkspaceID != "" {
		t.Fatalf("global edge leaked workspace scope: %#v", edges[0])
	}
}

func TestGraphProjectionSanitizesMemorySummary(t *testing.T) {
	nodes, _, err := graphProjection(Record{
		ID:          "mem-secret",
		UserID:      "user-1",
		WorkspaceID: "codex-mcp",
		Scope:       ScopeWorkspace,
		Level:       LevelEvent,
		Summary:     "API_KEY=super-secret-value",
		CreatedAt:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := nodes[1].Summary; got == "API_KEY=super-secret-value" || got == "" {
		t.Fatalf("summary was not sanitized: %q", got)
	}
}

func TestConversationProjectionStableKeyConsolidatesChangedMutableFact(t *testing.T) {
	first := Record{
		ID: "memory-a", UserID: "user-1", Scope: ScopeGlobal, Level: LevelWorkspace,
		Kind: "goal", SourceType: "conversation", Summary: "Target 10,000 users", Symbols: []string{"memory-key:user.goal.codelocal_users"}, CreatedAt: 100,
	}
	second := first
	second.ID = "memory-b"
	second.Summary = "Target 20,000 users"
	second.CreatedAt = 200
	firstNodes, _, err := graphProjection(first)
	if err != nil {
		t.Fatal(err)
	}
	secondNodes, _, err := graphProjection(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstNodes[1].ID != secondNodes[1].ID {
		t.Fatalf("stable memory key should consolidate changed fact: %q != %q", firstNodes[1].ID, secondNodes[1].ID)
	}
	if firstNodes[1].Summary == secondNodes[1].Summary {
		t.Fatal("stable identity must still carry the latest changed summary")
	}
}

func TestConversationProjectionConsolidatesRepeatedDurableFact(t *testing.T) {
	base := Record{
		UserID:     "user-1",
		Scope:      ScopeGlobal,
		Level:      LevelWorkspace,
		Kind:       "preference",
		SourceType: "conversation",
		Summary:    "Prefers concise execution-oriented answers",
		Confidence: .95,
		Importance: .85,
		CreatedAt:  100,
	}
	first := base
	first.ID = "memory-a"
	first.TaskID = "thread-a"
	second := base
	second.ID = "memory-b"
	second.TaskID = "thread-b"
	second.CreatedAt = 200

	firstNodes, firstEdges, err := graphProjection(first)
	if err != nil {
		t.Fatal(err)
	}
	secondNodes, _, err := graphProjection(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstNodes[1].ID != secondNodes[1].ID {
		t.Fatalf("repeated durable fact should consolidate to one graph node: %q != %q", firstNodes[1].ID, secondNodes[1].ID)
	}
	if firstNodes[1].Kind != "preference" || firstNodes[1].SourceType != "conversation" {
		t.Fatalf("conversation metadata lost: %#v", firstNodes[1])
	}
	if len(firstEdges) == 0 || firstEdges[0].Relation != "PREFERS" || firstEdges[0].FromNodeID != firstNodes[0].ID || firstEdges[0].ToNodeID != firstNodes[1].ID {
		t.Fatalf("preference should create semantic anchor edge: %#v", firstEdges)
	}
	if firstNodes[1].SourceMemoryID == secondNodes[1].SourceMemoryID {
		t.Fatal("source memories should remain distinct even when the graph node consolidates")
	}
}
