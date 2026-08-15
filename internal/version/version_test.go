package version

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestVersionMatchesPackageJSON(t *testing.T) {
	root := filepath.Join("..", "..")
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatal(err)
	}
	if pkg.Version != Version {
		t.Fatalf("package version %s != Go runtime version %s", pkg.Version, Version)
	}
}
