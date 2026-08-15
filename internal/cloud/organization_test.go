package cloud

import (
	"strings"
	"testing"
)

func TestOrganizationRulesQueryOwnerScoped(t *testing.T) {
	if !strings.Contains(organizationRulesForProjectSQL, "p.owner_user_id=$1") || !strings.Contains(organizationRulesForProjectSQL, "p.project_id=$2") {
		t.Fatalf("organization project rule query must remain owner/project scoped: %s", organizationRulesForProjectSQL)
	}
}

func TestNormalizeOrganizationRuleInputSanitizesAndCreatesStableID(t *testing.T) {
	input := OrganizationRuleInput{OwnerUserID: " owner ", OrganizationID: " org ", Text: "Never log API_KEY=secret-value", ApplyTo: []string{"**/*.go", "**/*.go"}, Required: true}
	first, err := normalizeOrganizationRuleInput(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := normalizeOrganizationRuleInput(input)
	if err != nil {
		t.Fatal(err)
	}
	if first.RuleID == "" || first.RuleID != second.RuleID {
		t.Fatalf("organization rule ID is not stable: %#v %#v", first, second)
	}
	if strings.Contains(first.Text, "secret-value") {
		t.Fatalf("organization rule leaked secret: %q", first.Text)
	}
	if len(first.ApplyTo) != 1 || first.ApplyTo[0] != "**/*.go" {
		t.Fatalf("applyTo not normalized: %#v", first.ApplyTo)
	}
}
