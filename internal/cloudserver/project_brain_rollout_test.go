package cloudserver

import "testing"

func TestProjectBrainCloudSyncRolloutKillSwitchAndBounds(t *testing.T) {
	t.Setenv("CODELOCAL_PROJECT_BRAIN_CLOUD_SYNC", "1")
	t.Setenv("CODELOCAL_PROJECT_BRAIN_ROLLOUT_PERCENT", "")
	if projectBrainCloudSyncEnabled("user-a", "device-a") {
		t.Fatal("unset rollout must fail closed")
	}
	t.Setenv("CODELOCAL_PROJECT_BRAIN_ROLLOUT_PERCENT", "not-a-number")
	if projectBrainCloudSyncEnabled("user-a", "device-a") {
		t.Fatal("malformed rollout must fail closed")
	}

	t.Setenv("CODELOCAL_PROJECT_BRAIN_CLOUD_SYNC", "0")
	t.Setenv("CODELOCAL_PROJECT_BRAIN_ROLLOUT_PERCENT", "100")
	if projectBrainCloudSyncEnabled("user-a", "device-a") {
		t.Fatal("kill switch must override rollout percentage")
	}

	t.Setenv("CODELOCAL_PROJECT_BRAIN_CLOUD_SYNC", "1")
	t.Setenv("CODELOCAL_PROJECT_BRAIN_ROLLOUT_PERCENT", "0")
	if projectBrainCloudSyncEnabled("user-a", "device-a") {
		t.Fatal("0 percent rollout must disable cloud sync")
	}

	t.Setenv("CODELOCAL_PROJECT_BRAIN_ROLLOUT_PERCENT", "100")
	if !projectBrainCloudSyncEnabled("user-a", "device-a") {
		t.Fatal("100 percent rollout must enable cloud sync")
	}
}

func TestProjectBrainCloudSyncRolloutIsStablePerDevice(t *testing.T) {
	t.Setenv("CODELOCAL_PROJECT_BRAIN_CLOUD_SYNC", "1")
	t.Setenv("CODELOCAL_PROJECT_BRAIN_ROLLOUT_PERCENT", "37")
	first := projectBrainCloudSyncEnabled("user-a", "device-a")
	for i := 0; i < 20; i++ {
		if got := projectBrainCloudSyncEnabled("user-a", "device-a"); got != first {
			t.Fatal("canary cohort assignment must be deterministic")
		}
	}
}
