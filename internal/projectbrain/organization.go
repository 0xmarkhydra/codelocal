package projectbrain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

type OrganizationPolicy struct {
	OrganizationID string   `json:"organizationId"`
	RuleID         string   `json:"ruleId"`
	Text           string   `json:"text"`
	ApplyTo        []string `json:"applyTo,omitempty"`
	Required       bool     `json:"required"`
}

func organizationPolicyApplies(policy OrganizationPolicy, targets []string) bool {
	if len(policy.ApplyTo) == 0 {
		return true
	}
	return globsApply(policy.ApplyTo, targets)
}

func ApplyOrganizationPolicies(base ResolvedRules, policies []OrganizationPolicy) ResolvedRules {
	out := ResolvedRules{
		Rules:     append([]CanonicalRule(nil), base.Rules...),
		Targets:   append([]string(nil), base.Targets...),
		Conflicts: append([]RuleConflict(nil), base.Conflicts...),
	}
	seen := map[string]struct{}{}
	for _, rule := range out.Rules {
		seen[rule.ID] = struct{}{}
	}
	for _, policy := range policies {
		policy.OrganizationID = strings.TrimSpace(policy.OrganizationID)
		policy.RuleID = strings.TrimSpace(policy.RuleID)
		policy.Text = strings.Join(strings.Fields(policy.Text), " ")
		policy.ApplyTo = uniqueStrings(policy.ApplyTo)
		if policy.OrganizationID == "" || policy.RuleID == "" || policy.Text == "" || !organizationPolicyApplies(policy, out.Targets) {
			continue
		}
		id := "org:" + policy.OrganizationID + ":" + policy.RuleID
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out.Rules = append(out.Rules, CanonicalRule{
			ID: id, Text: policy.Text, Authority: AuthorityOrganization, AuthorityRank: authorityRank(AuthorityOrganization),
			Provider: "codelocal-organization", SourceType: "organization_rule", SourcePath: "organization:" + policy.OrganizationID,
			ScopePath: ".", ApplyTo: policy.ApplyTo, Required: policy.Required, Trust: "trusted_organization_policy; cannot grant execution permission",
		})
	}
	sort.SliceStable(out.Rules, func(i, j int) bool {
		if out.Rules[i].AuthorityRank != out.Rules[j].AuthorityRank {
			return out.Rules[i].AuthorityRank > out.Rules[j].AuthorityRank
		}
		if out.Rules[i].SourcePath != out.Rules[j].SourcePath {
			return out.Rules[i].SourcePath < out.Rules[j].SourcePath
		}
		return out.Rules[i].ID < out.Rules[j].ID
	})
	out.Conflicts = detectRuleConflicts(out.Rules)
	fingerprintParts := make([]string, 0, len(out.Targets)+len(out.Rules))
	for _, target := range out.Targets {
		fingerprintParts = append(fingerprintParts, "target:"+target)
	}
	for _, rule := range out.Rules {
		fingerprintParts = append(fingerprintParts, fmt.Sprintf("%d:%s:%s", rule.AuthorityRank, rule.ID, rule.Text))
	}
	sum := sha256.Sum256([]byte(strings.Join(fingerprintParts, "\x00")))
	out.Fingerprint = hex.EncodeToString(sum[:])
	return out
}
