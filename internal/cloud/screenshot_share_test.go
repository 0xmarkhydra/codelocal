package cloud

import "testing"

func TestNormalizeScreenshotShareID(t *testing.T) {
	for _, value := range []string{"AbCdEf0123_-", "0123456789abcdef", "shot-link_123"} {
		if got := NormalizeScreenshotShareID(value); got != value {
			t.Fatalf("valid share id %q normalized to %q", value, got)
		}
	}
	for _, value := range []string{"short", "contains/slash", "contains space", "0123456789abcdef?x=1"} {
		if got := NormalizeScreenshotShareID(value); got != "" {
			t.Fatalf("invalid share id %q normalized to %q", value, got)
		}
	}
}

func TestRandomScreenshotShareIDUsesCompactURLSafeAlphabet(t *testing.T) {
	first := randomScreenshotShareID()
	second := randomScreenshotShareID()
	if len(first) != 12 || NormalizeScreenshotShareID(first) != first {
		t.Fatalf("unexpected generated share id %q", first)
	}
	if first == second {
		t.Fatalf("share ids unexpectedly collided: %q", first)
	}
}
