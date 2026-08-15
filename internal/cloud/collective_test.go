package cloud

import (
	"encoding/json"
	"strings"
	"testing"
)

func collectiveExperienceFixture() Experience {
	return Experience{
		UserID: "private-user-a", ExperienceID: "exp-private-a", ProjectID: "secret-project-a", RepositoryID: "private-repo-a",
		WorkspaceID: "workspace-a", DeviceID: "device-a", TaskID: "task-a", TaskKind: "bugfix",
		Objective: "Fix ACME customer billing bug with secret contract 123", Branch: "customer/acme-private",
		Files:   []string{"customers/acme/private_invoice.go", "internal/billing/db.go"},
		Symbols: []string{"AcmeSecretInvoice", "privateBillingToken"},
		Checks:  []string{"diff-check:abcdef", "test:123456", "test:duplicate"}, Outcome: "succeeded",
		RootCause: "customer API secret leaked into the transaction retry path", SkillID: "private-skill-id",
		VerificationSummary: "private verification text that must not cross the collective boundary",
		Metadata:            map[string]any{"executionTool": "verify", "qualityScore": 98, "diffObserved": true, "privatePath": "/Users/private/project"},
	}
}

func TestCollectiveFingerprintExcludesRawPrivateExperienceContent(t *testing.T) {
	experience := collectiveExperienceFixture()
	fingerprint, ok := collectiveFingerprintForExperience(experience)
	if !ok {
		t.Fatal("verified experience was not fingerprintable")
	}
	raw, err := json.Marshal(fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(raw))
	for _, forbidden := range []string{"acme", "secret contract", "private_invoice", "acmesecretinvoice", "privatebillingtoken", "customer api", "/users/private", "secret-project-a", "private-repo-a", "private-skill-id"} {
		if strings.Contains(text, strings.ToLower(forbidden)) {
			t.Fatalf("collective fingerprint leaked private content %q: %s", forbidden, text)
		}
	}
	if fingerprint.TaskKind != "bugfix" || len(fingerprint.CheckProfile) != 2 || fingerprint.CheckProfile[0] != "diff-check" || fingerprint.CheckProfile[1] != "test" {
		t.Fatalf("unexpected safe fingerprint: %#v", fingerprint)
	}
	if fingerprint.FileCountBucket != "2-3" || fingerprint.SymbolBucket != "2-3" || fingerprint.QualityBucket != "95-100" || !fingerprint.SkillUsed || !fingerprint.DiffObserved {
		t.Fatalf("structured buckets missing: %#v", fingerprint)
	}
}

func TestCollectivePatternIgnoresTenantProjectAndFreeFormText(t *testing.T) {
	left := collectiveExperienceFixture()
	right := collectiveExperienceFixture()
	right.UserID = "different-user"
	right.ExperienceID = "different-experience"
	right.ProjectID = "totally-different-project"
	right.RepositoryID = "totally-different-repo"
	right.WorkspaceID = "other-workspace"
	right.DeviceID = "other-device"
	right.TaskID = "other-task"
	right.Objective = "Completely unrelated private description"
	right.RootCause = "Different sensitive root cause"
	right.Files = []string{"private/a.go", "private/b.go"}
	right.Symbols = []string{"SecretA", "SecretB"}
	right.SkillID = "another-private-skill"
	leftFingerprint, _ := collectiveFingerprintForExperience(left)
	rightFingerprint, _ := collectiveFingerprintForExperience(right)
	if collectivePatternKey(leftFingerprint) != collectivePatternKey(rightFingerprint) {
		t.Fatalf("raw tenant/project/free-form content affected de-identified pattern: left=%#v right=%#v", leftFingerprint, rightFingerprint)
	}
	if collectiveEventToken(left) == collectiveEventToken(right) {
		t.Fatal("idempotency event token must remain contributor/event-specific")
	}
	if strings.Contains(collectiveEventToken(left), left.ExperienceID) || strings.Contains(collectiveEventToken(left), left.UserID) {
		t.Fatalf("collective event ledger token leaked raw identifiers: %q", collectiveEventToken(left))
	}
}

func TestCollectiveUnknownFreeFormCategoriesFailClosedToOther(t *testing.T) {
	experience := collectiveExperienceFixture()
	experience.TaskKind = "customer-acme-special-flow"
	experience.Metadata["executionTool"] = "private-deployer"
	experience.Checks = []string{"custom-secret-check:abc"}
	fingerprint, ok := collectiveFingerprintForExperience(experience)
	if !ok || fingerprint.TaskKind != "other" || fingerprint.ExecutionTool != "other" || len(fingerprint.CheckProfile) != 1 || fingerprint.CheckProfile[0] != "verified" {
		t.Fatalf("unrecognized private categories escaped whitelist: %#v", fingerprint)
	}
}

func TestCollectiveMigrationDefaultsPrivateAndSupportsWithdrawal(t *testing.T) {
	lower := strings.ToLower(collectiveIntelligenceMigrationSQL)
	for _, required := range []string{
		"contribution_enabled boolean not null default false",
		"suggestions_enabled boolean not null default false",
		"primary key(user_id,pattern_key)",
		"primary key(user_id,event_token)",
		"sample_count = success_count + failure_count",
	} {
		if !strings.Contains(lower, required) {
			t.Fatalf("collective migration missing %q", required)
		}
	}
	for _, forbidden := range []string{"objective", "root_cause", "repository_id", "project_id", "workspace_id", "device_id", "symbols", "files"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("collective schema contains forbidden raw private field %q", forbidden)
		}
	}
}

func TestCollectiveRecommendationRequiresMinimumCohort(t *testing.T) {
	t.Setenv("CODELOCAL_COLLECTIVE_MIN_CONTRIBUTORS", "1")
	if got := collectiveMinContributors(); got != 5 {
		t.Fatalf("collective minimum cohort=%d want privacy floor 5", got)
	}
	t.Setenv("CODELOCAL_COLLECTIVE_MIN_CONTRIBUTORS", "20")
	if got := collectiveMinContributors(); got != 20 {
		t.Fatalf("collective configured cohort=%d want 20", got)
	}
	lower := strings.ToLower(collectiveRecommendationsSQL)
	if !strings.Contains(lower, "user_id <> $2") || !strings.Contains(lower, "having count(*) >= $3") || !strings.Contains(lower, "group by pattern_key,fingerprint") {
		t.Fatalf("collective query does not enforce external-user cohort aggregation: %s", lower)
	}
	if strings.Contains(strings.Split(lower, "from codelocal_collective_user_patterns")[0], "user_id") {
		t.Fatal("collective recommendation output must not select contributor user ids")
	}
}

func TestCollectiveFeaturesDefaultOff(t *testing.T) {
	t.Setenv("CODELOCAL_COLLECTIVE_CONTRIBUTION", "")
	t.Setenv("CODELOCAL_COLLECTIVE_INTELLIGENCE", "")
	if collectiveEnvEnabled("CODELOCAL_COLLECTIVE_CONTRIBUTION") || collectiveEnvEnabled("CODELOCAL_COLLECTIVE_INTELLIGENCE") {
		t.Fatal("collective features must be default-off")
	}
}
