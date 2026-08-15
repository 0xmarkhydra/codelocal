package projectidentity

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeRemoteMatchesSSHAndHTTPS(t *testing.T) {
	values := []string{
		"git@github.com:0xmarkhydra/flashxflashx.git",
		"ssh://git@github.com/0xmarkhydra/flashxflashx.git",
		"https://github.com/0xmarkhydra/flashxflashx.git",
	}
	for _, value := range values {
		if got := NormalizeRemote(value); got != "github.com/0xmarkhydra/flashxflashx" {
			t.Fatalf("NormalizeRemote(%q) = %q", value, got)
		}
	}
	if got := NormalizeRemote("file:///Users/me/private/repo"); got != "" {
		t.Fatalf("local file remote must not be uploaded as identity metadata: %q", got)
	}
}

func TestTruncateRunesPreservesUnicodeProjectNames(t *testing.T) {
	value := strings.Repeat("ự", 121)
	got := truncateRunes(value, 120)
	if got != strings.Repeat("ự", 120) || len([]rune(got)) != 120 {
		t.Fatalf("unicode project name was not truncated rune-safely: %q", got)
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=CodeLocal Test", "GIT_AUTHOR_EMAIL=test@codelocal.invalid", "GIT_COMMITTER_NAME=CodeLocal Test", "GIT_COMMITTER_EMAIL=test@codelocal.invalid")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

func makeRepo(t *testing.T, root, relative, remote string) string {
	t.Helper()
	dir := filepath.Join(root, relative)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(relative+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "README.md")
	git(t, dir, "commit", "-qm", "initial")
	if remote != "" {
		git(t, dir, "remote", "add", "origin", remote)
	}
	return dir
}

func TestDiscoverMultiRepoProjectUsesStableRepositoryIDs(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "backend/product", "git@github.com:company/biddi-product.git")
	makeRepo(t, root, "backend/trading", "https://github.com/company/biddi-trading.git")
	if err := os.MkdirAll(filepath.Join(root, "node_modules", "ignored"), 0o755); err != nil {
		t.Fatal(err)
	}

	first := Discover(root, "BIDDI")
	if first.SuggestedName != "BIDDI" || len(first.Repositories) != 2 {
		t.Fatalf("unexpected snapshot: %#v", first)
	}
	if first.Repositories[0].ID == first.Repositories[1].ID {
		t.Fatal("distinct remotes must have distinct repository identities")
	}

	otherRoot := t.TempDir()
	makeRepo(t, otherRoot, "services/product", "https://github.com/company/biddi-product.git")
	makeRepo(t, otherRoot, "services/trading", "git@github.com:company/biddi-trading.git")
	second := Discover(otherRoot, "BIDDI on PC")
	ids := map[string]bool{}
	for _, repo := range first.Repositories {
		ids[repo.ID] = true
	}
	for _, repo := range second.Repositories {
		if !ids[repo.ID] {
			t.Fatalf("same repository on another machine/path must preserve identity: %#v", repo)
		}
	}
}

func TestDiscoverReadsExplicitProjectMarker(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".codelocal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".codelocal", "project.json"), []byte(`{"projectId":"prj_biddi_01","name":"BIDDI"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Discover(root, "fallback")
	if got.MarkerProjectID != "prj_biddi_01" || got.SuggestedName != "BIDDI" {
		t.Fatalf("marker was not applied: %#v", got)
	}
}
