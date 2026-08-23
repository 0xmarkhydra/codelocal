package mcpgateway

import (
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func normalizeRecoverableToolResult(result *mcp.CallToolResult) *mcp.CallToolResult {
	if result == nil || !result.IsError {
		return result
	}
	root := resultRoot(result)
	status, _ := root["status"].(string)
	if strings.EqualFold(strings.TrimSpace(status), "context_refresh_required") {
		result.IsError = false
	}
	return result
}
