package cloudserver

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/gateway"
	"github.com/0xmarkhydra/codelocal/internal/project"
)

func TestChooseCodeGraphWorkspacePrefersActive(t *testing.T) {
	catalog := []gateway.WorkspaceView{
		{Key: "offline", Status: "device_offline"},
		{Key: "sleeping", Status: "sleeping"},
		{Key: "active", Status: "active"},
	}
	if got := chooseCodeGraphWorkspace(catalog, ""); got == nil || got.Key != "active" {
		t.Fatalf("expected active workspace, got %#v", got)
	}
	if got := chooseCodeGraphWorkspace(catalog, "sleeping"); got == nil || got.Key != "sleeping" {
		t.Fatalf("explicit workspace selector must win, got %#v", got)
	}
}

func TestCodeGraphDepthIsBounded(t *testing.T) {
	for raw, want := range map[string]int{"": 1, "0": 1, "2": 2, "99": 3} {
		req := httptest.NewRequest("GET", "/dashboard/code-graph?depth="+raw, nil)
		if got := codeGraphDepth(req); got != want {
			t.Fatalf("depth %q=%d want %d", raw, got, want)
		}
	}
}

func TestCodeGraphDisplayViewDefaultsToArchitecture(t *testing.T) {
	for raw, want := range map[string]string{"": "architecture", "architecture": "architecture", "files": "files", "invalid": "architecture"} {
		req := httptest.NewRequest("GET", "/dashboard/code-graph?view="+raw, nil)
		if got := codeGraphDisplayView(req); got != want {
			t.Fatalf("view %q=%q want %q", raw, got, want)
		}
	}
}

func TestCodeGraphStatusExplainsOfflineWithoutCloudFallback(t *testing.T) {
	workspace := &gateway.WorkspaceView{DeviceName: "MacBook", WorkspaceName: "repo"}
	html := codeGraphStatus(project.CodeGraphView{}, workspace, "offline")
	for _, want := range []string{"Local runtime offline", "No raw source graph is stored in Cloud"} {
		if !strings.Contains(html, want) {
			t.Fatalf("offline state must explain privacy boundary %q", want)
		}
	}
}

func TestCodeGraphStatusExplainsAmbiguousSymbolWithoutGuessing(t *testing.T) {
	workspace := &gateway.WorkspaceView{DeviceName: "MacBook", WorkspaceName: "repo"}
	html := codeGraphStatus(project.CodeGraphView{Status: "ambiguous"}, workspace, "current")
	for _, want := range []string{"Multiple symbols match this query", "will not guess", "Store.Save"} {
		if !strings.Contains(html, want) {
			t.Fatalf("ambiguous state must explain safe disambiguation %q: %s", want, html)
		}
	}
}

func TestCodeGraphNeuralUsesBoundedRuntimePayload(t *testing.T) {
	view := project.CodeGraphView{
		Status: "current", Depth: 1, MaxNodes: codeGraphMaxVisibleNodes, GeneratedBy: "local-runtime",
		Nodes: []project.CodeGraphNode{{ID: "sym_a", Kind: "symbol", Name: "A", Provider: "gopls", ResolutionMode: "lsp", Confidence: 1}},
	}
	html := codeGraphNeural(view, "A", 1)
	for _, want := range []string{`data-neural-mode="code"`, `Find a function, method, file or symbol`, `semantic edges glow strongest`, `"generatedBy":"local-runtime"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("code graph neural payload missing %q", want)
		}
	}
	if strings.Contains(strings.ToLower(html), `"content":`) {
		t.Fatal("code graph dashboard payload must not contain raw source content")
	}
}
