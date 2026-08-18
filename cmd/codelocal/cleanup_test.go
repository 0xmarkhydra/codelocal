package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/runtimecontrol"
)

func TestParseCleanupOptionsRequiresExplicitAll(t *testing.T) {
	if _, err := parseCleanupOptions("reset", nil); err == nil || !strings.Contains(err.Error(), "requires --all") {
		t.Fatalf("expected explicit --all error, got %v", err)
	}
	options, err := parseCleanupOptions("reset", []string{"--yes", "--all"})
	if err != nil {
		t.Fatal(err)
	}
	if !options.all || !options.yes {
		t.Fatalf("unexpected options: %#v", options)
	}
	if _, err := parseCleanupOptions("reset", []string{"--all", "--force"}); err == nil {
		t.Fatal("unknown cleanup option must be rejected")
	}
}

func TestSafeCleanupDirRejectsProtectedDirectories(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"", string(filepath.Separator), home, os.TempDir()} {
		if _, err := safeCleanupDir(dir); err == nil {
			t.Fatalf("safeCleanupDir(%q) should fail", dir)
		}
	}
}

func TestSafeCleanupDirRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated Windows privileges")
	}
	root := t.TempDir()
	target := filepath.Join(root, "target")
	link := filepath.Join(root, "state-link")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := safeCleanupDir(link); err == nil || !strings.Contains(err.Error(), "symlinked") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
}

func TestPurgeLocalStateRemovesStateDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "codelocal-state")
	if err := os.MkdirAll(filepath.Join(dir, "browser-runtime"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "device-credential.json"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	removed, err := purgeLocalState(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if removed != dir {
		t.Fatalf("removed path = %q, want %q", removed, dir)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("state directory still exists: %v", err)
	}
}

func TestPurgeLocalStateStopsRunningRuntime(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "codelocal-state")
	lease, err := runtimecontrol.Acquire("test", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !lease.Acquired {
		t.Fatal("test runtime lease was not acquired")
	}
	runtimeCtx, cancelRuntime := context.WithCancel(context.Background())
	server, err := runtimecontrol.Start(runtimeCtx, dir, lease.Record.InstanceID, func(_ context.Context, command runtimecontrol.Command) (any, error) {
		if command.Type == "shutdown" {
			cancelRuntime()
		}
		return map[string]any{"stopping": command.Type == "shutdown"}, nil
	})
	if err != nil {
		_ = lease.Release()
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		<-runtimeCtx.Done()
		_ = server.Close()
		_ = lease.Release()
		close(done)
	}()

	if _, err := purgeLocalState(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	<-done
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("state directory still exists after runtime shutdown: %v", err)
	}
}

func TestUninstallCommandPurgesStateAndInvokesNPM(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	root := t.TempDir()
	stateDir := filepath.Join(root, "state")
	binDir := filepath.Join(root, "bin")
	logPath := filepath.Join(root, "npm-args.txt")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "automation.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	npm := filepath.Join(binDir, "npm")
	if err := os.WriteFile(npm, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" > \"$CODELOCAL_TEST_NPM_LOG\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODELOCAL_STATE_DIR", stateDir)
	t.Setenv("CODELOCAL_TEST_NPM_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := uninstallCommand(context.Background(), []string{"--all", "--yes"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
		t.Fatalf("state directory still exists: %v", err)
	}
	args, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(args)); got != "uninstall -g codelocal" {
		t.Fatalf("npm arguments = %q", got)
	}
}
