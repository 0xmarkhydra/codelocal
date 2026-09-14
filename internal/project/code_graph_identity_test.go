package project

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/localfs"
)

func commitProjectFixture(t *testing.T, root string) {
	t.Helper()
	for _, args := range [][]string{{"add", "."}, {"-c", "user.name=CodeLocal", "-c", "user.email=test@codelocal.invalid", "commit", "-qm", "fixture"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func TestSemanticProviderInfoDoesNotBuildCodeGraph(t *testing.T) {
	root := t.TempDir()
	initNestedProjectRepo(t, root, ".")
	writeKnowledgeFixture(t, root, "main.go", "package sample\nfunc Target() {}\n")

	fs, err := localfs.New(root)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(fs)
	defer engine.Close()

	info := engine.SemanticProviderInfo()
	if info["codeGraph"] != nil {
		t.Fatalf("provider-only semantic info unexpectedly contains Code Graph data: %#v", info)
	}
	if engine.indexBuiltAt != 0 || len(engine.index) != 0 {
		t.Fatalf("provider-only semantic info built the structural index: builtAt=%d files=%d", engine.indexBuiltAt, len(engine.index))
	}
}

func TestCodeGraphSnapshotTracksRepositorySourceRevision(t *testing.T) {
	root := t.TempDir()
	initNestedProjectRepo(t, root, ".")
	writeKnowledgeFixture(t, root, "main.go", "package sample\nfunc Target() {}\n")
	commitProjectFixture(t, root)

	fs, err := localfs.New(root)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(fs)
	defer engine.Close()

	first, err := engine.CodeGraphSnapshots()
	if err != nil || len(first) != 1 {
		t.Fatalf("first snapshots=%#v err=%v", first, err)
	}
	if first[0].Commit == "" || len(first[0].SourceHash) != 64 || first[0].Revision == "" || first[0].Status != "current" || first[0].Dirty {
		t.Fatalf("unexpected clean snapshot: %#v", first[0])
	}
	if first[0].FileCount != 1 || first[0].SymbolCount < 1 || first[0].IndexedAt <= 0 {
		t.Fatalf("snapshot counts/freshness missing: %#v", first[0])
	}

	writeKnowledgeFixture(t, root, "main.go", "package sample\nfunc Target() { helper() }\nfunc helper() {}\n")
	second, err := engine.CodeGraphSnapshots()
	if err != nil || len(second) != 1 {
		t.Fatalf("second snapshots=%#v err=%v", second, err)
	}
	if !second[0].Dirty || second[0].SourceHash == first[0].SourceHash || second[0].Revision == first[0].Revision {
		t.Fatalf("working-tree change did not change graph revision: first=%#v second=%#v", first[0], second[0])
	}

	info := engine.SemanticInfo()
	graph, _ := info["codeGraph"].(map[string]any)
	if graph["status"] != "current" {
		t.Fatalf("semantic info missing Code Graph status: %#v", info)
	}
}

func TestCanonicalSymbolIdentityRequiresSemanticResolution(t *testing.T) {
	root := t.TempDir()
	initNestedProjectRepo(t, root, ".")
	writeKnowledgeFixture(t, root, "main.go", "package sample\nfunc Target() {}\n")
	fs, err := localfs.New(root)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(fs)
	defer engine.Close()

	semantic := map[string]any{
		"path": "main.go", "name": "Target", "detail": "sample", "line": 2, "column": 6,
		"resolutionMode": "lsp", "provider": "gopls", "confidence": 1.0,
	}
	engine.annotatePathMap(semantic)
	engine.annotateCanonicalSymbol(semantic)
	id := strings.TrimSpace(semantic["symbolId"].(string))
	if !strings.HasPrefix(id, "sym_") || semantic["repositoryRelativePath"] != "main.go" {
		t.Fatalf("canonical symbol identity missing: %#v", semantic)
	}

	repeat := map[string]any{
		"path": "main.go", "name": "Target", "detail": "sample", "line": 2, "column": 6,
		"resolutionMode": "lsp",
	}
	engine.annotateCanonicalSymbol(repeat)
	if repeat["symbolId"] != id {
		t.Fatalf("symbol ID is not deterministic: first=%q repeat=%#v", id, repeat)
	}

	fallback := map[string]any{"path": "main.go", "name": "Target", "line": 2, "column": 6, "resolutionMode": "ast"}
	engine.annotateCanonicalSymbol(fallback)
	if fallback["symbolId"] != nil {
		t.Fatalf("non-semantic fallback was incorrectly promoted to canonical symbol: %#v", fallback)
	}
}
