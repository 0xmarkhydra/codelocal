package cloud

import "testing"

func TestSkillAffinityScoreUsesNeutralPrior(t *testing.T) {
	if got := skillAffinityScore(0, 0); got != 0 {
		t.Fatalf("no evidence must stay neutral, got %v", got)
	}
	if got := skillAffinityScore(1, 0); got <= 0 || got >= 0.10 {
		t.Fatalf("one success must move ranking gently, got %v", got)
	}
	if got := skillAffinityScore(0, 1); got >= 0 || got <= -0.10 {
		t.Fatalf("one failure must move ranking gently, got %v", got)
	}
}

func TestSkillAffinityScoreIsBounded(t *testing.T) {
	if got := skillAffinityScore(100000, 0); got > 0.25 || got <= 0.24 {
		t.Fatalf("positive affinity must converge within cap, got %v", got)
	}
	if got := skillAffinityScore(0, 100000); got < -0.25 || got >= -0.24 {
		t.Fatalf("negative affinity must converge within cap, got %v", got)
	}
}

func TestSkillAffinityScoreIgnoresNegativeCounts(t *testing.T) {
	if got := skillAffinityScore(-5, -3); got != 0 {
		t.Fatalf("invalid negative evidence must sanitize to neutral, got %v", got)
	}
}
