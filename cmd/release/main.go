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
	GOOS       string
	GOARCH     string
	File       string
	HelperFile string
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
	must(os.MkdirAll(filepath.Join(staging, "bin", "helpers"), 0o755))
	targets := []target{
		{"darwin", "arm64", "codelocal-darwin-arm64", "computer-darwin-arm64"},
		{"darwin", "amd64", "codelocal-darwin-x64", "computer-darwin-amd64"},
		{"linux", "arm64", "codelocal-linux-arm64", "computer-linux-arm64"},
		{"linux", "amd64", "codelocal-linux-x64", "computer-linux-amd64"},
		{"windows", "amd64", "codelocal-win32-x64.exe", "computer-windows-amd64.exe"},
		{"windows", "arm64", "codelocal-win32-arm64.exe", "computer-windows-arm64.exe"},
	}
	for _, t := range targets {
		fmt.Printf("building %s/%s\n", t.GOOS, t.GOARCH)
		env := append(os.Environ(), "CGO_ENABLED=0", "GOOS="+t.GOOS, "GOARCH="+t.GOARCH)
		core := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", filepath.Join(staging, "bin", "native", t.File), "./cmd/codelocal")
		core.Dir = root
		core.Env = env
		core.Stdout = os.Stdout
		core.Stderr = os.Stderr
		must(core.Run())

		helper := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", filepath.Join(staging, "bin", "helpers", t.HelperFile), "./cmd/computerhelper")
		helper.Dir = root
		helper.Env = env
		helper.Stdout = os.Stdout
		helper.Stderr = os.Stderr
		must(helper.Run())
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
const helperFiles = {
  'darwin-arm64': 'computer-darwin-arm64',
  'darwin-x64': 'computer-darwin-amd64',
  'linux-arm64': 'computer-linux-arm64',
  'linux-x64': 'computer-linux-amd64',
  'win32-x64': 'computer-windows-amd64.exe',
  'win32-arm64': 'computer-windows-arm64.exe'
};
const file = files[key];
const helperFile = helperFiles[key];
if (!file || !helperFile) {
  console.error('CodeLocal does not have native binaries for ' + key + '.');
  process.exit(1);
}
const packageRoot = path.resolve(__dirname, '..');
const playwrightCli = path.join(packageRoot, 'node_modules', '.bin', process.platform === 'win32' ? 'playwright-cli.cmd' : 'playwright-cli');
const binary = path.join(__dirname, 'native', file);
const computerHelper = path.join(__dirname, 'helpers', helperFile);
const env = {
  ...process.env,
  CODELOCAL_PACKAGE_ROOT: packageRoot,
  CODELOCAL_PLAYWRIGHT_CLI: process.env.CODELOCAL_PLAYWRIGHT_CLI || playwrightCli,
  CODELOCAL_COMPUTER_HELPER: process.env.CODELOCAL_COMPUTER_HELPER || computerHelper
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
		"description": "Universal MCP runtime and durable Project Brain for AI coding clients and agents.",
		"license":     "UNLICENSED",
		"repository": map[string]string{
			"type": "git",
			"url":  "https://github.com/0xmarkhydra/codex-mcp.git",
		},
		"bin":     map[string]string{"codelocal": "bin/codelocal.js"},
		"files":   []string{"bin/", "README.md"},
		"engines": map[string]string{"node": ">=20"},
		"dependencies": map[string]string{
			"@playwright/cli":        "0.1.17",
			"@mobilenext/mobile-mcp": "1.0.2",
		},
		"keywords": []string{"chatgpt", "mcp", "coding", "local", "go", "playwright", "browser-automation", "computer-use"},
	}
	publicRaw, _ := json.MarshalIndent(public, "", "  ")
	must(os.WriteFile(filepath.Join(staging, "package.json"), append(publicRaw, '\n'), 0o600))
	releaseChannel := strings.ToLower(strings.TrimSpace(os.Getenv("CODELOCAL_RELEASE_CHANNEL")))
	installCommand := "npm i -g codelocal"
	if releaseChannel == "beta" {
		installCommand += "@beta"
	}
	readme := `# CodeLocal

Universal MCP runtime and durable Project Brain for AI coding clients and agents.

Website: [https://codelocal.cloud](https://codelocal.cloud/)

## Install

` + "```bash\n" + installCommand + "\n" + "```\n\n" + `## Authorize a project

` + "```bash\n" + `cd /path/to/project
codelocal .
` + "```\n\n" + "`codelocal .` only authorizes that folder locally. It does not pair the machine or connect to CodeLocal Cloud.\n\n" + `## Start CodeLocal

` + "```bash\n" + `codelocal
` + "```\n\n" + `On first start, CodeLocal asks which local capabilities connected MCP clients may use. Browser Automation lets a compatible AI client inspect and interact with websites in an isolated session; if enabled, CodeLocal downloads one managed Chromium browser and shows the install progress before starting. Coding works without this optional download. Computer Use remains a separate opt-in capability and includes both the native desktop helper and a managed Mobile MCP backend inside the CodeLocal package. Mobile automation becomes active only when a simulator, emulator or authorized real device is available; Android targets require platform tools/adb and iOS targets require the relevant Xcode/device tooling.

The Go runtime then pairs this machine if needed, syncs authorized workspaces, and waits for authenticated MCP clients. One machine runs one runtime; multiple workspaces activate lazily inside it.

Use ` + "`codelocal status`" + ` to inspect it and ` + "`codelocal stop`" + ` to stop it.

Use ` + "`codelocal setup`" + ` to change either choice later and ` + "`codelocal doctor`" + ` to inspect Browser/Computer readiness. The user only installs and starts ` + "`codelocal`" + `; Playwright and native Computer Use helpers are internal package details and do not require separate install commands.

## Reset or uninstall

Return CodeLocal to first-time setup while keeping the CLI installed:

` + "```bash\n" + `codelocal reset --all
` + "```\n\n" + `This stops the runtime and removes the local login, workspace grants, approvals, indexes, history, managed Chromium browser and Browser/Computer choices. Run ` + "`codelocal`" + ` afterward to start the onboarding flow again.

Remove both the local data and the globally installed npm package:

` + "```bash\n" + `codelocal uninstall --all
` + "```\n\n" + `Both commands ask for confirmation. For intentional non-interactive cleanup only, add ` + "`--yes`" + `:

` + "```bash\n" + `codelocal reset --all --yes
codelocal uninstall --all --yes
` + "```\n\n" + `Operating-system privacy permissions such as macOS Accessibility and Screen Recording remain controlled by the OS and are not silently changed.
`
	must(os.WriteFile(filepath.Join(staging, "README.md"), []byte(readme), 0o600))
	nativeEntries, _ := os.ReadDir(filepath.Join(staging, "bin", "native"))
	helperEntries, _ := os.ReadDir(filepath.Join(staging, "bin", "helpers"))
	names := []string{}
	for _, entry := range nativeEntries {
		names = append(names, "native/"+entry.Name())
	}
	for _, entry := range helperEntries {
		names = append(names, "helpers/"+entry.Name())
	}
	sort.Strings(names)
	fmt.Printf("staged CodeLocal %s (%s) on %s/%s\n", manifest.Version, strings.Join(names, ", "), runtime.GOOS, runtime.GOARCH)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
