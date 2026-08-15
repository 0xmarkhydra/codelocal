package cloud

import (
	"strings"
	"testing"
)

func TestExperienceRequiresVerifiedEvidence(t *testing.T) {
	_, err := normalizeExperienceInput(ExperienceInput{UserID: "user", Objective: "fix bug", Outcome: "succeeded", VerificationSummary: "tests passed"})
	if err == nil {
		t.Fatal("unverified outcome must not become Experience")
	}
}

func TestExperienceSanitizesSecretsAndBoundsEvidence(t *testing.T) {
	input, err := normalizeExperienceInput(ExperienceInput{
		UserID: " user ", Objective: "fix login API_KEY=super-secret", Outcome: "succeeded", Verified: true,
		VerificationSummary: "Bearer token-value tests passed", Files: []string{"a.go", "a.go"}, Checks: []string{"go-test", "go-test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(input.Objective, "super-secret") || strings.Contains(input.VerificationSummary, "token-value") {
		t.Fatalf("experience leaked secret material: %#v", input)
	}
	if len(input.Files) != 1 || len(input.Checks) != 1 {
		t.Fatalf("experience evidence not compacted: %#v", input)
	}
}

func TestExperienceIDIsIdempotentAndTenantScoped(t *testing.T) {
	base := ExperienceInput{UserID: "user-a", ProjectID: "project", Objective: "fix bug", Outcome: "succeeded", VerificationSummary: "verified", Verified: true, IdempotencyKey: "same-task"}
	first := experienceID(base)
	if second := experienceID(base); second != first {
		t.Fatalf("experience id not stable: %q != %q", first, second)
	}
	other := base
	other.UserID = "user-b"
	if experienceID(other) == first {
		t.Fatal("experience identity must be tenant-scoped")
	}
}
