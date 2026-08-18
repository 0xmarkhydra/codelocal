package project

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/localfs"
)

func initCodeGraphRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return root
}

func writeCodeGraphFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newCodeGraphEngine(t *testing.T, root string) *Engine {
	t.Helper()
	fs, err := localfs.New(root)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(fs)
	t.Cleanup(engine.Close)
	return engine
}

func TestCodeGraphNeighborhoodOverviewIsBoundedAndRevisionAware(t *testing.T) {
	root := initCodeGraphRepo(t)
	writeCodeGraphFile(t, root, "go.mod", "module example.com/graph\n\ngo 1.22\n")
	writeCodeGraphFile(t, root, "main.go", "package main\n\nimport \"fmt\"\n\nfunc main(){ fmt.Println(\"hi\") }\n")
	engine := newCodeGraphEngine(t, root)

	view, err := engine.CodeGraphNeighborhood(context.Background(), "", "", 1, 12)
	if err != nil {
		t.Fatal(err)
	}
	if view.MaxNodes != 12 || len(view.Nodes) > 12 {
		t.Fatalf("bounded view violated: %#v", view)
	}
	if len(view.Snapshots) != 1 || view.Snapshots[0].Revision == "" {
		t.Fatalf("missing revision-aware snapshot: %#v", view.Snapshots)
	}
	if view.GeneratedBy != "local-runtime" {
		t.Fatalf("unexpected graph source: %q", view.GeneratedBy)
	}
}

func TestCodeGraphNeighborhoodSymbolKeepsFallbackNonCanonical(t *testing.T) {
	root := initCodeGraphRepo(t)
	writeCodeGraphFile(t, root, "go.mod", "module example.com/graph\n\ngo 1.22\n")
	writeCodeGraphFile(t, root, "main.go", `package main

func helper() {}
func target() { helper() }
func caller() { target() }
`)
	engine := newCodeGraphEngine(t, root)
	engine.LSP = nil

	view, err := engine.CodeGraphNeighborhood(context.Background(), "target", "", 1, 40)
	if err != nil {
		t.Fatal(err)
	}
	if view.SelectedID == "" || len(view.Nodes) == 0 {
		t.Fatalf("expected selected symbol view: %#v", view)
	}
	selected := view.Nodes[0]
	if selected.Canonical {
		t.Fatalf("structural fallback must not be promoted to canonical: %#v", selected)
	}
	for _, edge := range view.Edges {
		if edge.Relation != "CALLS" {
			t.Fatalf("unexpected relation: %#v", edge)
		}
		if edge.Confidence <= 0 || edge.Confidence > 1 {
			t.Fatalf("edge confidence missing: %#v", edge)
		}
	}
}

func TestChooseGraphSymbolRefusesAmbiguousDuplicateMethods(t *testing.T) {
	symbols := []map[string]any{
		{"name": "Save", "path": "store.go", "line": 10, "detail": "*Store"},
		{"name": "Save", "path": "cache.go", "line": 20, "detail": "*Cache"},
	}
	selected, ambiguous := chooseGraphSymbol(symbols, "Save", "")
	if selected != nil || len(ambiguous) != 2 {
		t.Fatalf("duplicate method name must remain ambiguous: selected=%#v ambiguous=%#v", selected, ambiguous)
	}
	selected, ambiguous = chooseGraphSymbol(symbols, "Store.Save", "")
	if selected == nil || selected["path"] != "store.go" || len(ambiguous) != 0 {
		t.Fatalf("qualified receiver method should resolve deterministically: selected=%#v ambiguous=%#v", selected, ambiguous)
	}
}

func TestCodeGraphGoTypesConnectsMethodReceiverAndInterface(t *testing.T) {
	root := initCodeGraphRepo(t)
	writeCodeGraphFile(t, root, "go.mod", "module example.com/graph\n\ngo 1.22\n")
	writeCodeGraphFile(t, root, "store.go", `package graph

type Saver interface { Save() error }
type Store struct{}
func (*Store) Save() error { return nil }
`)
	engine := newCodeGraphEngine(t, root)
	if engine.LSP != nil {
		engine.LSP.Close()
		engine.LSP = nil
	}

	view, err := engine.CodeGraphNeighborhood(context.Background(), "Save", "", 2, 40)
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != "current" || view.SelectedID == "" {
		t.Fatalf("method graph did not resolve: %#v", view)
	}
	relations := map[string]bool{}
	for _, edge := range view.Edges {
		relations[edge.Relation] = true
		if edge.Relation == "IMPLEMENTS" && (edge.Provider != "go-types" || edge.ResolutionMode != "type-analysis" || edge.Confidence != 1) {
			t.Fatalf("IMPLEMENTS edge lacks authoritative type evidence: %#v", edge)
		}
	}
	if !relations["CONTAINS"] || !relations["IMPLEMENTS"] {
		t.Fatalf("expected method -> receiver -> interface chain, got edges=%#v nodes=%#v", view.Edges, view.Nodes)
	}
}

func TestBoundedCodeGraphArgsClampDepthAndNodes(t *testing.T) {
	depth, nodes := boundedCodeGraphArgs(99, 9999)
	if depth != maxCodeGraphDepth || nodes != maxCodeGraphNodes {
		t.Fatalf("unexpected bounds depth=%d nodes=%d", depth, nodes)
	}
}
