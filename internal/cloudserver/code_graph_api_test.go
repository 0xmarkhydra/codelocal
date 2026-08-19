package cloudserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/gateway"
	"github.com/0xmarkhydra/codelocal/internal/project"
)

func TestCodeGraphWorkspaceByPublicIDDoesNotRequireRoutingKey(t *testing.T) {
	catalog := []gateway.WorkspaceView{
		{Key: "private-routing-key", DeviceID: "dev-1", WorkspaceID: "ws-1", WorkspaceName: "alpha", RuntimeOnline: true},
		{Key: "private-routing-key-2", DeviceID: "dev-2", WorkspaceID: "ws-2", WorkspaceName: "beta"},
	}
	selected := codeGraphWorkspaceByPublicID(catalog, "dev-1", "ws-1")
	if selected == nil || selected.Key != "private-routing-key" {
		t.Fatalf("public workspace selection failed: %#v", selected)
	}
	if codeGraphWorkspaceByPublicID(catalog, "dev-1", "missing") != nil {
		t.Fatal("mismatched public workspace ids must not fall back to another checkout")
	}
}

func TestBuildCodeGraphResourceUsesResponseLocalIDsAndDropsInternalIdentity(t *testing.T) {
	workspace := gateway.WorkspaceView{
		Key: "private-routing-key", DeviceID: "device-private-id", DeviceName: "Mac",
		WorkspaceID: "workspace-public-id", WorkspaceName: "CodeLocal", Status: "active", RuntimeOnline: true,
		ProjectID: "private-project-id", ProjectRoot: "/Users/private/source", Capabilities: map[string]any{"shell": true},
	}
	view := project.CodeGraphView{
		Status: "current", View: "architecture", Query: "Store.Save", Depth: 2, MaxNodes: 999, SelectedID: "symbol-private-a", Truncated: true,
		Snapshots: []project.CodeGraphSnapshot{{
			RepositoryID: "repo-private-id", RepositoryPath: "packages/core", Branch: "dev", Commit: "1234567890abcdef",
			SourceHash: "private-source-hash", Revision: "private-revision", Dirty: true, IndexedAt: 123, FileCount: 4, SymbolCount: 20,
		}},
		Nodes: []project.CodeGraphNode{
			{ID: "symbol-private-a", Kind: "function", Name: "Save", Path: "internal/store.go", RepositoryID: "repo-private-id", RepositoryPath: "packages/core", QualifiedName: "Store.Save", Confidence: 1.5, Selected: true},
			{ID: "symbol-private-b", Kind: "function", Name: "Caller", Path: "internal/caller.go", RepositoryID: "repo-private-id", RepositoryPath: "packages/core", Confidence: .8},
		},
		Edges: []project.CodeGraphEdge{
			{ID: "edge-private", From: "symbol-private-b", To: "symbol-private-a", Relation: "CALLS", FallbackReason: "private fallback detail", Confidence: -.4, Count: 2},
			{ID: "orphan", From: "missing", To: "symbol-private-a", Relation: "CALLS", Confidence: 1},
		},
		Impact: &project.CodeGraphImpact{DirectCallers: 1, AffectedFiles: 2, AverageConfidence: 1.4, Risk: "medium"},
	}

	result := buildCodeGraphResource(view, &workspace, "current")
	if result.SelectedID != "n1" || len(result.Nodes) != 2 || result.Nodes[0].ID != "n1" || len(result.Edges) != 1 || result.Edges[0].ID != "e1" {
		t.Fatalf("unexpected response-local graph identity: %#v", result)
	}
	if result.MaxNodes != codeGraphMaxVisibleNodes || result.Nodes[0].Confidence != 1 || result.Edges[0].Confidence != 0 || result.Impact == nil || result.Impact.AverageConfidence != 1 {
		t.Fatalf("graph bounds were not normalized: %#v", result)
	}
	if len(result.Repositories) != 1 || result.Repositories[0].Commit != "1234567890ab" || result.Repositories[0].Path != "packages/core" {
		t.Fatalf("unexpected repository display DTO: %#v", result.Repositories)
	}

	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(raw)
	for _, forbidden := range []string{
		"private-routing-key", "private-project-id", "/Users/private/source", "repo-private-id",
		"symbol-private-a", "symbol-private-b", "edge-private", "private-source-hash", "private-revision", "private fallback detail", "capabilities",
	} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("internal code graph identity leaked (%s): %s", forbidden, serialized)
		}
	}
}

func TestCodeGraphResourceKeepsWorkspaceRelativePaths(t *testing.T) {
	view := project.CodeGraphView{
		Status: "current", View: "files", Depth: 1, MaxNodes: 10,
		Nodes: []project.CodeGraphNode{{ID: "a", Kind: "file", Name: "store.go", Path: "internal/store.go", RepositoryPath: ".", Confidence: 1}},
	}
	result := buildCodeGraphResource(view, nil, "current")
	if len(result.Nodes) != 1 || result.Nodes[0].Path != "internal/store.go" || result.Nodes[0].RepositoryPath != "." {
		t.Fatalf("workspace-relative display paths should be preserved: %#v", result.Nodes)
	}
}
