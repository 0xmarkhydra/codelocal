package mcpgateway

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPublicToolSurfaceGenerationThreeAddsOnlyBlog(t *testing.T) {
	first := PublicToolSurface()
	second := PublicToolSurface()
	if first != second {
		t.Fatalf("tool surface must be stable within a process: %#v != %#v", first, second)
	}
	if first.Version != 3 || first.Version != PublicToolSurfaceVersion {
		t.Fatalf("surface version=%d want generation 3", first.Version)
	}
	if first.Count != len(compactToolDefinitions()) || first.Count != 21 {
		t.Fatalf("generation 3 should expose exactly 21 tools: surface=%d registry=%d", first.Count, len(compactToolDefinitions()))
	}
	if len(first.Hash) != 64 {
		t.Fatalf("generation-3 surface hash must be sha256: %q", first.Hash)
	}
	if _, ok := currentPublicToolNames()["blog"]; !ok {
		t.Fatal("generation 3 must advertise the blog tool")
	}
}

func TestGenerationThreePreservesGenerationTwoABI(t *testing.T) {
	legacy := legacyPublicToolDefinitions()
	if len(legacy) != 20 {
		t.Fatalf("legacy generation must retain 20 tools, got %d", len(legacy))
	}
	if got := legacyPublicToolSurfaceHash(); got != PinnedLegacyPublicToolSurfaceHash {
		t.Fatalf("generation-2 tool surface drifted: got %s want %s", got, PinnedLegacyPublicToolSurfaceHash)
	}
	if got := legacyPublicToolContractHash(); got != PinnedLegacyPublicToolContractHash {
		t.Fatalf("generation-2 tool contract drifted: got %s want %s", got, PinnedLegacyPublicToolContractHash)
	}
	if PublicMCPImplementationVersion != "1.5.16" {
		t.Fatalf("public MCP implementation identity changed with the app release: %q", PublicMCPImplementationVersion)
	}
	current := publicToolContractHash()
	if len(current) != 64 || current == PinnedLegacyPublicToolContractHash {
		t.Fatalf("generation-3 contract hash must be a distinct sha256: %q", current)
	}
}

func TestBlogToolContractIsActionDrivenAndCloudScoped(t *testing.T) {
	defs := compactBlogToolDefinitions()
	if len(defs) != 1 || defs[0].Name != "blog" {
		t.Fatalf("unexpected blog definitions: %#v", defs)
	}
	def := defs[0]
	if def.Resolve != nil || def.Execute == nil {
		t.Fatal("blog must execute against cloud Store directly rather than route through local workspace operations")
	}
	if def.Annotations == nil || def.Annotations.ReadOnlyHint || !annotationFlag(def.Annotations.DestructiveHint) || annotationFlag(def.Annotations.OpenWorldHint) {
		t.Fatalf("unexpected blog annotations: %#v", def.Annotations)
	}
	var schema struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(def.Schema, &schema); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(schema.Required, []string{"action"}) {
		t.Fatalf("blog schema required=%v", schema.Required)
	}
	if !reflect.DeepEqual(schema.Properties["action"].Enum, blogToolActions) {
		t.Fatalf("blog actions=%v want=%v", schema.Properties["action"].Enum, blogToolActions)
	}
	if _, exists := schema.Properties["workspaceKey"]; exists {
		t.Fatal("cloud Blog tool must not require or advertise a local workspaceKey")
	}
}

func TestUnsupportedActionExplainsSchemaRecovery(t *testing.T) {
	err := unsupportedActionSchemaError("old_action")
	if err == nil {
		t.Fatal("expected compatibility error")
	}
	message := err.Error()
	for _, want := range []string{"old_action", "tool surface", "reconnect or refresh CodeLocal"} {
		if !strings.Contains(message, want) {
			t.Fatalf("error missing %q: %s", want, message)
		}
	}
}

func TestToolResultsCarrySurfaceFingerprintWithoutTextInflation(t *testing.T) {
	result := textResult(map[string]any{"ok": true}, false)
	root, ok := result.StructuredContent.(map[string]any)
	if !ok || root == nil {
		t.Fatalf("structured content=%#v", result.StructuredContent)
	}
	surface, ok := root["codeLocalToolSurface"].(ToolSurfaceInfo)
	if !ok {
		t.Fatalf("missing typed tool surface metadata: %#v", root["codeLocalToolSurface"])
	}
	if surface != PublicToolSurface() {
		t.Fatalf("surface metadata=%#v want %#v", surface, PublicToolSurface())
	}
	if len(result.Content) == 0 || strings.Contains(result.Content[0].(*mcp.TextContent).Text, surface.Hash) {
		t.Fatal("tool surface fingerprint should stay in structured metadata unless a compatibility notice is needed")
	}
}
