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

func TestBoundedCodeGraphArgsClampDepthAndNodes(t *testing.T) {
	depth, nodes := boundedCodeGraphArgs(99, 9999)
	if depth != maxCodeGraphDepth || nodes != maxCodeGraphNodes {
		t.Fatalf("unexpected bounds depth=%d nodes=%d", depth, nodes)
	}
}
