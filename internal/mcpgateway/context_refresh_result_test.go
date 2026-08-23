package mcpgateway

import "testing"

func TestContextRefreshRequiredIsRecoverable(t *testing.T) {
	result := textResult(map[string]any{"projectBrain": map[string]any{}}, false)
	got := contextRefreshRequired(result, []string{"internal/localclient/engine.go"})
	if got == nil {
		t.Fatal("expected result")
	}
	if got.IsError {
		t.Fatal("context refresh is recoverable and must not be an MCP tool error")
	}
	root := resultRoot(got)
	if root["status"] != "context_refresh_required" {
		t.Fatalf("unexpected status: %#v", root["status"])
	}
}
