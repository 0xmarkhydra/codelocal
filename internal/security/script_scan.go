package security

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Script inspection closes the gap left by command-line-only policy.
//
// The path and credential rules in policy.go only ever see the text of the
// command. That is enough for `cat .ironize_browser/profile/Default/Cookies`,
// because the credential path is literally on the command line, but it fails
// for the far more natural attack shape:
//
//	echo 'shutil.copy(".ironize_browser/profile/Default/Cookies", "/tmp/x")' > read.py
//	python3 read.py          # command line is clean, credential path is not
//
// Since a workspace that contains a browser profile is transitively equivalent
// to "the user's logged-in accounts", the agent must not be able to reach that
// profile through a helper script either. When script inspection is enabled the
// interpreter arguments are resolved inside the workspace, the file text is
// read, and the same credential markers are applied to the contents.

// maxScriptInspectionBytes bounds how much text is scanned so that a large
// generated artefact (a bundled JS build, a vendored dependency) cannot turn a
// policy check into a multi-megabyte read. Anything larger is skipped rather
// than blocked, because size is not evidence of credential access.
const maxScriptInspectionBytes = 256 * 1024

// inspectableScriptExtensions are the file types whose text is scanned. Only
// files an interpreter would execute are listed: a JSON fixture or a CSV row
// that happens to contain the word "Cookies" stays readable.
var inspectableScriptExtensions = map[string]struct{}{
	".py": {}, ".pyw": {}, ".js": {}, ".mjs": {}, ".cjs": {}, ".ts": {}, ".tsx": {},
	".jsx": {}, ".sh": {}, ".bash": {}, ".zsh": {}, ".ksh": {}, ".fish": {},
	".rb": {}, ".pl": {}, ".pm": {}, ".php": {}, ".lua": {}, ".r": {}, ".jl": {},
	".swift": {}, ".go": {}, ".java": {}, ".kt": {}, ".cs": {}, ".ps1": {}, ".bat": {},
	".cmd": {}, ".awk": {}, ".sed": {},
}

// scriptCredentialMarkers are the high-confidence credential fragments that
// identify material belonging to the *user's accounts* rather than to the
// project being edited. They are matched against the normalized script text,
// so both "/Users/mac/.ssh/id_rsa" and ".ssh/id_rsa" are caught.
//
// The list is deliberately narrow. Markers such as ".env" are NOT included:
// loading the project's own dotenv file is a normal part of running an app,
// and blocking it would break ordinary `node app.js` / `python main.py` work
// without protecting anything that the command-line policy does not already
// cover. Only account-level credential stores belong here.
var scriptCredentialMarkers = []string{
	"/.ssh/", "/.aws/", "/.gnupg/", "/.gcloud/", "/.azure/",
	"/chrome-profile/", "/.ironize_browser/",
	"/.codex/auth.json", "/.codex/credentials.json", "/.codex/device-credential.json",
}

// browserProfileDir matches a directory component that only appears in a real
// browser profile tree.
var browserProfileDir = regexp.MustCompile(`/(chrome-profile|\.ironize_browser|user data|libraries/application support)[/. ]`)

// browserProfileSeparator matches the per-profile subdirectory Chromium uses.
var browserProfileSeparator = regexp.MustCompile(`/(default|profile ?[0-9]+|guest profile)/`)

// browserCredentialBase matches a credential store by exact basename. The
// trailing separator is required, so a project file named "cookies.js" or
// "history.md" does not match while "Cookies" and "Login Data" do.
var browserCredentialBase = regexp.MustCompile(`/(cookies|login ?data|web ?data|logins\.json|signons\.sqlite|cert9\.db|key4\.db|local state)/`)

