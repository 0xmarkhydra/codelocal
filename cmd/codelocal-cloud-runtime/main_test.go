package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSafeSeedRemoteRejectsCredentialsAndNonHTTP(t *testing.T) {
	if got := safeSeedRemote("https://github.com/acme/repo.git#main"); got != "https://github.com/acme/repo.git" {
		t.Fatalf("safeSeedRemote() = %q", got)
	}
	for _, value := range []string{
		"https://token@github.com/acme/repo.git",
		"ssh://git@github.com/acme/repo.git",
		"git@github.com:acme/repo.git",
		"file:///tmp/repo",
	} {
		if got := safeSeedRemote(value); got != "" {
			t.Fatalf("unsafe remote %q accepted as %q", value, got)
		}
	}
}

func TestSafeSeedPathRejectsTraversal(t *testing.T) {
	for input, want := range map[string]string{".": ".", "apps/web": "apps/web", "apps/../api": "api"} {
		if got := safeSeedPath(input); got != want {
			t.Fatalf("safeSeedPath(%q)=%q want %q", input, got, want)
		}
	}
	for _, value := range []string{"../secret", "/etc", "../../tmp"} {
		if got := safeSeedPath(value); got != "" {
			t.Fatalf("unsafe path %q accepted as %q", value, got)
		}
	}
}

func TestHydrateWorkspaceRejectsMissingSources(t *testing.T) {
	for _, raw := range []string{"", "[]", "not-json"} {
		if err := hydrateWorkspace(context.Background(), t.TempDir(), raw); err == nil {
			t.Fatalf("hydrateWorkspace(%q) error=nil", raw)
		}
	}
}

func TestHydrateWorkspaceReusesExistingGitCheckoutWithoutNetwork(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `[{"repositoryId":"repo-1","remote":"https://github.com/acme/repo.git","relativePath":"."}]`
	if err := hydrateWorkspace(context.Background(), root, raw); err != nil {
		t.Fatalf("hydrateWorkspace() error = %v", err)
	}
}
