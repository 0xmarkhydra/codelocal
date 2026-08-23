package mcpgateway

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPublicToolSurfaceIsStableAndMatchesCompactRegistry(t *testing.T) {
	first := PublicToolSurface()
	second := PublicToolSurface()
	if first != second {
		t.Fatalf("tool surface must be stable within a process: %#v != %#v", first, second)
	}
	if first.Version != PublicToolSurfaceVersion {
		t.Fatalf("surface version=%d want %d", first.Version, PublicToolSurfaceVersion)
	}
	if first.Count != len(compactToolDefinitions()) {
		t.Fatalf("surface count=%d registry=%d", first.Count, len(compactToolDefinitions()))
	}
	if first.Count != 19 {
		t.Fatalf("public MCP surface changed unexpectedly: got %d tools want 19", first.Count)
	}
	if first.Hash != PinnedPublicToolSurfaceHash {
		t.Fatalf("public MCP surface changed: got %s want pinned %s; preserve the existing contract or deliberately create a compatibility generation", first.Hash, PinnedPublicToolSurfaceHash)
	}
}

func TestPublicMCPContractIsPinnedAcrossBackendReleases(t *testing.T) {
	if PublicMCPImplementationVersion != "1.5.16" {
		t.Fatalf("public MCP implementation identity changed with the app release: %q", PublicMCPImplementationVersion)
	}
	got := publicToolContractHash()
	if got != PinnedPublicToolContractHash {
		t.Fatalf("public MCP contract changed: got %s want pinned %s; tool metadata/instructions are thread-facing ABI and require an explicit compatibility generation", got, PinnedPublicToolContractHash)
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
