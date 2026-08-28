package mcpgateway

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestSkillsStayInternalToPreservePublicToolSurface(t *testing.T) {
	for _, definition := range compactToolDefinitions() {
		if definition.Name == "skills" {
			t.Fatal("skills must stay internal; automatic skill routing should not expand the public MCP surface")
		}
	}
}

func TestSkillsRouteUsesCentralBuiltinWithoutWorkspace(t *testing.T) {
	result, err := executeSkillsTool(context.Background(), nil, "user", map[string]any{
		"action":  "route",
		"query":   "make this dashboard easier to scan and responsive",
		"signals": []any{"frontend", "visual", "dashboard"},
		"stack":   []any{"nextjs"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.IsError {
		t.Fatalf("expected successful result, got %#v", result)
	}
	if !strings.Contains(fmt.Sprint(result.StructuredContent), "ui-ux-pro") {
		t.Fatalf("expected ui-ux-pro selection, got %#v", result.StructuredContent)
	}
}
