package process

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
