package security

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The workspace's own dotenv file is configuration, not account credential
// material: a dev server cannot start without it. The allowance is opt-in and
// scoped to the workspace, and these tests pin both halves of that promise.
func TestWorkspaceDotenvAllowedOnlyWhenOptedIn(t *testing.T) {
	root := t.TempDir()

	t.Setenv(workspaceDotenvEnvVar, "")
	if !IsSensitivePath(".env") {
		t.Fatal(".env must stay sensitive while the allowance is off")
	}
	d := Classify("cat .env", NetworkApproval, Context{WorkspaceRoot: root, CWD: root})
	if !d.Blocked {
		t.Fatalf(".env read must be blocked while off, got %+v", d)
	}

	t.Setenv(workspaceDotenvEnvVar, "1")
	if IsSensitivePath(".env") {
		t.Fatal(".env inside the workspace must be readable once opted in")
	}
	d = Classify("cat .env", NetworkApproval, Context{WorkspaceRoot: root, CWD: root})
	if d.Blocked {
		t.Fatalf(".env read must be allowed once opted in, got %q", d.Reason)
	}
}

// The allowance must not leak outside the workspace. These are the shapes an
// escalation would take.
func TestDotenvAllowanceNeverEscapesWorkspace(t *testing.T) {
	t.Setenv(workspaceDotenvEnvVar, "1")
	root := t.TempDir()

	outside := []string{
		"/etc/.env",
		"/Users/someone-else/project/.env",
		"~/.env",
		"../.env",
		"../../secrets/.env",
		"sub/../../.env",
		"C:/.env",
	}
	for _, path := range outside {
		if !IsSensitivePath(path) {
			t.Fatalf("%q must stay sensitive", path)
		}
	}
	for _, command := range []string{
		"cat /etc/.env",
		"cat ~/.env",
		"cat ../.env",
		"cat ../.env.production",
		"head -5 ../../.env.local",
	} {
		d := Classify(command, NetworkApproval, Context{WorkspaceRoot: root, CWD: root})
		if !d.Blocked {
			t.Fatalf("%q must stay blocked, got %q", command, d.Reason)
		}
	}
}

// Real credential stores must be untouched by this change.
func TestDotenvAllowanceDoesNotRelaxOtherSecrets(t *testing.T) {
	t.Setenv(workspaceDotenvEnvVar, "1")
	root := t.TempDir()

	for _, path := range []string{".ssh/id_rsa", ".ironize_browser/profile/Default/Cookies", ".codex/auth.json", ".aws/credentials"} {
		if !IsSensitivePath(path) {
			t.Fatalf("%q must stay sensitive", path)
		}
	}
	for _, command := range []string{
		"cat .ssh/id_rsa",
		"cat .ironize_browser/profile/Default/Cookies",
		"cat .codex/auth.json",
		// A renamed copy of a real dotenv is still a live configuration file,
		// so the allowance must cover it only inside the workspace.
		"cat /tmp/.env.bak",
	} {
		d := Classify(command, NetworkApproval, Context{WorkspaceRoot: root, CWD: root})
		if !d.Blocked {
			t.Fatalf("%q must stay blocked, got %q", command, d.Reason)
		}
	}
}

// A committed dotenv template is documentation and stays readable either way.
func TestDotenvTemplatesAlwaysReadable(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{".env.example", ".env.sample", ".env.template"} {
		if IsSensitivePath(name) {
			t.Fatalf("%s must stay readable", name)
		}
		d := Classify("cat "+name, NetworkApproval, Context{WorkspaceRoot: root, CWD: root})
		if d.Blocked {
			t.Fatalf("cat %s must stay allowed, got %q", name, d.Reason)
		}
	}
}

// Seatbelt is the layer that actually enforces this for an interpreter. When
// the allowance is off the profile must not mention any workspace root; when
// it is on, the root must be anchored instead of a bare "/.env$" rule, which
// would expose every dotenv file on the machine.
func TestSeatbeltDotenvRuleIsAnchoredToWorkspace(t *testing.T) {
	if SeatbeltPath() == "" {
		t.Skip("seatbelt not available on this host")
	}
	t.Setenv(workspaceDotenvEnvVar, "")
	off := SeatbeltProfile("/Users/mac/project")
	if strings.Contains(off, "/Users/mac/project") {
		t.Fatal("profile must not mention a workspace root while the allowance is off")
	}

	t.Setenv(workspaceDotenvEnvVar, "1")
	on := SeatbeltProfile("/Users/mac/project")
	if !strings.Contains(on, "/Users/mac/project") {
		t.Fatal("profile must anchor the dotenv allowance to the workspace root")
	}
	// Without a root there is nothing to re-allow, so the blanket deny stands.
	noRoot := SeatbeltProfile()
	if strings.Contains(noRoot, "/.env([^/]*)$") {
		t.Fatal("no root means no blanket dotenv allow rule")
	}
	// Relative or non-absolute roots are ignored rather than escaping.
	for _, bad := range []string{"relative/path", ".", "..", "/"} {
		if got := seatbeltDotenvRoots([]string{bad}); len(got) != 0 {
			t.Fatalf("root %q must be rejected, got %v", bad, got)
		}
	}
}

