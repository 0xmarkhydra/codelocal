package protocol

import "testing"

func TestCompatibilityAndSideEffects(t *testing.T) {
	if !Compatible(1) || !Compatible(2) || !Compatible(3) || Compatible(0) || Compatible(4) {
		t.Fatal("protocol compatibility range is wrong")
	}
	for _, tool := range []string{"write_file", "git_commit", "run_command", "approval_mode", "mcp_call", "exec_write", "pty_write", "pty_resize", "exec_signal", "pty_signal", "exec_kill", "pty_kill", "browser_click", "computer_type"} {
		if !SideEffecting(tool) {
			t.Fatalf("expected %s to be side effecting", tool)
		}
	}
	if SideEffecting("read_file") {
		t.Fatal("read_file must remain read-only")
	}
}
