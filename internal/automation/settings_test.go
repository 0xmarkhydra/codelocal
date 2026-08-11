package automation

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDefaultSettingsAreSafeAndUseful(t *testing.T) {
	settings := Default()
	if !settings.Coding {
		t.Fatal("coding must always be enabled")
	}
	if !settings.Browser.Enabled {
		t.Fatal("browser automation should be enabled by default")
	}
	if settings.Browser.Prepared {
		t.Fatal("browser must not be marked prepared before provisioning succeeds")
	}
	if settings.Computer.Enabled {
		t.Fatal("full computer use must remain opt-in")
	}
}

func TestSettingsRoundTripInLocalState(t *testing.T) {
	t.Setenv("CODELOCAL_STATE_DIR", t.TempDir())
	settings := Default()
	settings.Browser.Enabled = false
	settings.Browser.Prepared = true
	settings.Computer.Enabled = true
	if err := Save(settings); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded == nil {
		t.Fatal("Load() returned nil settings")
	}
	if loaded.Browser.Enabled {
		t.Fatal("browser setting did not round-trip")
	}
	if !loaded.Browser.Prepared {
		t.Fatal("browser prepared state did not round-trip")
	}
	if !loaded.Computer.Enabled {
		t.Fatal("computer setting did not round-trip")
	}
	if loaded.CompletedAt == 0 {
		t.Fatal("completedAt should be persisted")
	}
	if loaded.Version != settingsVersion {
		t.Fatalf("version = %d, want %d", loaded.Version, settingsVersion)
	}
}

func TestLoadMigratesLegacySettingsWithoutDisablingCoding(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CODELOCAL_STATE_DIR", dir)
	legacy := []byte("{\"version\":1,\"coding\":false,\"browser\":{\"enabled\":true},\"computer\":{\"enabled\":false}}\n")
	if err := os.WriteFile(filepath.Join(dir, "automation.json"), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || !loaded.Coding {
		t.Fatal("legacy settings must never disable Coding")
	}
	if loaded.Browser.Prepared {
		t.Fatal("legacy settings must remain retryable until browser preparation succeeds")
	}
}

func TestLoadMigratesSharedBrowserCacheSettings(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CODELOCAL_STATE_DIR", dir)
	legacy := []byte("{\"version\":2,\"coding\":true,\"browser\":{\"enabled\":true,\"prepared\":true},\"computer\":{\"enabled\":true}}\n")
	if err := os.WriteFile(filepath.Join(dir, "automation.json"), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.Browser.Prepared {
		t.Fatal("version 2 browser settings must be prepared again in CodeLocal-owned storage")
	}
}

func TestBrowserCLIPathPrefersBundledEnvironmentPath(t *testing.T) {
	dir := t.TempDir()
	name := "playwright-cli"
	if runtime.GOOS == "windows" {
		name += ".cmd"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("stub"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODELOCAL_PLAYWRIGHT_CLI", path)
	if got := BrowserCLIPath(); got != path {
		t.Fatalf("BrowserCLIPath() = %q, want %q", got, path)
	}
}

func TestEnsureBrowserRuntimeStreamsProgress(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	t.Setenv("CODELOCAL_STATE_DIR", stateDir)
	path := filepath.Join(dir, "playwright-cli")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf 'downloading browser: %s %s at %s\\n' \"$1\" \"$2\" \"$PLAYWRIGHT_BROWSERS_PATH\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODELOCAL_PLAYWRIGHT_CLI", path)
	var progress bytes.Buffer
	if err := EnsureBrowserRuntime(context.Background(), &progress); err != nil {
		t.Fatal(err)
	}
	if got, want := progress.String(), "downloading browser: install-browser chromium at "+filepath.Join(stateDir, "browser-runtime")+"\n"; got != want {
		t.Fatalf("progress output = %q", got)
	}
	if info, err := os.Stat(filepath.Join(stateDir, "browser-runtime")); err != nil || !info.IsDir() {
		t.Fatalf("managed browser directory was not created privately: %v", err)
	}
}

func TestEnsureBrowserRuntimeIncludesCommandOutputInError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	dir := t.TempDir()
	t.Setenv("CODELOCAL_STATE_DIR", filepath.Join(dir, "state"))
	path := filepath.Join(dir, "playwright-cli")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf 'download blocked by proxy\\n' >&2\nexit 7\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODELOCAL_PLAYWRIGHT_CLI", path)
	var progress bytes.Buffer
	err := EnsureBrowserRuntime(context.Background(), &progress)
	if err == nil || !strings.Contains(err.Error(), "download blocked by proxy") {
		t.Fatalf("EnsureBrowserRuntime() error = %v", err)
	}
	if got := progress.String(); got != "download blocked by proxy\n" {
		t.Fatalf("progress output = %q", got)
	}
}
