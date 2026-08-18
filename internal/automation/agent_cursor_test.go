package automation

import "testing"

func TestLegacyAgentCursorIsOptIn(t *testing.T) {
	t.Setenv("CODELOCAL_AGENT_CURSOR", "")
	if legacyAgentCursorEnabled() {
		t.Fatal("legacy osascript agent cursor must be disabled by default")
	}
	for _, value := range []string{"1", "true", "on", "TRUE"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("CODELOCAL_AGENT_CURSOR", value)
			if !legacyAgentCursorEnabled() {
				t.Fatalf("CODELOCAL_AGENT_CURSOR=%q should opt in to the legacy visual cursor", value)
			}
		})
	}
}
