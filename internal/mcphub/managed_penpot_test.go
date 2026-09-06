package mcphub

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestManagedPenpotConfigFromOverride(t *testing.T) {
	t.Setenv("CODELOCAL_PENPOT_MCP_URL", "http://127.0.0.1:55123/mcp")
	cfg, ok := managedPenpotConfig()
	if !ok {
		t.Fatal("expected managed Penpot config")
	}
	if cfg.Name != managedPenpotName || !cfg.Managed || !cfg.Enabled || cfg.Scope != "global" || cfg.Transport != "http" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if cfg.URL != "http://127.0.0.1:55123/mcp" {
		t.Fatalf("unexpected managed URL: %s", cfg.URL)
	}
}

func TestManagedPenpotConfigIsAlwaysExposed(t *testing.T) {
	t.Setenv("CODELOCAL_PENPOT_MCP_URL", "")
	t.Setenv("CODELOCAL_PENPOT_MCP_CLI", "")
	t.Setenv("CODELOCAL_PACKAGE_ROOT", "")
	t.Setenv("PATH", "")
	cfg, ok := managedPenpotConfig()
	if !ok {
		t.Fatal("default-installed Penpot server must remain visible before its lazy runtime starts")
	}
	if cfg.Name != managedPenpotName || !cfg.Managed || !cfg.Enabled || cfg.URL != managedPenpotURL {
		t.Fatalf("unexpected default managed config: %#v", cfg)
	}
}

func TestListIncludesManagedPenpotWithoutPackagedBackend(t *testing.T) {
	t.Setenv("CODELOCAL_STATE_DIR", t.TempDir())
	t.Setenv("CODELOCAL_PENPOT_MCP_URL", "")
	t.Setenv("CODELOCAL_PENPOT_MCP_CLI", "")
	t.Setenv("CODELOCAL_PACKAGE_ROOT", "")
	t.Setenv("PATH", "")
	hub, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Close()
	value, err := hub.List()
	if err != nil {
		t.Fatal(err)
	}
	servers, ok := value.([]map[string]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("unexpected server list: %#v", value)
	}
	server := servers[0]
	if server["name"] != managedPenpotName || server["managed"] != true || server["version"] != managedPenpotVersion {
		t.Fatalf("managed Penpot was not exposed: %#v", server)
	}
}

func TestEffectiveManagedPenpotCannotBeShadowed(t *testing.T) {
	t.Setenv("CODELOCAL_PENPOT_MCP_URL", "http://127.0.0.1:55124/mcp")
	reg := registryFile{Version: 1, Servers: []ServerConfig{{
		Name: managedPenpotName, Enabled: true, Scope: "global", Transport: "http", URL: "https://example.invalid/mcp",
	}}}
	servers := effective(reg, t.TempDir())
	if len(servers) != 1 {
		t.Fatalf("expected one effective server, got %d", len(servers))
	}
	if !servers[0].Managed || servers[0].URL != "http://127.0.0.1:55124/mcp" {
		t.Fatalf("managed Penpot must override registry shadow: %#v", servers[0])
	}
}

func TestNormalizeRejectsReservedPenpotName(t *testing.T) {
	_, err := normalize(t.TempDir(), ServerConfig{Name: managedPenpotName, Enabled: true, Scope: "global", Transport: "http", URL: "https://example.com/mcp"}, nil)
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("expected reserved-name error, got %v", err)
	}
}

func TestManagedPenpotCommandFindsPackagedEntry(t *testing.T) {
	if _, err := os.Stat("/usr/bin/env"); err != nil {
		t.Skip("requires a normal host filesystem")
	}
	root := t.TempDir()
	entry := filepath.Join(root, "node_modules", "@penpot", "mcp", "bin", "mcp-local.js")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte("#!/usr/bin/env node\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODELOCAL_PENPOT_MCP_CLI", "")
	t.Setenv("CODELOCAL_PACKAGE_ROOT", root)
	command, args, ok := managedPenpotCommand()
	if !ok {
		t.Skip("node is not available on this test host")
	}
	if filepath.Base(command) != "node" || len(args) != 1 || args[0] != entry {
		t.Fatalf("unexpected packaged command: %q %#v", command, args)
	}
}

func TestManagedPenpotCommandFindsHoistedEntry(t *testing.T) {
	rootParent := t.TempDir()
	root := filepath.Join(rootParent, "codelocal")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(rootParent, "@penpot", "mcp", "bin", "mcp-local.js")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte("#!/usr/bin/env node\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODELOCAL_PENPOT_MCP_CLI", "")
	t.Setenv("CODELOCAL_PACKAGE_ROOT", root)
	command, args, ok := managedPenpotCommand()
	if !ok {
		t.Skip("node is not available on this test host")
	}
	if filepath.Base(command) != "node" || len(args) != 1 || args[0] != entry {
		t.Fatalf("unexpected hoisted command: %q %#v", command, args)
	}
}

func TestManagedPenpotCommandFallsBackToPinnedNPX(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable fixture uses Unix permissions")
	}
	bin := t.TempDir()
	npx := filepath.Join(bin, "npx")
	if err := os.WriteFile(npx, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODELOCAL_PENPOT_MCP_CLI", "")
	t.Setenv("CODELOCAL_PACKAGE_ROOT", "")
	t.Setenv("PATH", bin)
	command, args, ok := managedPenpotCommand()
	if !ok {
		t.Fatal("expected pinned npx fallback")
	}
	if command != npx || len(args) != 2 || args[0] != "-y" || args[1] != managedPenpotNPXPackage {
		t.Fatalf("unexpected npx fallback: %q %#v", command, args)
	}
}

func TestManagedPenpotEnvEnablesSQLite(t *testing.T) {
	t.Setenv("NODE_OPTIONS", "--max-old-space-size=2048")
	env := managedPenpotEnv()
	var options string
	for _, entry := range env {
		if strings.HasPrefix(entry, "NODE_OPTIONS=") {
			options = strings.TrimPrefix(entry, "NODE_OPTIONS=")
		}
	}
	if !strings.Contains(options, "--max-old-space-size=2048") || !strings.Contains(options, "--experimental-sqlite") {
		t.Fatalf("unexpected NODE_OPTIONS: %q", options)
	}
}
