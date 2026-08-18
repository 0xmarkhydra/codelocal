package project

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/localfs"
)

func newProjectTestEngine(t *testing.T, files map[string]string) *Engine {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fs, err := localfs.New(root)
	if err != nil {
		t.Fatal(err)
	}
	return New(fs)
}

func TestGoStructuralIndexUsesASTForMethodsAndImports(t *testing.T) {
	engine := newProjectTestEngine(t, map[string]string{
		"main.go": `package sample
import "fmt"
type Store struct{}
func (s *Store) Save() {
	fmt.Println("import { fake } from './fake'")
}
`,
	})
	defer engine.Close()

	symbols, err := engine.Symbols("Save", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(symbols) != 1 || symbols[0]["name"] != "Save" || symbols[0]["line"] != 4 {
		t.Fatalf("receiver method was not indexed by Go AST: %#v", symbols)
	}

	edges, err := engine.ImportGraph(20)
	if err != nil {
		t.Fatal(err)
	}
	foundFmt := false
	for _, edge := range edges {
		if edge["from"] != "main.go" {
			continue
		}
		if edge["specifier"] == "./fake" {
			t.Fatalf("Go string literal became a false import edge: %#v", edges)
		}
		if edge["specifier"] == "fmt" {
			foundFmt = true
		}
	}
	if !foundFmt {
		t.Fatalf("real Go import missing from graph: %#v", edges)
	}
}

func TestCallerTextFallbackExcludesFunctionDeclaration(t *testing.T) {
	engine := newProjectTestEngine(t, map[string]string{
		"main.go": `package sample
func UpdateUserPassword() {}
func ResetPassword() {
	UpdateUserPassword()
}
`,
	})
	defer engine.Close()

	callers, err := engine.CallersAt(context.Background(), "", 1, 1, "UpdateUserPassword", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(callers) != 1 {
		t.Fatalf("callers=%#v want one real call site", callers)
	}
	if callers[0]["path"] != "main.go" || callers[0]["resolutionMode"] != "text" || callers[0]["fallbackReason"] != "position_unavailable" {
		t.Fatalf("fallback provenance missing: %#v", callers[0])
	}
}

func TestGoCalleesRecognizesGenericAndSelectorCalls(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte(`package sample
func Target() {
	Generic[int]()
	service.Run[string]()
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	callees, err := goCallees(path, 2, "Target", 20)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, callee := range callees {
		seen[callee["name"].(string)] = true
	}
	if !seen["Generic"] || !seen["service.Run"] {
		t.Fatalf("generic call expressions were not normalized correctly: %#v", callees)
	}
}

func TestGoCalleesStopsAtSelectedFunctionBody(t *testing.T) {
	engine := newProjectTestEngine(t, map[string]string{
		"main.go": `package sample
func Target() {
	alpha()
}
func Following() {
	beta()
}
func alpha() {}
func beta() {}
`,
	})
	if engine.LSP != nil {
		engine.LSP.Close()
		engine.LSP = nil
	}
	defer engine.Close()

	callees, err := engine.CalleesAt(context.Background(), "main.go", 2, 6, "Target", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(callees) != 1 || callees[0]["name"] != "alpha" {
		t.Fatalf("callees crossed function boundary: %#v", callees)
	}
	if callees[0]["provider"] != "go-ast" || callees[0]["resolutionMode"] != "ast" || callees[0]["fallbackReason"] != "lsp_unavailable" {
		t.Fatalf("AST fallback provenance missing: %#v", callees[0])
	}
}
