package protocol

import "testing"

func TestCompatibilityAndSideEffects(t *testing.T) {
	if !Compatible(1) || !Compatible(2) || Compatible(0) || Compatible(3) {
		t.Fatal("protocol compatibility range is wrong")
	}
	for _, tool := range []string{"write_file", "git_commit", "run_command", "mcp_call"} {
		if !SideEffecting(tool) {
			t.Fatalf("expected %s to be side effecting", tool)
		}
	}
	if SideEffecting("read_file") {
		t.Fatal("read_file must remain read-only")
	}
}
