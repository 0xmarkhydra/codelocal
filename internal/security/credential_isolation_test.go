package security

import "testing"

// Inline interpreter one-liners used to reach credential stores that the
// path-level policy already blocks. The quoted argument glues the marker to
// punctuation ("copy('.ironize_browser/profile/Default/Cookies','/tmp/x')"),
// so a plain "/"-split never matched. These cases pin the normalized scan.
func TestInlineInterpreterCannotReachCredentialStores(t *testing.T) {
	ctx := Context{WorkspaceRoot: "/Users/mac/Desktop", CWD: "/Users/mac/Desktop"}
	cases := []string{
		`python3 -c "import shutil;shutil.copy('.ironize_browser/profile/Default/Cookies','/tmp/p.db')"`,
		`node -e "require('fs').copyFileSync('.ironize_browser/profile/Default/Cookies','/tmp/p.db')"`,
		`python3 -c "open('/tmp/p.txt','w').write(open('.ironize_browser/profile/Default/Cookies','rb').read(32).hex())"`,
		`perl -e 'open(F, "Veo Tool/data/chrome-profile/Default/Cookies")'`,
		`python3 -c "print(open('/Users/mac/.codex/auth.json').read()[:20])"`,
		`python3 -c "print(open('~/.codex/device-credential.json').read())"`,
		`python3 -c "print(open('Veo Tool/data/chrome-profile/Default/Login Data').read())"`,
	}
	for _, command := range cases {
		decision := Classify(command, NetworkApproval, ctx)
		if !decision.Blocked {
			t.Errorf("expected BLOCKED, got %s (%v): %s", decision.RiskLevel, decision.MatchedRules, command)
		}
	}
}

// The same normalization must not swallow ordinary project work.
func TestProjectWorkStaysUnblocked(t *testing.T) {
	ctx := Context{WorkspaceRoot: "/Users/mac/Desktop", CWD: "/Users/mac/Desktop"}
	cases := []string{
		`python3 scripts/migrate-products.py`,
		`python3 -c "print(open('Shopify Automatic Ironing Machine/config/settings_schema.json').read()[:40])"`,
		`python3 -c "import os;print(len(os.listdir('sections')))"`,
		`node -e "console.log(require('fs').statSync('package.json').size)"`,
		`git status --short`,
		`shopify theme check`,
	}
	for _, command := range cases {
		decision := Classify(command, NetworkApproval, ctx)
		if decision.Blocked {
			t.Errorf("unexpected BLOCKED (%v): %s", decision.MatchedRules, command)
		}
	}
}

// ~/.codex holds client account material; only the credential files are
// sensitive, the directory itself must stay usable.
func TestCodexCredentialFilesAreSensitive(t *testing.T) {
	blocked := []string{
		".codex/auth.json",
		"/Users/mac/.codex/auth.json",
		".codex/device-credential.json",
	}
	for _, path := range blocked {
		if !IsSensitivePath(path) {
			t.Errorf("expected sensitive: %s", path)
		}
	}
	allowed := []string{
		"config.toml",
		".codex/AGENTS.md",
		".codex/skills/foo/SKILL.md",
	}
	for _, path := range allowed {
		if IsSensitivePath(path) {
			t.Errorf("expected readable: %s", path)
		}
	}
}
