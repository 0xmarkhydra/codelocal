package workspace

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/state"
)

func TestRegistryConcurrentGrantAndRevoke(t *testing.T) {
	old := os.Getenv("CODELOCAL_STATE_DIR")
	stateDir := t.TempDir()
	if err := os.Setenv("CODELOCAL_STATE_DIR", stateDir); err != nil {
		t.Fatal(err)
	}
	defer os.Setenv("CODELOCAL_STATE_DIR", old)

	base := t.TempDir()
	a := filepath.Join(base, "a")
	b := filepath.Join(base, "b")
	if err := os.MkdirAll(a, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(b, 0o755); err != nil {
		t.Fatal(err)
	}

	reg := New()
	var wg sync.WaitGroup
	for _, path := range []string{a, b} {
		path := path
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := reg.Grant(path, ""); err != nil {
				t.Errorf("grant %s: %v", path, err)
			}
		}()
	}
	wg.Wait()

	items, err := reg.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 workspaces, got %d", len(items))
	}
	if _, err := os.Stat(filepath.Join(state.Dir(), "workspaces.json")); err != nil {
		t.Fatal(err)
	}

	removed, err := reg.Revoke(IDForPath(a))
	if err != nil || !removed {
		t.Fatalf("revoke: removed=%v err=%v", removed, err)
	}
	items, err = reg.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].WorkspaceID != IDForPath(b) {
		t.Fatalf("unexpected registry after revoke: %#v", items)
	}
}

func TestRegistryRefusesHome(t *testing.T) {
	old := os.Getenv("CODELOCAL_STATE_DIR")
	if err := os.Setenv("CODELOCAL_STATE_DIR", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer os.Setenv("CODELOCAL_STATE_DIR", old)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New().Grant(home, ""); err == nil {
		t.Fatal("expected home directory grant to be refused")
	}
}
