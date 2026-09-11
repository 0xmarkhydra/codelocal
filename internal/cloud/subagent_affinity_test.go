package cloud

import (
	"strings"
	"testing"
)

func TestSubagentAffinityScoreSmoothsSingleSample(t *testing.T) {
	if got := subagentAffinityScore(0, 0); got != 0 {
		t.Fatalf("no evidence must stay neutral, got %v", got)
	}
	// 1 success / 0 failure with +4 prior: 0.10 * 1/5 = 0.02 — gentle nudge.
	if got := subagentAffinityScore(1, 0); got <= 0 || got >= 0.05 {
		t.Fatalf("one success must move ranking gently, got %v", got)
	}
	if got := subagentAffinityScore(0, 1); got >= 0 || got <= -0.05 {
		t.Fatalf("one failure must move ranking gently, got %v", got)
	}
}

func TestSubagentAffinityScoreIsBounded(t *testing.T) {
	if got := subagentAffinityScore(100000, 0); got > 0.10 || got <= 0.09 {
		t.Fatalf("positive affinity must converge within cap, got %v", got)
	}
	if got := subagentAffinityScore(0, 100000); got < -0.10 || got >= -0.09 {
		t.Fatalf("negative affinity must converge within cap, got %v", got)
	}
}

func TestSubagentAffinityScoreIgnoresNegativeCounts(t *testing.T) {
	if got := subagentAffinityScore(-5, -3); got != 0 {
		t.Fatalf("invalid negative evidence must sanitize to neutral, got %v", got)
	}
}

func TestNormalizeSubagentRoutingIDBoundsAndSanitizes(t *testing.T) {
	if got := normalizeSubagentRoutingID(" Explorer "); got != "explorer" {
		t.Fatalf("unexpected id %q", got)
	}
	if got := normalizeSubagentRoutingID("my explorer.v2"); got != "my-explorer-v2" {
		t.Fatalf("unexpected id %q", got)
	}
	if got := normalizeSubagentRoutingID(strings.Repeat("a", 65)); got != "" {
		t.Fatalf("oversize id must be rejected, got %q", got)
	}
	if got := normalizeSubagentRoutingID("!!!"); got != "" {
		t.Fatalf("empty-after-clean id must be rejected, got %q", got)
	}
}

func TestSubagentExperienceRequiresVerifiedOutcome(t *testing.T) {
	_, err := normalizeExperienceInput(ExperienceInput{
		UserID: "user", Objective: "survey auth", Outcome: "succeeded",
		VerificationSummary: "done", Verified: true, SubagentID: "explorer", SubagentRole: "investigator", SubagentVerified: false,
	})
	if err == nil {
		t.Fatal("unverified subagent outcome must not become Experience")
	}
	for name, bad := range map[string]ExperienceInput{
		"orphan metadata": {
			UserID: "user", Objective: "survey auth", Outcome: "succeeded", VerificationSummary: "done", Verified: true,
			SubagentRole: "investigator", SubagentVerified: true,
		},
		"scope": {
			UserID: "user", Objective: "survey auth", Outcome: "succeeded", VerificationSummary: "done", Verified: true,
			SubagentID: "explorer", SubagentRole: "investigator", SubagentScope: "wide", SubagentVerified: true,
		},
		"id": {
			UserID: "user", Objective: "survey auth", Outcome: "succeeded", VerificationSummary: "done", Verified: true,
			SubagentID: "!!!", SubagentRole: "investigator", SubagentVerified: true,
		},
		"role": {
			UserID: "user", Objective: "survey auth", Outcome: "succeeded", VerificationSummary: "done", Verified: true,
			SubagentID: "explorer", SubagentRole: "boss", SubagentVerified: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := normalizeExperienceInput(bad); err == nil {
				t.Fatalf("invalid subagent %s accepted", name)
			}
		})
	}
	for _, outcome := range []string{"succeeded", "failed"} {
		ok := ExperienceInput{
			UserID: "user", Objective: "survey auth", Outcome: outcome,
			VerificationSummary: "evidenced outcome", Verified: true,
			SubagentID: "explorer", SubagentRole: "investigator", SubagentScope: "project", SubagentVerified: true,
		}
		normalized, err := normalizeExperienceInput(ok)
		if err != nil {
			t.Fatal(err)
		}
		if normalized.SubagentID != "explorer" || normalized.SubagentRole != "investigator" || normalized.SubagentScope != "project" {
			t.Fatalf("unexpected normalized subagent: %+v", normalized)
		}
	}
}
