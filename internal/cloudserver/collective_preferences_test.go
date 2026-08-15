package cloudserver

import (
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/webauth"
)

func TestCollectiveFormBoolIsExplicit(t *testing.T) {
	for _, value := range []string{"1", "true", "YES", "on", "enabled"} {
		if !collectiveFormBool(value) {
			t.Fatalf("expected %q to enable setting", value)
		}
	}
	for _, value := range []string{"", "0", "false", "off", "random"} {
		if collectiveFormBool(value) {
			t.Fatalf("unexpected truthy value %q", value)
		}
	}
}

func TestCollectivePreferencePayloadExposesPrivacyBoundaryAndConfiguredCohort(t *testing.T) {
	t.Setenv("CODELOCAL_COLLECTIVE_CONTRIBUTION", "1")
	t.Setenv("CODELOCAL_COLLECTIVE_INTELLIGENCE", "0")
	t.Setenv("CODELOCAL_COLLECTIVE_MIN_CONTRIBUTORS", "24")
	payload := collectivePreferencePayload(cloud.CollectivePreference{ContributionEnabled: true})
	rollout := payload["rollout"].(map[string]any)
	if rollout["contributionAvailable"] != true || rollout["suggestionsAvailable"] != false || rollout["minimumContributors"] != 24 {
		t.Fatalf("unexpected collective rollout payload: %#v", rollout)
	}
	privacy := payload["privacy"].(map[string]any)
	for key, value := range privacy {
		if value != false {
			t.Fatalf("privacy boundary %q unexpectedly true: %#v", key, privacy)
		}
	}
}

func TestCollectivePreferencesCardDefaultsToOptOutAndExplainsWithdrawal(t *testing.T) {
	t.Setenv("CODELOCAL_COLLECTIVE_CONTRIBUTION", "")
	t.Setenv("CODELOCAL_COLLECTIVE_INTELLIGENCE", "")
	identity := &webauth.Identity{User: cloud.User{Email: "user@example.com"}, CSRF: "csrf-safe-token"}
	card := collectivePreferencesCard(identity, cloud.CollectivePreference{})
	if !strings.Contains(card, `name="csrf" value="csrf-safe-token"`) || !strings.Contains(card, "Private opt-in") {
		t.Fatalf("collective card missing identity/security metadata: %s", card)
	}
	if strings.Contains(card, `name="contributionEnabled" value="1" checked`) || strings.Contains(card, `name="suggestionsEnabled" value="1" checked`) {
		t.Fatalf("collective settings must default opt-out: %s", card)
	}
	for _, required := range []string{"Turning this off withdraws", "never raw code", "project/repository identity", "local verification"} {
		if !strings.Contains(card, required) {
			t.Fatalf("collective privacy explanation missing %q", required)
		}
	}
}

func TestCollectivePreferencesCardReflectsExplicitOptIn(t *testing.T) {
	t.Setenv("CODELOCAL_COLLECTIVE_CONTRIBUTION", "1")
	t.Setenv("CODELOCAL_COLLECTIVE_INTELLIGENCE", "1")
	identity := &webauth.Identity{User: cloud.User{Email: "user@example.com"}, CSRF: "csrf-safe-token"}
	card := collectivePreferencesCard(identity, cloud.CollectivePreference{ContributionEnabled: true, SuggestionsEnabled: true})
	if !strings.Contains(card, `name="contributionEnabled" value="1" checked`) || !strings.Contains(card, `name="suggestionsEnabled" value="1" checked`) {
		t.Fatalf("collective opt-in not reflected in card: %s", card)
	}
	if strings.Count(card, "Rollout: available") != 2 {
		t.Fatalf("collective rollout status not reflected: %s", card)
	}
}

func TestCollectiveRecommendationsCardShowsOnlyAggregateEvidence(t *testing.T) {
	t.Setenv("CODELOCAL_COLLECTIVE_INTELLIGENCE", "1")
	preference := cloud.CollectivePreference{SuggestionsEnabled: true}
	card := collectiveRecommendationsCard(preference, []cloud.CollectiveRecommendation{{
		PatternKey: "pattern_private_internal_id",
		Fingerprint: cloud.CollectiveFingerprint{
			TaskKind: "bugfix", CheckProfile: []string{"diff-check", "test"}, FileCountBucket: "2-3",
			QualityBucket: "95-100", ExecutionTool: "verify", SkillUsed: true, DiffObserved: true,
		},
		ContributorCount: 24, SampleCount: 80, MeanUserSuccessRate: .91,
	}})
	for _, required := range []string{"Bugfix", "91% mean success", "24 other contributors", "diff-check, test", "No contributor identity or project name is exposed"} {
		if !strings.Contains(card, required) {
			t.Fatalf("collective recommendation missing aggregate evidence %q: %s", required, card)
		}
	}
	if strings.Contains(card, "pattern_private_internal_id") || strings.Contains(card, "SampleCount") {
		t.Fatalf("collective recommendation exposed internal identifiers: %s", card)
	}
}

func TestCollectiveRecommendationsCardDoesNotShowWhenUserDidNotOptIn(t *testing.T) {
	t.Setenv("CODELOCAL_COLLECTIVE_INTELLIGENCE", "1")
	if card := collectiveRecommendationsCard(cloud.CollectivePreference{}, []cloud.CollectiveRecommendation{{ContributorCount: 50}}); card != "" {
		t.Fatalf("collective recommendations shown without user opt-in: %s", card)
	}
}
