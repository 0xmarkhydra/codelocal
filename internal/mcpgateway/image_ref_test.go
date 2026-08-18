package mcpgateway

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestToolResultUsesResourceLinkForSignedImageReference(t *testing.T) {
	result := toolResultWithNotice(map[string]any{
		"windowId": "ax:1:0",
		"__mcpImageRef": map[string]any{
			"imageRef":  "sha256:abc",
			"url":       "https://media.example.test/signed.png?sig=x",
			"mimeType":  "image/png",
			"size":      int64(1234),
			"sha256":    "abc",
			"expiresAt": int64(9999999999999),
			"transport": "signed-url",
		},
	}, false, "")
	if result == nil || len(result.Content) == 0 {
		t.Fatal("signed image ref should produce MCP content")
	}
	link, ok := result.Content[len(result.Content)-1].(*mcp.ResourceLink)
	if !ok {
		t.Fatalf("expected ResourceLink, got %T", result.Content[len(result.Content)-1])
	}
	if link.URI != "https://media.example.test/signed.png?sig=x" || link.MIMEType != "image/png" {
		t.Fatalf("unexpected resource link: %#v", link)
	}
	if link.Size == nil || *link.Size != 1234 {
		t.Fatalf("unexpected resource link size: %#v", link.Size)
	}
	if structured, ok := result.StructuredContent.(map[string]any); !ok || structured["__mcpImageRef"] != nil || structured["visual"] == nil {
		t.Fatalf("internal marker should become public visual metadata: %#v", result.StructuredContent)
	}
}
