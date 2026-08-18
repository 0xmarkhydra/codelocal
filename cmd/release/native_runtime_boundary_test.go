package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type rootPackageManifest struct {
	Scripts map[string]string `json:"scripts"`
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestShippingScriptsRemainNativeGoOnly(t *testing.T) {
	root := repositoryRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest rootPackageManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	for name, script := range manifest.Scripts {
		lower := strings.ToLower(script)
		for _, forbidden := range []string{"src/client-v2", "src/server-saas", "src/index.ts", "ts-node", "tsx ", "node src/"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("shipping script %q reintroduced legacy TypeScript runtime dependency %q: %s", name, forbidden, script)
			}
		}
	}
}

func TestReleaseBuilderDoesNotPackageLegacyTypeScriptRuntime(t *testing.T) {
	root := repositoryRoot(t)
	err := filepath.WalkDir(filepath.Join(root, "cmd", "release"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || filepath.Ext(path) != ".go" {
			return walkErr
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := strings.ToLower(string(raw))
		for _, forbidden := range []string{"src/client-v2", "src/server-saas", "src/index.ts"} {
			if strings.Contains(text, forbidden) && !strings.HasSuffix(path, "native_runtime_boundary_test.go") {
				t.Errorf("release builder %s references quarantined legacy runtime %q", filepath.Base(path), forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
