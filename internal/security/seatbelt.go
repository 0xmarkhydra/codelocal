package security

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Sandboxing note.
//
// The command rules in Classify() are a text-level heuristic: they inspect the
// command string. Any interpreter can defeat a text heuristic by building a
// path at runtime, e.g.
//
//	python3 -c "print(open('/Users/mac/.codex/'+'auth.json').read())"
//
// Nothing that matches on the raw command text can close that class of bypass,
// because the credential path never appears literally. The only durable fix is
// to enforce the same boundary in the kernel, where the path is finally
// resolved.
//
// On macOS that boundary is seatbelt (sandbox-exec). The profile below denies
// read, write and hardlink creation for the credential stores that the policy
// layer already classifies as sensitive. Because the kernel applies the rule
// after path resolution, symlinks, hardlinks, relative traversal and
// runtime-constructed strings are all covered by the same rule.

// sandboxEnvVar turns kernel sandboxing off. It exists as an explicit escape
// hatch for a workspace that genuinely needs a denied path; leaving it unset
// keeps the sandbox on.
const sandboxEnvVar = "CODELOCAL_SANDBOX"

// SandboxDisabled reports whether the operator turned kernel sandboxing off.
func SandboxDisabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(sandboxEnvVar))) {
	case "0", "false", "off", "no", "disabled":
		return true
	}
	return false
}

// SeatbeltPath returns the sandbox helper, or "" when kernel sandboxing cannot
// run on this host.
func SeatbeltPath() string {
	if runtime.GOOS != "darwin" || SandboxDisabled() {
		return ""
	}
	path, err := exec.LookPath("sandbox-exec")
	if err != nil {
		return ""
	}
	return path
}

// seatbeltLiteralPaths are exact credential files. They are denied wholesale
// (deny file-read* file-write*) because a shell command has no legitimate
// reason to touch them.
func seatbeltLiteralPaths(home string) []string {
	join := func(parts ...string) string { return filepath.Join(append([]string{home}, parts...)...) }
	return []string{
		// The AI client's own account material. Reaching this is equivalent to
		// stealing the operator's model account token.
		join(".codex", "auth.json"),
		join(".codex", "credentials.json"),
		join(".codex", "device-credential.json"),
		// Cloud/registry credential files.
		join(".aws", "credentials"),
		// ~/.config/gh/hosts.yml is deliberately left readable: on this host it
		// holds only the account name and protocol, and `gh` keeps the actual
		// token in the login keychain. Blocking it breaks the git credential
		// helper without protecting anything.
		join(".docker", "config.json"),
		join(".netrc"),
		join(".pypirc"),
		join(".git-credentials"),
	}
}

// seatbeltDirPaths are directories whose entire contents are credential
// material (key material, not project data).
func seatbeltDirPaths(home string) []string {
	join := func(parts ...string) string { return filepath.Join(append([]string{home}, parts...)...) }
	return []string{
		join(".gnupg"),
	}
}

// seatbeltRegexPaths mirror the sensitive-path rules in IsSensitivePath, but
// expressed as kernel rules. They catch credential stores that live inside an
// authorized workspace, where a literal path would have to be guessed.
func seatbeltRegexPaths() []string {
	return []string{
		// Rotated or backed-up copies of the credential files. A literal rule
		// misses auth.json.bak_<stamp>, and on this host those backups contain
		// a live access token.
		`/\.codex/(auth|credentials|device-credential)\.json(\.[^/]*)?$`,
		// SSH private key material (the socket directory stays usable).
		`/\.ssh/(id_[^/]*|[^/]*\.pem|[^/]*_key|[^/]*\.ppk)$`,
		// Browser profile credential stores. The profile directory may be
		// "Default", "Profile 1", "chrome-profile", or a tool-specific name.
		`/(Default|Profile [0-9]+|chrome-profile|\.ironize_browser)/(Cookies|Cookies-journal|Login Data|Login Data For Account|Web Data|Affiliation Database|Network/Cookies)$`,
		// Chromium's profile root also holds the encryption key for the
		// cookie/login databases and the live session cache.
		`/(chrome-profile|\.ironize_browser)/.+/(Local State|Local Storage/.+)$`,
		// The login keychain is where the CodeLocal MCP OAuth token, the Codex
		// account credential and the browser-safe-storage key live. The system
		// keychain holds preinstalled trust roots and is left readable so
		// signed tooling keeps working.
		`/Library/Keychains/login\.keychain(-db)?$`,
		`/Library/Application Support/Google/Chrome/.+/(Cookies|Login Data|Web Data|Local State)$`,
	}
}

// seatbeltAllowedPaths re-allow files that the deny rules above would other-
// wise over-block. In seatbelt the last matching rule wins, so these are
// appended after the denies.
func seatbeltAllowedPaths() []string {
	return []string{
		`/\.env\.(example|sample|template)$`,
	}
}

