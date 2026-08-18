package automation

import (
	"testing"
	"time"
)

func TestComputerUserRecentlyActiveUsesIdleThreshold(t *testing.T) {
	active, err := computerUserRecentlyActive(map[string]any{"idleMs": float64(250)}, 2*time.Second)
	if err != nil || !active {
		t.Fatalf("recent user activity should be active: active=%v err=%v", active, err)
	}
	active, err = computerUserRecentlyActive(map[string]any{"idleMs": float64(2500)}, 2*time.Second)
	if err != nil || active {
		t.Fatalf("idle user should not be active: active=%v err=%v", active, err)
	}
}

func TestComputerUserRecentlyActiveRejectsInvalidPayload(t *testing.T) {
	for _, activity := range []map[string]any{
		{},
		{"idleMs": "100"},
		{"idleMs": float64(-1)},
	} {
		if _, err := computerUserRecentlyActive(activity, 2*time.Second); err == nil {
			t.Fatalf("invalid user activity payload accepted: %#v", activity)
		}
	}
}

func TestUserActivityGuardCapabilityIsExplicit(t *testing.T) {
	controller := &ComputerController{Capabilities: map[string]any{"userActivityGuard": true}}
	if !controller.UserActivityGuardSupported() {
		t.Fatal("explicit user activity guard capability should be honored")
	}
	controller.Capabilities["userActivityGuard"] = false
	if controller.UserActivityGuardSupported() {
		t.Fatal("disabled user activity guard capability should not be inferred")
	}
}
