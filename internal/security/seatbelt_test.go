package security

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// The text-level rules in Classify are a heuristic. An interpreter defeats them
// by assembling the sensitive path at runtime, so the reported symptom is not a
// missing rule but a class of bypass that no command-string match can close.
// These cases pin the kernel-boundary profile that does close it.
func TestSeatbeltProfileDeniesCredentialStores(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("seatbelt is macOS-only")
	}
	profile := SeatbeltProfile()
	if profile == "" {
		t.Skipf("seatbelt unavailable: %s", SandboxUnavailableReason())
	}
	for _, want := range []string{
		".codex/auth.json",
		".codex/device-credential.json",
		".ssh/",
		"/Library/Keychains/",
		`/\.env$`,
		"file-link",
	} {
		if !strings.Contains(profile, want) {
			t.Errorf("profile must mention %q:\n%s", want, profile)
		}
	}
}

// A seatbelt profile is only useful if it actually stops the runtime-built
// path. This drives the real sandbox helper so a profile that merely looks
// right cannot pass.
func TestSeatbeltBlocksRuntimeBuiltCredentialPath(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("seatbelt is macOS-only")
	}
	helper := SeatbeltPath()
	profile := SeatbeltProfile()
	if helper == "" || profile == "" {
		t.Skipf("seatbelt unavailable: %s", SandboxUnavailableReason())
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}
	// The path never appears literally: it is concatenated at runtime, which is
	// exactly what the command-string rules cannot see.
	script := `print(open('/Users/mac/.codex/'+'auth.json').read()[:8])`
	out, _ := exec.Command(helper, "-p", profile, python, "-c", script).CombinedOutput()
	if strings.Contains(string(out), "auth_mode") {
		t.Fatalf("sandbox leaked the codex token file: %s", out)
	}
	if !strings.Contains(string(out), "Operation not permitted") {
		t.Fatalf("expected a kernel denial, got: %s", out)
	}
}

// The boundary has to leave ordinary development work alone, otherwise it gets
// switched off and stops protecting anything.
func TestSeatbeltAllowsOrdinaryDevelopmentCommands(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("seatbelt is macOS-only")
	}
	helper := SeatbeltPath()
	profile := SeatbeltProfile()
	if helper == "" || profile == "" {
		t.Skipf("seatbelt unavailable: %s", SandboxUnavailableReason())
	}
	for _, command := range []string{"echo seatbelt-ok", "pwd", "git --version"} {
		out, err := exec.Command(helper, "-p", profile, "/bin/sh", "-c", command).CombinedOutput()
		if err != nil {
			t.Errorf("ordinary command %q was blocked: %v: %s", command, err, out)
		}
	}
}

// Regression: the text rules were patched once already, and a runtime-built
// path walked straight through them. A string-matching fix cannot be trusted
// for this class, so the kernel profile is exercised against each bypass shape
// that was actually observed reaching credential bytes.
func TestSeatbeltBlocksObservedBypassShapes(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("seatbelt is macOS-only")
	}
	helper := SeatbeltPath()
	profile := SeatbeltProfile()
	if helper == "" || profile == "" {
		t.Skipf("seatbelt unavailable: %s", SandboxUnavailableReason())
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}
	home, _ := os.UserHomeDir()
	secret := home + "/.codex/auth.json"

	cases := []struct {
		name string
		argv []string
	}{
		{"concatenated path", []string{python, "-c", `print(open('` + home + `/.codex/'+'auth.json').read()[:8])`}},
		{"chr-built path", []string{python, "-c", `import builtins;builtins.print(open(''.join(chr(c) for c in [47,85,115,101,114,115])).read()[:1])`}},
		{"glob-expanded path", []string{"/bin/sh", "-c", "cat " + home + "/.codex/auth*"}},
		{"copy out", []string{"/bin/cp", secret, "/tmp/seatbelt-copy.json"}},
	}
	for _, tc := range cases {
		argv := append([]string{"-p", profile}, tc.argv...)
		out, _ := exec.Command(helper, argv...).CombinedOutput()
		if strings.Contains(string(out), "auth_mode") {
			t.Errorf("%s leaked credentials: %s", tc.name, out)
		}
	}
	if _, err := exec.Command(helper, "-p", profile, "/bin/ln", "-f", secret, "/tmp/seatbelt-link.json").CombinedOutput(); err == nil {
		if data, readErr := os.ReadFile("/tmp/seatbelt-link.json"); readErr == nil && strings.Contains(string(data), "auth_mode") {
			t.Errorf("hardlink to a denied file was created and read")
		}
	}
}

// A committed dotenv template must stay readable. The policy already allowed
// ".env.example" by path; the bare-basename rule used by shell readers did not,
// so `cat .env.example` was refused even though reading the same file through
// the read tool succeeded.
func TestDotenvTemplatesStayReadableThroughShellReaders(t *testing.T) {
	root := "/Users/mac/Desktop/automatic-iron-machine-main"
	ctx := Context{WorkspaceRoot: root, CWD: root}
	for _, command := range []string{
		"cat .env.example",
		"cat .env.sample",
		"head -1 .env.template",
		"cat .env.example | head -5",
	} {
		decision := Classify(command, NetworkApproval, ctx)
		if decision.Blocked {
			t.Errorf("dotenv template must stay readable: %s (%v)", command, decision.MatchedRules)
		}
	}
	// The real dotenv file stays refused.
	for _, command := range []string{"cat .env", "cat .env.local", "cat .env.production"} {
		decision := Classify(command, NetworkApproval, ctx)
		if !decision.Blocked {
			t.Errorf("dotenv credential file must stay blocked: %s", command)
		}
	}
}
