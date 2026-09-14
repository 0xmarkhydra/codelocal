package security

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeScript(t *testing.T, root, name, body string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

// The regression this file exists for: a clean command line that reaches
// credential material through the body of a workspace script.
func TestScriptBodyCannotReachBrowserProfile(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "read.py", `print(open(".ironize_browser/profile/Default/Cookies").read())`)

	d := Classify("python3 read.py", NetworkApproval, Context{WorkspaceRoot: root, CWD: root})
	if !d.Blocked {
		t.Fatalf("script reading a browser profile must be blocked, got %+v", d)
	}
	if !strings.Contains(d.Reason, "sensitive script content") {
		t.Fatalf("reason should name script content, got %q", d.Reason)
	}
}

func TestScriptBodyCannotReachSSHKeys(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "tools/dump.mjs", `import fs from 'node:fs';console.log(fs.readFileSync('/Users/mac/.ssh/id_rsa'))`)

	d := Classify("node tools/dump.mjs", NetworkApproval, Context{WorkspaceRoot: root, CWD: root})
	if !d.Blocked {
		t.Fatalf("script reading ~/.ssh must be blocked, got %+v", d)
	}
}

// Chained commands must not become a bypass.
func TestChainedScriptStillBlocked(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "sub/read.py", `open("../../.ironize_browser/profile/Default/Cookies")`)

	d := Classify("cd sub && python3 read.py", NetworkApproval, Context{WorkspaceRoot: root, CWD: root})
	if !d.Blocked {
		t.Fatalf("chained script must be blocked, got %+v", d)
	}
}

// Shebang scripts without an extension are executable scripts too.
func TestShebangScriptIsInspected(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "grab", "#!/usr/bin/env python3\nprint(open('.ironize_browser/profile/Default/Cookies').read())\n")

	d := Classify("./grab", NetworkApproval, Context{WorkspaceRoot: root, CWD: root})
	if !d.Blocked {
		t.Fatalf("shebang script must be inspected, got %+v", d)
	}
}

// Ordinary development must keep working. These are the workflows the guard
// would be useless if it broke.
func TestOrdinaryProjectScriptsStayAllowed(t *testing.T) {
	cases := []struct {
		name string
		body string
		cmd  string
	}{
		{"dotenv", "require('dotenv').config();\n// reads the project's own .env\n", "node app.js"},
		{"cookies-rest-client", "const c = await fetch(url, {headers:{cookie: token}});\n", "node api.js"},
		{"history-module", "export const history = [];\n", "npx vitest run history.test.ts"},
		{"grep-docs", "print('cookies policy docs')\n", "python3 docs.py"},
		{"build", "console.log('building')\n", "node build.js"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeScript(t, root, filepath.Base(strings.Fields(tc.cmd)[1]), tc.body)
			d := Classify(tc.cmd, NetworkApproval, Context{WorkspaceRoot: root, CWD: root})
			if d.Blocked {
				t.Fatalf("%s must stay allowed, got reason %q", tc.cmd, d.Reason)
			}
		})
	}
}

// A credential store name alone is not enough: it must look like a browser
// profile, otherwise a project file naming "Cookies" becomes unreadable.
func TestBrowserStoreNameNeedsProfileShape(t *testing.T) {
	if rules := ScriptCredentialRules(`const path = 'src/fixtures/Cookies';`); len(rules) != 0 {
		t.Fatalf("bare fixture path should not trip: %v", rules)
	}
	if rules := ScriptCredentialRules(`open('.ironize_browser/profile/Default/Cookies')`); len(rules) == 0 {
		t.Fatal("real browser profile path must trip")
	}
	if rules := ScriptCredentialRules(`open('/Users/mac/Desktop/Veo Tool/data/chrome-profile/Default/Cookies')`); len(rules) == 0 {
		t.Fatal("chrome-profile path must trip")
	}
}

// The scan must not read something enormous as part of a policy check.
func TestOversizedScriptIsSkippedNotRead(t *testing.T) {
	root := t.TempDir()
	big := strings.Repeat("// padding\n", maxScriptInspectionBytes/8)
	writeScript(t, root, "big.js", big)
	d := Classify("node big.js", NetworkApproval, Context{WorkspaceRoot: root, CWD: root})
	if d.Blocked {
		t.Fatalf("size alone must not block, got %+v", d)
	}
}

// Wrapper indirection is the most common way a reviewer is fooled: the command
// line looks like `bash run.sh`, and the credential path is one file deeper.
func TestWrapperIndirectionIsBlocked(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "read.py", `open(".ironize_browser/profile/Default/Cookies")`)
	writeScript(t, root, "run.sh", "python3 read.py\n")

	for _, command := range []string{
		`sh -c "python3 read.py"`,
		`bash -c 'python3 read.py'`,
		"bash run.sh",
		"sh run.sh",
	} {
		d := Classify(command, NetworkApproval, Context{WorkspaceRoot: root, CWD: root})
		if !d.Blocked {
			t.Fatalf("%q must be blocked, got reason %q", command, d.Reason)
		}
	}
}

// A legitimate wrapper chain must still run: run.sh -> build.sh -> codegen.py,
// where nothing touches credential material.
func TestLegitimateWrapperChainStaysAllowed(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "codegen.py", "print('generating')\n")
	writeScript(t, root, "build.sh", "python3 codegen.py\n")
	writeScript(t, root, "run.sh", "bash build.sh\n")

	d := Classify("bash run.sh", NetworkApproval, Context{WorkspaceRoot: root, CWD: root})
	if d.Blocked {
		t.Fatalf("clean wrapper chain must stay allowed, got %q", d.Reason)
	}
}

// A script that references itself must terminate.
func TestSelfReferencingScriptTerminates(t *testing.T) {
	root := t.TempDir()
	writeScript(t, root, "loop.sh", "bash loop.sh\n")
	done := make(chan Decision, 1)
	go func() {
		done <- Classify("bash loop.sh", NetworkApproval, Context{WorkspaceRoot: root, CWD: root})
	}()
	select {
	case d := <-done:
		if d.Blocked {
			t.Fatalf("no credential material, should not block: %q", d.Reason)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("script inspection did not terminate")
	}
}

// Scripts outside the workspace are the explicit-path rule's job; they must not
// silently become readable by being named on a command line.
func TestScriptOutsideWorkspaceStillEscapeBlocked(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeScript(t, outside, "read.py", "print(1)\n")
	d := Classify("python3 "+filepath.Join(outside, "read.py"), NetworkApproval, Context{WorkspaceRoot: root, CWD: root})
	if !d.Blocked {
		t.Fatalf("out-of-workspace script must be blocked, got %+v", d)
	}
}