// SeatbeltProfile renders the seatbelt (SBPL) profile used for every
// agent-spawned shell command. It returns "" when kernel sandboxing is
// unavailable, in which case the caller runs unsandboxed and records that fact.
func SeatbeltProfile(workspaceRoots ...string) string {
	if SeatbeltPath() == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("(version 1)\n")
	b.WriteString("(allow default)\n")
	// Credential stores that the text policy already blocks. Denying write and
	// link alongside read stops a command from exfiltrating a copy and stops
	// the hardlink trick that would otherwise alias the file at an allowed path.
	b.WriteString("(deny file-read* file-write*\n")
	for _, path := range seatbeltLiteralPaths(home) {
		b.WriteString("  (literal " + sbplString(path) + ")\n")
	}
	for _, path := range seatbeltDirPaths(home) {
		b.WriteString("  (subpath " + sbplString(path) + ")\n")
	}
	for _, pattern := range seatbeltRegexPaths() {
		b.WriteString("  (regex #\"" + pattern + "\")\n")
	}
	// A dotenv file is read into the environment, so deny it wherever it sits.
	b.WriteString("  (regex #\"/\\.env$\")\n")
	b.WriteString("  (regex #\"/\\.env\\.[^/]+$\")\n")
	b.WriteString(")\n")
	b.WriteString("(deny file-link\n")
	for _, path := range seatbeltLiteralPaths(home) {
		b.WriteString("  (literal " + sbplString(path) + ")\n")
	}
	for _, pattern := range seatbeltRegexPaths() {
		b.WriteString("  (regex #\"" + pattern + "\")\n")
	}
	b.WriteString(")\n")
	// `security` is the CLI front-end for the keychain that holds the CodeLocal
	// and Codex OAuth tokens. The text policy blocks `security find|dump`, but
	// an interpreter can reach the same items without those subcommands.
	b.WriteString("(deny process-exec\n")
	b.WriteString("  (literal " + sbplString("/usr/bin/security") + ")\n")
	b.WriteString(")\n")
	b.WriteString("(allow file-read*\n")
	for _, pattern := range seatbeltAllowedPaths() {
		b.WriteString("  (regex #\"" + pattern + "\")\n")
	}
	// The workspace's own dotenv file, when the operator opted in. Seatbelt
	// applies the last matching rule, so this re-allows a path the blanket
	// deny above rejected.
	//
	// The pattern is anchored to the authorized workspace roots rather than
	// written as a bare "/\.env$": a bare rule would make every dotenv file
	// on the machine readable, including another project's or a backup in the
	// home directory. Here `node server.js` can load its own configuration
	// while the boundary around everyone else's stays intact.
	for _, root := range seatbeltDotenvRoots(workspaceRoots) {
		// The subdirectory part matters: a project's dotenv usually sits in the
		// project folder (workspace/zz-app/.env), not at the workspace root.
		// Anchoring to the root alone would only cover the shallow case.
		b.WriteString("  (regex #\"" + sbplRegexEscape(root) + "(/[^/]*)*/\\.env([^/]*)$\")\n")
	}
	b.WriteString(")\n")
	return b.String()
}

func sbplString(value string) string {
	return `"` + strings.ReplaceAll(value, `\`, `\\`) + `"`
}

// SandboxUnavailableReason explains why kernel sandboxing is off, for audit and
// for the process result metadata.
func SandboxUnavailableReason() string {
	if SandboxDisabled() {
		return "disabled by " + sandboxEnvVar
	}
	if runtime.GOOS != "darwin" {
		return "kernel sandboxing is only implemented for macOS seatbelt"
	}
	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		return "sandbox-exec not found on PATH"
	}
	if home, err := os.UserHomeDir(); err != nil || home == "" {
		return "home directory could not be resolved"
	}
	return ""
}

// seatbeltDotenvRoots lists the workspace roots whose dotenv files stay
// readable while the operator allows workspace-local dotenv access. The list
// is empty unless the allowance is on, so the rendered profile is unchanged
// for every other deployment.
func seatbeltDotenvRoots(candidates []string) []string {
	if !WorkspaceDotenvAllowed() {
		return nil
	}
	var roots []string
	seen := map[string]bool{}
	for _, raw := range candidates {
		root := strings.TrimSpace(raw)
		if root == "" || !filepath.IsAbs(root) {
			continue
		}
		root = strings.TrimRight(filepath.Clean(root), "/")
		if root == "" || root == "/" || seen[root] {
			continue
		}
		seen[root] = true
		roots = append(roots, root)
	}
	return roots
}

// sbplRegexEscape escapes a path so it can be embedded in an SBPL regex
// literal. Only regex metacharacters are escaped; slashes stay literal because
// SBPL regexes match resolved absolute paths.
func sbplRegexEscape(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch r {
		case '.', '+', '*', '?', '(', ')', '[', ']', '{', '}', '|', '^', '$', '\\':
			b.WriteRune('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
