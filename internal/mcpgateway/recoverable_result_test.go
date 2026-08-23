package mcpgateway

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestNormalizeRecoverableToolResultClearsContextRefreshError(t *testing.T) {
	result := &mcp.CallToolResult{
		IsError: true,
		StructuredContent: map[string]any{
			"status": "context_refresh_required",
		},
	}
	got := normalizeRecoverableToolResult(result)
	if got == nil || got.IsError {
		t.Fatalf("context refresh must be recoverable: %#v", got)
	}
}

func TestNormalizeRecoverableToolResultPreservesRealErrors(t *testing.T) {
	result := &mcp.CallToolResult{
		IsError: true,
		StructuredContent: map[string]any{
			"status": "blocked",
		},
	}
	got := normalizeRecoverableToolResult(result)
	if got == nil || !got.IsError {
		t.Fatalf("real errors must stay errors: %#v", got)
	}
}
