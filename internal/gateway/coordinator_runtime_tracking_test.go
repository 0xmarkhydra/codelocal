package gateway

import (
	"context"
	"strings"
	"testing"
)

func TestCoordinatorBeginRuntimeCallSkipsLocalRoute(t *testing.T) {
	coordinator := &Coordinator{
		runtimeSessions: &CloudRuntimeSessionStore{},
		runtimeProfile:  "general-small",
	}
	finish, err := coordinator.beginRuntimeCall(context.Background(), "workspace-key", "workspace-key")
	if err != nil {
		t.Fatalf("local route unexpectedly required cloud session tracking: %v", err)
	}
	finish()
}

func TestCoordinatorBeginRuntimeCallTracksAliasedCloudRoute(t *testing.T) {
	coordinator := &Coordinator{
		// Nil Redis makes BeginCall fail immediately. That failure is useful here:
		// it proves an aliased route entered cloud call tracking instead of taking
		// the local no-op path.
		runtimeSessions: &CloudRuntimeSessionStore{},
		runtimeProfile:  "general-small",
	}
	_, err := coordinator.beginRuntimeCall(context.Background(), "workspace-key", "cloud-runtime-key")
	if err == nil {
		t.Fatal("aliased cloud route skipped runtime call tracking")
	}
	if !strings.Contains(err.Error(), "Redis unavailable") {
		t.Fatalf("unexpected tracking error: %v", err)
	}
}