// The pattern builder must not let a path break out of the SBPL regex literal.
func TestSBPLRegexEscapeNeutralisesMetacharacters(t *testing.T) {
	got := sbplRegexEscape("/Users/mac/my.project (copy)+")
	for _, meta := range []string{".", "(", ")", "+"} {
		if strings.Contains(got, meta) && !strings.Contains(got, `\`+meta) {
			t.Fatalf("metacharacter %q was not escaped in %q", meta, got)
		}
	}
	if !strings.HasPrefix(got, "/Users/mac/my") {
		t.Fatalf("plain path segments must survive: %q", got)
	}
}

// The engine renders the profile from the workspace it is about to run in.
func TestSeatbeltProfileAcceptsWorkspaceRoot(t *testing.T) {
	if SeatbeltPath() == "" {
		t.Skip("seatbelt not available on this host")
	}
	t.Setenv(workspaceDotenvEnvVar, "1")
	root := t.TempDir()
	profile := SeatbeltProfile(root)
	abs, _ := filepath.Abs(root)
	if !strings.Contains(profile, strings.TrimRight(abs, "/")) {
		t.Fatalf("profile must contain the workspace root %q", abs)
	}
	if !strings.Contains(profile, "(deny file-read* file-write*") {
		t.Fatal("profile must keep the credential deny block")
	}
	if !strings.Contains(profile, "(allow file-read*") {
		t.Fatal("profile must keep the trailing allow block")
	}
}

func TestWorkspaceDotenvAllowedParsesEnv(t *testing.T) {
	for _, value := range []string{"1", "true", "ON", "yes", "enabled", " true "} {
		t.Setenv(workspaceDotenvEnvVar, value)
		if !WorkspaceDotenvAllowed() {
			t.Fatalf("%q should enable the allowance", value)
		}
	}
	for _, value := range []string{"", "0", "false", "off", "no", "nope"} {
		t.Setenv(workspaceDotenvEnvVar, value)
		if WorkspaceDotenvAllowed() {
			t.Fatalf("%q should keep the allowance off", value)
		}
	}
}

func TestWorkspaceLocalDotenvRejectsThenAllows(t *testing.T) {
	allowed := []string{".env", ".env.local", "apps/web/.env", ".env.production"}
	for _, path := range allowed {
		if !workspaceLocalDotenv(path) {
			t.Fatalf("%q should count as workspace-local", path)
		}
	}
	rejected := []string{"", "/.env", "~/.env", "../.env", "a/../../.env", "/etc/.env"}
	for _, path := range rejected {
		if workspaceLocalDotenv(path) {
			t.Fatalf("%q must not count as workspace-local", path)
		}
	}
}

func TestDotenvAllowanceIsDocumentedInEnv(t *testing.T) {
	if workspaceDotenvEnvVar != "CODELOCAL_ALLOW_WORKSPACE_DOTENV" {
		t.Fatalf("env var name changed: %s", workspaceDotenvEnvVar)
	}
	if os.Getenv("CODELOCAL_ALLOW_WORKSPACE_DOTENV") == "0" &&
		WorkspaceDotenvAllowed() {
		t.Fatal("explicit 0 must disable")
	}
}

// Real projects keep their dotenv inside the project folder, not at the
// workspace root: workspace/zz-app/.env. An anchored rule that only covers the
// shallow case would silently stop covering the common one.
func TestSeatbeltDotenvCoversNestedProjectDirs(t *testing.T) {
	if SeatbeltPath() == "" {
		t.Skip("seatbelt not available on this host")
	}
	t.Setenv(workspaceDotenvEnvVar, "1")
	profile := SeatbeltProfile("/Users/mac/Documents/Codex")
	pattern := sbplRegexEscape("/Users/mac/Documents/Codex") + "(/[^/]*)*/\\.env([^/]*)$"
	if !strings.Contains(profile, pattern) {
		t.Fatalf("profile must cover nested project dirs, want %q", pattern)
	}
}

// The kernel rule, not the text rule, is what makes this real. Prove it by
// running an interpreter that builds the path at runtime.
func TestSeatbeltAllowsWorkspaceDotenvForInterpreter(t *testing.T) {
	if SeatbeltPath() == "" {
		t.Skip("seatbelt not available on this host")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app", ".env"), []byte("PORT=4321\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(workspaceDotenvEnvVar, "1")
	profile := SeatbeltProfile(root)

	// The rendered profile must be valid SBPL: a malformed profile makes
	// sandbox-exec refuse to start the command at all.
	cmd := exec.Command(SeatbeltPath(), "-p", profile, "/bin/sh", "-c", "cat app/.env")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sandboxed read of workspace .env failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "PORT=4321") {
		t.Fatalf("unexpected output: %q", out)
	}

	// And a dotenv outside the workspace still fails under the same profile.
	outside := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(outside, []byte("SECRET=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(SeatbeltPath(), "-p", profile, "/bin/sh", "-c", "cat "+outside)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("read of %s must fail under the profile, got %q", outside, out)
	}
}
