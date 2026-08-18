package automation

import "testing"

func TestComputerWindowEnumerationHealthDetectsFallbackOnly(t *testing.T) {
	health := computerWindowEnumerationHealth([]any{map[string]any{
		"windowId": "screen:main",
		"fallback": true,
	}})
	if health["status"] != "degraded" || health["reason"] != "WINDOW_ENUMERATION_FALLBACK_ONLY" {
		t.Fatalf("unexpected fallback-only health: %#v", health)
	}
	if health["applicationWindowCount"] != 0 || health["fallbackWindowCount"] != 1 {
		t.Fatalf("unexpected fallback-only counts: %#v", health)
	}
}

func TestComputerWindowEnumerationHealthIsHealthyWithApplicationWindow(t *testing.T) {
	health := computerWindowEnumerationHealth([]any{
		map[string]any{"windowId": "screen:main", "fallback": true},
		map[string]any{"windowId": "ax:123:0", "app": "Code", "title": "CodeLocal"},
	})
	if health["status"] != "healthy" {
		t.Fatalf("application window should make enumeration healthy: %#v", health)
	}
	if health["applicationWindowCount"] != 1 || health["fallbackWindowCount"] != 1 {
		t.Fatalf("unexpected healthy counts: %#v", health)
	}
}
