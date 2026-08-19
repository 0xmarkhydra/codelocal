package cloudserver

import (
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/0xmarkhydra/codelocal/internal/gateway"
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
		req := httptest.NewRequest("GET", "/api/v1/code/graph?depth="+raw, nil)
		if got := codeGraphDepth(req); got != want {
			t.Fatalf("depth %q=%d want %d", raw, got, want)
		}
	}
}

func TestCodeGraphDisplayViewDefaultsToArchitecture(t *testing.T) {
	for raw, want := range map[string]string{"": "architecture", "architecture": "architecture", "files": "files", "invalid": "architecture"} {
		req := httptest.NewRequest("GET", "/api/v1/code/graph?view="+raw, nil)
		if got := codeGraphDisplayView(req); got != want {
			t.Fatalf("view %q=%q want %q", raw, got, want)
		}
	}
}

func TestCodeGraphParamsAreRuneBounded(t *testing.T) {
	symbol := strings.Repeat("đ", 200)
	repository := strings.Repeat("仓", 300)
	req := httptest.NewRequest("GET", "/api/v1/code/graph?symbol="+symbol+"&repositoryPath="+repository, nil)
	gotSymbol := codeGraphSymbol(req)
	gotRepository := boundedCodeGraphParam(req.URL.Query().Get("repositoryPath"), 240)
	if !utf8.ValidString(gotSymbol) || !utf8.ValidString(gotRepository) {
		t.Fatal("bounded graph parameters must preserve valid UTF-8")
	}
	if utf8.RuneCountInString(gotSymbol) != 160 || utf8.RuneCountInString(gotRepository) != 240 {
		t.Fatalf("unexpected rune bounds symbol=%d repository=%d", utf8.RuneCountInString(gotSymbol), utf8.RuneCountInString(gotRepository))
	}
}
