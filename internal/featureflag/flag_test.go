package featureflag

import "testing"

func TestEnabledParsesTruthyValues(t *testing.T) {
	for _, value := range []string{"1", "true", "TRUE", "yes", "on", " ON "} {
		t.Setenv("CODELOCAL_FEATURE_WORKTREE_V2", value)
		if !Enabled(WorktreeV2) {
			t.Fatalf("value %q should enable the flag", value)
		}
	}
}

func TestEnabledDefaultsOff(t *testing.T) {
	t.Setenv("CODELOCAL_FEATURE_PATCH_ENGINE_V2", "")
	if Enabled(PatchEngineV2) {
		t.Fatal("unset flag must default to disabled")
	}
	for _, value := range []string{"0", "false", "no", "off", "banana"} {
		t.Setenv("CODELOCAL_FEATURE_PATCH_ENGINE_V2", value)
		if Enabled(PatchEngineV2) {
			t.Fatalf("value %q must not enable the flag", value)
		}
	}
}

func TestEnabledUnknownIsFailClosed(t *testing.T) {
	t.Setenv("CODELOCAL_FEATURE_ANYTHING", "1")
	t.Setenv("CODELOCAL_FEATURE_WORKTREE_V2", "1")
	if Enabled("not_a_flag") || Enabled("") {
		t.Fatal("unknown or empty flag names must stay disabled")
	}
	if !Enabled(" worktree_v2 ") {
		t.Fatal("padded known names must resolve")
	}
}

func TestSnapshotCoversKnownFlags(t *testing.T) {
	t.Setenv("CODELOCAL_FEATURE_AGENT_RUNTIME_V2", "1")
	snapshot := Snapshot()
	if len(snapshot) != len(Known()) {
		t.Fatalf("snapshot has %d entries, want %d", len(snapshot), len(Known()))
	}
	if !snapshot[AgentRuntimeV2] {
		t.Fatal("enabled flag must appear enabled in snapshot")
	}
	if snapshot[PolicyKernelV2] {
		t.Fatal("unset flag must appear disabled in snapshot")
	}
}
