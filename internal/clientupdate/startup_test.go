package clientupdate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCheckStartupFetchesNPMDistTagAndCachesNotice(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/-/package/codelocal/dist-tags" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"latest":"1.5.14","beta":"1.6.0-beta.1"}`))
	}))
	defer server.Close()

	now := time.Unix(1_800_000_000, 0)
	options := StartupOptions{
		InstalledVersion: "1.5.13",
		StateDir:         t.TempDir(),
		Channel:          "latest",
		RegistryURL:      server.URL,
		CacheTTL:         6 * time.Hour,
		HTTPClient:       server.Client(),
		Now:              func() time.Time { return now },
	}
	notice, err := CheckStartup(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if notice == nil || notice.LatestVersion != "1.5.14" || notice.UpdateCommand != "npm i -g codelocal@latest" {
		t.Fatalf("unexpected notice: %#v", notice)
	}
	if hits.Load() != 1 {
		t.Fatalf("registry hits=%d want=1", hits.Load())
	}

	// A fresh cache must make the next startup network-free while still showing
	// the already-known update.
	server.Close()
	notice, err = CheckStartup(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if notice == nil || notice.LatestVersion != "1.5.14" {
		t.Fatalf("cached notice lost: %#v", notice)
	}
	if hits.Load() != 1 {
		t.Fatalf("fresh cache unexpectedly hit registry again: %d", hits.Load())
	}
}

func TestCheckStartupFreshUpToDateCacheSkipsNetwork(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(1_800_000_000, 0)
	if err := writeStartupCache(dir, startupCache{CheckedAt: now.UnixMilli(), Channel: "latest", LatestVersion: "1.5.14"}); err != nil {
		t.Fatal(err)
	}
	notice, err := CheckStartup(context.Background(), StartupOptions{
		InstalledVersion: "1.5.14",
		StateDir:         dir,
		Channel:          "latest",
		RegistryURL:      "http://127.0.0.1:1",
		CacheTTL:         6 * time.Hour,
		Now:              func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if notice != nil {
		t.Fatalf("up-to-date install should not receive notice: %#v", notice)
	}
}

func TestCheckStartupOfflineFallsBackToStaleCache(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(1_800_000_000, 0)
	if err := writeStartupCache(dir, startupCache{CheckedAt: now.Add(-24 * time.Hour).UnixMilli(), Channel: "latest", LatestVersion: "1.5.14"}); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 50 * time.Millisecond}
	notice, err := CheckStartup(context.Background(), StartupOptions{
		InstalledVersion: "1.5.13",
		StateDir:         dir,
		Channel:          "latest",
		RegistryURL:      "http://127.0.0.1:1",
		CacheTTL:         time.Hour,
		HTTPClient:       client,
		Now:              func() time.Time { return now },
	})
	if err == nil {
		t.Fatal("expected registry error while offline")
	}
	if notice == nil || notice.LatestVersion != "1.5.14" {
		t.Fatalf("stale cache should preserve known update: %#v", notice)
	}
}

func TestStartupOptionsInferBetaAndRespectConfiguredRegistry(t *testing.T) {
	t.Setenv("CODELOCAL_RELEASE_CHANNEL", "")
	t.Setenv("CODELOCAL_UPDATE_REGISTRY", "")
	t.Setenv("npm_config_registry", "https://registry.example.test/")
	options := StartupOptionsFromEnv("1.6.0-beta.2", t.TempDir())
	if options.Channel != "beta" {
		t.Fatalf("channel=%q want beta", options.Channel)
	}
	if options.RegistryURL != "https://registry.example.test/" {
		t.Fatalf("registry=%q", options.RegistryURL)
	}

	stable := StartupOptionsFromEnv("1.5.14", t.TempDir())
	if stable.Channel != "latest" {
		t.Fatalf("stable channel=%q want latest", stable.Channel)
	}
}

func TestStartupCheckEnabledCanBeDisabled(t *testing.T) {
	t.Setenv("CODELOCAL_UPDATE_CHECK", "0")
	if StartupCheckEnabled() {
		t.Fatal("update check should be disabled")
	}
	t.Setenv("CODELOCAL_UPDATE_CHECK", "")
	if !StartupCheckEnabled() {
		t.Fatal("update check should default to enabled")
	}
}

func TestStartupCacheIsPrivateAndRenderCLIIsActionable(t *testing.T) {
	dir := t.TempDir()
	if err := writeStartupCache(dir, startupCache{CheckedAt: 1, Channel: "latest", LatestVersion: "1.5.14"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(startupCachePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("cache mode=%#o want 0600", info.Mode().Perm())
	}
	text := RenderCLI(Notice{InstalledVersion: "1.5.13", LatestVersion: "1.5.14", UpdateCommand: "npm i -g codelocal@latest", RestartCommand: "codelocal"})
	for _, want := range []string{"1.5.13", "1.5.14", "npm i -g codelocal@latest", "Then run: codelocal"} {
		if !strings.Contains(text, want) {
			t.Fatalf("CLI notice missing %q: %s", want, text)
		}
	}
}
