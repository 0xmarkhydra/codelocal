package process

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func usePortableTestShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Setenv("SHELL", "/bin/sh")
	}
}

func waitExited(t *testing.T, manager *Manager, processID string) Snapshot {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, err := manager.Snapshot(processID, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !snapshot.Running {
			return snapshot
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, _ = manager.Cancel(processID, "test timeout")
	t.Fatalf("process %s did not exit", processID)
	return Snapshot{}
}

func TestProcessManagerExecutesAndCleansRequestMapping(t *testing.T) {
	usePortableTestShell(t)
	root, err := filepath.Abs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(root, "test-workspace", nil, nil)
	started, err := manager.Start("go version", StartOptions{CWD: root, Timeout: 10 * time.Second, RequestID: "request-1"})
	if err != nil {
		t.Fatal(err)
	}
	if started.ExecutionMode != "host-policy" || started.CWD != "." {
		t.Fatalf("started=%#v", started)
	}
	finished := waitExited(t, manager, started.ProcessID)
	if finished.ExitCode == nil || *finished.ExitCode != 0 || !strings.Contains(finished.Stdout["text"].(string), "go version") {
		t.Fatalf("finished=%#v", finished)
	}
	cancelled := manager.CancelRequest("request-1", "late cancel")
	if cancelled["cancelled"] != false {
		t.Fatalf("request mapping was not released: %#v", cancelled)
	}
}

func TestProcessSnapshotUsesDisplayCWDWithoutLeakingExecutionPath(t *testing.T) {
	usePortableTestShell(t)
	workspace := t.TempDir()
	execution := filepath.Join(t.TempDir(), "private-worktree")
	if err := os.MkdirAll(execution, 0o755); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(workspace, "test-workspace", nil, nil)
	started, err := manager.Start("pwd", StartOptions{CWD: execution, DisplayCWD: "backend/auth", Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	finished := waitExited(t, manager, started.ProcessID)
	if finished.CWD != "backend/auth" {
		t.Fatalf("logical CWD not preserved: %#v", finished)
	}
	if strings.Contains(finished.CWD, "private-worktree") || strings.Contains(finished.CWD, execution) {
		t.Fatalf("snapshot leaked private execution path: %#v", finished)
	}
	stdout, _ := finished.Stdout["text"].(string)
	if strings.Contains(stdout, execution) || strings.Contains(stdout, "private-worktree") {
		t.Fatalf("process output leaked private execution path: %#v", finished.Stdout)
	}
	if !strings.Contains(stdout, "backend/auth") {
		t.Fatalf("process output did not replace private cwd with logical cwd: %#v", finished.Stdout)
	}
	listed := manager.List()
	if len(listed) != 1 || listed[0]["cwd"] != "backend/auth" {
		t.Fatalf("process list did not preserve display CWD: %#v", listed)
	}
}

func TestProcessPathAliasesIncludeMSYSWindowsForm(t *testing.T) {
	aliases := processPathAliases(`C:\Users\runneradmin\AppData\Local\Temp\private-worktree`)
	joined := strings.Join(aliases, "\n")
	for _, want := range []string{
		`C:\Users\runneradmin\AppData\Local\Temp\private-worktree`,
		"C:/Users/runneradmin/AppData/Local/Temp/private-worktree",
		"/c/Users/runneradmin/AppData/Local/Temp/private-worktree",
		"/tmp/private-worktree",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing Windows path alias %q in %#v", want, aliases)
		}
	}
}

func TestReadBufferUsesUTF8ByteOffsets(t *testing.T) {
	data := []byte("😀é")
	buffer := streamBuffer{Data: append([]byte(nil), data...), BaseOffset: 0, TotalBytes: int64(len(data))}
	cursor := int64(len([]byte("😀")))
	result := readBuffer(buffer, &cursor)
	if result["text"] != "é" || result["cursor"] != int64(len(data)) {
		t.Fatalf("result=%#v", result)
	}
}

func TestSnapshotAndListRedactCommandSecrets(t *testing.T) {
	usePortableTestShell(t)
	root := t.TempDir()
	manager := NewManager(root, "test-workspace", nil, nil)
	started, err := manager.Start("echo --token super-secret-value", StartOptions{CWD: root, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	finished := waitExited(t, manager, started.ProcessID)
	if strings.Contains(finished.Command, "super-secret-value") || !strings.Contains(finished.Command, "[REDACTED]") {
		t.Fatalf("snapshot command was not redacted: %q", finished.Command)
	}
	for _, record := range manager.List() {
		if command, _ := record["command"].(string); strings.Contains(command, "super-secret-value") {
			t.Fatalf("process list leaked command secret: %q", command)
		}
	}
}

func TestManagerPrunesFinishedProcesses(t *testing.T) {
	usePortableTestShell(t)
	t.Setenv("CODELOCAL_MAX_PROCESSES", "1")
	root := t.TempDir()
	manager := NewManager(root, "test-workspace", nil, nil)
	first, err := manager.Start("go version", StartOptions{CWD: root, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_ = waitExited(t, manager, first.ProcessID)
	second, err := manager.Start("go version", StartOptions{CWD: root, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_ = waitExited(t, manager, second.ProcessID)
	if _, err := manager.Snapshot(first.ProcessID, nil, nil); err == nil {
		t.Fatal("old finished process should have been pruned")
	}
}

func TestTail(t *testing.T) {
	if got := Tail([]byte("abcdef"), 3); got != "def" {
		t.Fatalf("Tail=%q", got)
	}
	if got := Tail([]byte("abc"), 8); got != "abc" {
		t.Fatalf("Tail=%q", got)
	}
}

func TestProcessSubprocessEnvironmentSanitization(t *testing.T) {
	usePortableTestShell(t)
	t.Setenv("OPENAI_API_KEY", "secret-key-12345")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "aws-secret-67890")
	t.Setenv("MY_APP_TOKEN", "my-token-abcde")
	t.Setenv("SAFE_TEST_VAR", "safe-value-xyz")

	root := t.TempDir()
	manager := NewManager(root, "test-workspace", nil, nil)
	started, err := manager.Start("echo $OPENAI_API_KEY $AWS_SECRET_ACCESS_KEY $MY_APP_TOKEN $SAFE_TEST_VAR", StartOptions{CWD: root, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	finished := waitExited(t, manager, started.ProcessID)
	stdout, _ := finished.Stdout["text"].(string)
	if strings.Contains(stdout, "secret-key-12345") || strings.Contains(stdout, "aws-secret-67890") || strings.Contains(stdout, "my-token-abcde") {
		t.Fatalf("subprocess leaked sensitive environment variables: %q", stdout)
	}
	if !strings.Contains(stdout, "safe-value-xyz") {
		t.Fatalf("subprocess failed to preserve safe environment variables: %q", stdout)
	}
}
