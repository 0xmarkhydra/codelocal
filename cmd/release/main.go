package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type rootManifest struct {
	Version string `json:"version"`
}

type target struct {
	GOOS   string
	GOARCH string
	File   string
}

func main() {
	root, err := os.Getwd()
	must(err)
	raw, err := os.ReadFile(filepath.Join(root, "package.json"))
	must(err)
	var manifest rootManifest
	must(json.Unmarshal(raw, &manifest))
	if strings.TrimSpace(manifest.Version) == "" {
		panic("package.json version is required")
	}
	staging := filepath.Join(root, ".release", "npm")
	must(os.RemoveAll(staging))
	must(os.MkdirAll(filepath.Join(staging, "bin", "native"), 0o755))
	targets := []target{{"darwin", "arm64", "codelocal-darwin-arm64"}, {"darwin", "amd64", "codelocal-darwin-x64"}, {"linux", "arm64", "codelocal-linux-arm64"}, {"linux", "amd64", "codelocal-linux-x64"}, {"windows", "amd64", "codelocal-win32-x64.exe"}, {"windows", "arm64", "codelocal-win32-arm64.exe"}}
	for _, t := range targets {
		fmt.Printf("building %s/%s\n", t.GOOS, t.GOARCH)
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", filepath.Join(staging, "bin", "native", t.File), "./cmd/codelocal")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+t.GOOS, "GOARCH="+t.GOARCH)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		must(cmd.Run())
	}
	launcher := `#!/usr/bin/env node
const { spawnSync } = require('node:child_process');
const path = require('node:path');
const key = process.platform + '-' + process.arch;
const files = {
  'darwin-arm64': 'codelocal-darwin-arm64',
  'darwin-x64': 'codelocal-darwin-x64',
  'linux-arm64': 'codelocal-linux-arm64',
  'linux-x64': 'codelocal-linux-x64',
  'win32-x64': 'codelocal-win32-x64.exe',
  'win32-arm64': 'codelocal-win32-arm64.exe'
};
const file = files[key];
if (!file) {
  console.error('CodeLocal does not have a native binary for ' + key + '.');
  process.exit(1);
}
const packageRoot = path.resolve(__dirname, '..');
const playwrightCli = path.join(packageRoot, 'node_modules', '.bin', process.platform === 'win32' ? 'playwright-cli.cmd' : 'playwright-cli');
const binary = path.join(__dirname, 'native', file);
const env = {
  ...process.env,
  CODELOCAL_PACKAGE_ROOT: packageRoot,
  CODELOCAL_PLAYWRIGHT_CLI: process.env.CODELOCAL_PLAYWRIGHT_CLI || playwrightCli
};
const result = spawnSync(binary, process.argv.slice(2), { stdio: 'inherit', env });
if (result.error) {
  console.error(result.error.message);
  process.exit(1);
}
if (result.signal) {
  process.kill(process.pid, result.signal);
}
process.exit(result.status ?? 1);
`
	must(os.WriteFile(filepath.Join(staging, "bin", "codelocal.js"), []byte(launcher), 0o755))
	public := map[string]any{
		"name":        "codelocal",
		"version":     manifest.Version,
		"description": "Native Go runtime that securely connects ChatGPT to local development workspaces.",
		"license":     "UNLICENSED",
		"bin":         map[string]string{"codelocal": "bin/codelocal.js"},
		"files":       []string{"bin/", "README.md"},
		"engines":     map[string]string{"node": ">=20"},
		"dependencies": map[string]string{
			"@playwright/cli": "0.1.17",
		},
		"keywords": []string{"chatgpt", "mcp", "coding", "local", "go", "playwright", "browser-automation"},
	}
	publicRaw, _ := json.MarshalIndent(public, "", "  ")
	must(os.WriteFile(filepath.Join(staging, "package.json"), append(publicRaw, '\n'), 0o600))
	releaseChannel := strings.ToLower(strings.TrimSpace(os.Getenv("CODELOCAL_RELEASE_CHANNEL")))
	installCommand := "npm i -g codelocal"
	if releaseChannel == "beta" {
		installCommand += "@beta"
	}
	readme := `# CodeLocal

Native Go local development runtime for ChatGPT.

## Install

` + "```bash\n" + installCommand + "\n" + "```\n\n" + `## Authorize a project

` + "```bash\n" + `cd /path/to/project
codelocal .
` + "```\n\n" + "`codelocal .` only authorizes that folder locally. It does not pair the machine or connect to CodeLocal Cloud.\n\n" + `## Start CodeLocal

` + "```bash\n" + `codelocal
` + "```\n\n" + `On first start, CodeLocal asks which local capabilities ChatGPT may use. Browser Automation is powered by the Playwright CLI dependency that ships with CodeLocal; if enabled, CodeLocal prepares its managed browser automatically before starting the runtime. Computer Use remains a separate opt-in capability.

The Go runtime then pairs this machine if needed, syncs authorized workspaces, and waits for ChatGPT. One machine runs one runtime; multiple workspaces activate lazily inside it.

Use ` + "`codelocal status`" + ` to inspect it and ` + "`codelocal stop`" + ` to stop it.

The user only installs and starts ` + "`codelocal`" + `; Playwright is an internal dependency and does not require a separate global install command.
`
	must(os.WriteFile(filepath.Join(staging, "README.md"), []byte(readme), 0o600))
	entries, _ := os.ReadDir(filepath.Join(staging, "bin", "native"))
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	fmt.Printf("staged CodeLocal %s (%s) on %s/%s\n", manifest.Version, strings.Join(names, ", "), runtime.GOOS, runtime.GOARCH)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