// ScriptCredentialRules reports the credential-material markers found in the
// *text* of a script the command would execute. An empty result means the
// command never names account credential material through a script body.
func ScriptCredentialRules(content string) []string {
	if content == "" {
		return nil
	}
	scan := "/" + credentialScanForm(content) + "/"
	var rules []string
	seen := map[string]bool{}
	add := func(rule string) {
		if seen[rule] {
			return
		}
		seen[rule] = true
		rules = append(rules, rule)
	}
	for _, marker := range scriptCredentialMarkers {
		if strings.Contains(scan, marker) {
			add("script content references credential material: " + strings.Trim(marker, "/"))
		}
	}
	// A browser credential store is only reachable through a profile tree, so
	// the store name alone is not enough. Requiring the profile shape keeps
	// project code that merely mentions a "Cookies" path segment readable.
	if browserCredentialBase.MatchString(scan) &&
		(browserProfileDir.MatchString(scan) || browserProfileSeparator.MatchString(scan)) {
		add("script content references a browser credential store")
	}
	return rules
}

// scriptInspectionDepth bounds transitive script resolution. A chain such as
// run.sh -> build.sh -> codegen.py is legitimate and must still be followed,
// while a self-referencing script must not spin forever.
const scriptInspectionDepth = 4

// sensitiveScriptInCommand resolves the interpreter arguments that point at
// files inside the workspace and returns the first credential rule their
// contents match. Files outside the workspace are already refused by the
// explicit-path rule, and files that are themselves credential material are
// already refused by the sensitive-path rule, so both are skipped here.
//
// Two indirections are followed, because both let a clean command line reach a
// credential path:
//
//	bash run.sh        -> run.sh body runs `python3 read.py`
//	sh -c "python3 x"  -> the inner command string is itself inspected
func sensitiveScriptInCommand(command string, ctx Context) string {
	return inspectScriptChain(command, ctx, 0, map[string]bool{})
}

func inspectScriptChain(command string, ctx Context, depth int, visited map[string]bool) string {
	if depth > scriptInspectionDepth {
		return ""
	}
	for _, path := range executedScriptPaths(command, ctx) {
		if visited[path] {
			continue
		}
		visited[path] = true
		content, ok := readScriptHead(path)
		if !ok {
			continue
		}
		if rules := ScriptCredentialRules(content); len(rules) > 0 {
			return path + " (" + rules[0] + ")"
		}
		// The script may itself invoke another in-workspace script or wrap an
		// inline interpreter command. Resolve those from the script's own
		// directory, the way the shell would.
		inner := ctx
		inner.CWD = filepath.Dir(path)
		for _, line := range scriptCommandLines(content) {
			if hit := inspectScriptChain(line, inner, depth+1, visited); hit != "" {
				return path + " -> " + hit
			}
		}
	}
	return ""
}

// scriptCommandLines extracts the candidate commands a script body runs. It is
// intentionally permissive: quoting, pipes and command substitution are not
// modelled, because a false negative here is a security hole while a false
// positive only produces a more conservative decision.
func scriptCommandLines(content string) []string {
	var out []string
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		if len(out) >= 64 {
			break
		}
		out = append(out, line)
	}
	return out
}

// inlineInterpreterCommand returns the command string passed to `sh -c`,
// `bash -c`, `node -e`, `python3 -c` and friends, so that the inner command is
// classified with the same rules instead of being written off as an opaque
// approval. The outer rule in Classify still applies; this only ensures the
// inner text is not a blind spot.
func inlineInterpreterCommand(command string, exec string, args []string) string {
	if !isInterpreter(exec) || len(args) < 2 {
		return ""
	}
	if !contains([]string{"-e", "-c", "-command", "/c", "--eval", "--command"}, strings.ToLower(args[0])) {
		return ""
	}
	return strings.TrimSpace(strings.Trim(args[1], `"'`))
}

