package cloudserver

import "testing"

func TestMaskLeaderboardEmail(t *testing.T) {
	if got := maskLeaderboardEmail("monglv36@gmail.com"); got != "mo***@gmail.com" {
		t.Fatalf("maskLeaderboardEmail() = %q", got)
	}
	if got := maskLeaderboardEmail("a@example.com"); got != "a***@example.com" {
		t.Fatalf("maskLeaderboardEmail(short) = %q", got)
	}
}

func TestLeaderboardInitial(t *testing.T) {
	if got := leaderboardInitial("mong@example.com"); got != "M" {
		t.Fatalf("leaderboardInitial() = %q", got)
	}
	if got := leaderboardInitial(""); got != "C" {
		t.Fatalf("leaderboardInitial(empty) = %q", got)
	}
}

func TestUsageDayStartFloorsToUTCStorageBucket(t *testing.T) {
	const dayMs = int64(24 * 60 * 60 * 1000)
	if got := usageDayStart(dayMs + 12345); got != dayMs {
		t.Fatalf("usageDayStart() = %d want %d", got, dayMs)
	}
}
