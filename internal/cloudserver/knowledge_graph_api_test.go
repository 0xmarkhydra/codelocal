package cloudserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func TestKnowledgeGraphResourceUsesResponseLocalIDsAndDropsInternalIdentifiers(t *testing.T) {
	graph := cloud.KnowledgeGraph{
		Stats: map[string]int{
			"projects":                1,
			"nodes":                   4,
			"edges":                   3,
			"sensitiveInternalMetric": 99,
		},
		Nodes: []cloud.KnowledgeNode{
			{ID: "user:private-user-id", Kind: "user", Name: "You", Scope: "global", Confidence: 1, Importance: 1},
			{ID: "project:private-project-id", Kind: "project", Name: "Alpha", Scope: "project", Confidence: 1, Importance: .9},
			{ID: "repo:private-repository-id", Kind: "repository", Name: "private-repo", Summary: "ssh://private.example/repo.git", Scope: "repository", Confidence: 1, Importance: .8},
			{ID: "device:private-device-id", Kind: "device", Name: "private-device-id", Scope: "device", Confidence: 1, Importance: .6, SourceMemoryID: "private-source-memory-id"},
		},
		Edges: []cloud.KnowledgeEdge{
			{ID: "works-on:private-project-id", From: "user:private-user-id", To: "project:private-project-id", Relation: "WORKS_ON", Confidence: 1, Importance: 1},
			{ID: "project-repo:private", From: "project:private-project-id", To: "repo:private-repository-id", Relation: "CONTAINS_REPO", Confidence: 1, Importance: .8},
			{ID: "orphan-private-edge", From: "project:private-project-id", To: "missing:private", Relation: "BROKEN", Confidence: 1, Importance: 1},
		},
	}

	payload := buildKnowledgeGraphResourceDTO(graph, 320)
	if len(payload.Nodes) != 4 || len(payload.Edges) != 2 {
		t.Fatalf("unexpected graph size: nodes=%d edges=%d", len(payload.Nodes), len(payload.Edges))
	}
	if payload.Nodes[0].ID != "n1" || payload.Nodes[1].ID != "n2" || payload.Edges[0].ID != "e1" {
		t.Fatalf("graph IDs are not response-local: nodes=%#v edges=%#v", payload.Nodes, payload.Edges)
	}
	if payload.Nodes[2].Summary != "" {
		t.Fatalf("repository remote leaked through summary: %#v", payload.Nodes[2])
	}
	if payload.Nodes[3].Name != "Device 1" {
		t.Fatalf("device identifier was not replaced by a display ordinal: %#v", payload.Nodes[3])
	}
	if _, exists := payload.Stats["sensitiveInternalMetric"]; exists {
		t.Fatalf("unknown internal graph stat leaked: %#v", payload.Stats)
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(raw)
	for _, forbidden := range []string{
		"private-user-id", "private-project-id", "private-repository-id", "private-device-id",
		"private-source-memory-id", "private.example", "orphan-private-edge", `"sourceMemoryId"`,
	} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("knowledge graph API leaked internal identifier/value %q: %s", forbidden, serialized)
		}
	}
}

func TestKnowledgeGraphResourceBoundsTextAndUnitValues(t *testing.T) {
	longName := strings.Repeat("name ", 80)
	longSummary := strings.Repeat("summary ", 100)
	payload := buildKnowledgeGraphResourceDTO(cloud.KnowledgeGraph{
		Nodes: []cloud.KnowledgeNode{{ID: "memory:1", Kind: strings.Repeat("kind", 30), Name: longName, Summary: longSummary, Scope: strings.Repeat("scope", 30), Confidence: 4, Importance: -1}},
	}, 1)
	if !payload.Meta.AtNodeLimit || payload.Meta.NodeLimit != 1 {
		t.Fatalf("unexpected graph limit metadata: %#v", payload.Meta)
	}
	if len(payload.Nodes) != 1 {
		t.Fatalf("node missing: %#v", payload.Nodes)
	}
	node := payload.Nodes[0]
	if node.Confidence != 1 || node.Importance != 0 {
		t.Fatalf("unit values not clamped: %#v", node)
	}
	if len([]rune(node.Name)) > 121 || len([]rune(node.Summary)) > 281 || len([]rune(node.Kind)) > 65 || len([]rune(node.Scope)) > 65 {
		t.Fatalf("graph text was not bounded: %#v", node)
	}
}
