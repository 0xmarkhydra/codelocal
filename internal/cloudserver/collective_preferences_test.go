package cloudserver

import (
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
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