// executedScriptPaths extracts the in-workspace script files a command names.
func executedScriptPaths(command string, ctx Context) []string {
	if ctx.WorkspaceRoot == "" {
		return nil
	}
	parsed, ok := parseCommand(command)
	if !ok || parsed == nil || parsed.CommandIndex >= len(parsed.Words) {
		return nil
	}
	root, err := filepath.Abs(ctx.WorkspaceRoot)
	if err != nil {
		return nil
	}
	cwd := ctx.CWD
	if cwd == "" {
		cwd = ctx.WorkspaceRoot
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}
	tokens := append([]string{parsed.Words[parsed.CommandIndex]}, parsed.Args...)
	var out []string
	seen := map[string]bool{}
	for _, token := range tokens {
		value := scriptToken(token)
		if value == "" {
			continue
		}
		// A path in another home directory is resolved by the explicit-path
		// rule, not here.
		if value == "~" || strings.HasPrefix(value, "~/") {
			continue
		}
		if crossPlatformAbsolute(value) && !filepath.IsAbs(value) {
			continue
		}
		expanded := value
		if !filepath.IsAbs(expanded) {
			expanded = filepath.Join(cwd, expanded)
		}
		abs, err := filepath.Abs(expanded)
		if err != nil || !inside(root, abs) {
			continue
		}
		// Credential files and credential directories are refused on their own;
		// re-reading them here would only duplicate the existing rule.
		if IsSensitivePath(abs) {
			continue
		}
		if !inspectableScript(abs) {
			continue
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		out = append(out, abs)
	}
	return out
}

// scriptToken reduces a raw interpreter argument to a candidate file path.
// Unlike pathCandidate it also accepts plain relative paths ("read.py"), since
// scripts are commonly invoked from the workspace root.
func scriptToken(token string) string {
	raw := strings.Trim(token, `"'`+",;|&()<>")
	if idx := strings.Index(raw, "="); idx > 0 && strings.HasPrefix(raw, "-") {
		raw = raw[idx+1:]
	}
	raw = strings.Trim(raw, `"'`)
	if raw == "" || strings.HasPrefix(raw, "-") {
		return ""
	}
	lower := strings.ToLower(raw)
	for _, prefix := range []string{"http://", "https://", "ws://", "wss://", "ssh://", "git@"} {
		if strings.HasPrefix(lower, prefix) {
			return ""
		}
	}
	return raw
}

// inspectableScript reports whether a candidate is a script whose text should
// be scanned: a known interpreter extension, or an executable carrying a
// shebang.
func inspectableScript(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if info.Size() > maxScriptInspectionBytes {
		return false
	}
	if _, ok := inspectableScriptExtensions[strings.ToLower(filepath.Ext(path))]; ok {
		return true
	}
	if filepath.Ext(path) != "" {
		return false
	}
	head, ok := readScriptHead(path)
	if !ok {
		return false
	}
	return shebangInterpreter(head) != ""
}

// shebangInterpreter returns the interpreter an extensionless script declares
// on its first line, resolving the usual indirection:
//
//	#!/usr/bin/env python3   -> python3
//	#!/usr/bin/python3 -u    -> python3
//	#!/bin/sh                -> sh
//
// A `#!` alone is not enough: `#!/usr/bin/env -S node --flag` must still be
// recognised as an interpreted script rather than an arbitrary binary.
func shebangInterpreter(head string) string {
	first := head
	if idx := strings.IndexByte(first, '\n'); idx >= 0 {
		first = first[:idx]
	}
	first = strings.TrimSpace(strings.TrimSuffix(first, "\r"))
	if !strings.HasPrefix(first, "#!") {
		return ""
	}
	words, ok := shellWords(strings.TrimSpace(strings.TrimPrefix(first, "#!")))
	if !ok || len(words) == 0 {
		return ""
	}
	index := 0
	if base := strings.ToLower(filepath.Base(words[index])); base == "env" {
		// Skip env and its flags, then the first non-flag word is the real
		// interpreter. `-S` may be followed by a command string.
		index++
		for index < len(words) {
			word := words[index]
			if word == "-S" || word == "--split-string" {
				index++
				continue
			}
			if strings.HasPrefix(word, "-") {
				index++
				continue
			}
			break
		}
	}
	if index >= len(words) {
		return ""
	}
	candidate := strings.ToLower(filepath.Base(words[index]))
	if !isInterpreter(candidate) {
		return ""
	}
	return candidate
}

// readScriptHead reads at most maxScriptInspectionBytes of a script.
func readScriptHead(path string) (string, bool) {
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer func() { _ = file.Close() }()
	buf := make([]byte, maxScriptInspectionBytes)
	n, err := file.Read(buf)
	if n == 0 && err != nil {
		return "", false
	}
	head := buf[:n]
	// Reject binary payloads: an unreadable blob is not a script, and scanning
	// it only produces noise.
	if index := strings.IndexByte(string(head), 0); index >= 0 {
		return "", false
	}
	return string(head), true
}
