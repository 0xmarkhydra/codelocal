package ui

import (
	"strings"
	"testing"
)

func TestProductionDashboardIncludesKnowledgeGraphNavigation(t *testing.T) {
	html := Page("CodeLocal Cloud", "Signed in as user@example.com", `<div>body</div>`)
	if !strings.Contains(html, `href="/dashboard/knowledge"`) || !strings.Contains(html, `>Knowledge Graph</span>`) {
		t.Fatal("production Go dashboard must expose the Knowledge Graph page in its control-plane navigation")
	}
	if !strings.Contains(html, `'/dashboard/knowledge':'Knowledge Graph'`) {
		t.Fatal("dashboard route label map must recognize the Knowledge Graph page")
	}
}
