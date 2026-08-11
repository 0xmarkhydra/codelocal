package automation

import (
	"os"
	"path/filepath"
	"runtime"
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
