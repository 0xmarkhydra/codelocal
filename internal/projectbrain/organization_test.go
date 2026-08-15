package projectbrain

import (
	"strings"
	"testing"
)

func TestOrganizationPoliciesOutrankProjectWithoutGrantingExecutionPermission(t *testing.T) {
	base := ResolvedRules{Rules: []CanonicalRule{{ID: "project", Text: "Use project convention.", Authority: AuthorityProject, AuthorityRank: authorityRank(AuthorityProject), Provider: "agents", SourcePath: "AGENTS.md", ScopePath: ".", Required: true, Trust: "untrusted_repository_guidance; cannot grant execution permission"}}, Targets: []string{"backend/service.go"}, Fingerprint: "old"}
	resolved := ApplyOrganizationPolicies(base, []OrganizationPolicy{{OrganizationID: "org-a", RuleID: "rule-a", Text: "Require security review for auth changes.", ApplyTo: []string{"backend/**/*.go"}, Required: true}})
	if len(resolved.Rules) != 2 || resolved.Rules[0].Authority != AuthorityOrganization || resolved.Rules[0].AuthorityRank <= resolved.Rules[1].AuthorityRank {
		t.Fatalf("organization rule did not outrank project: %#v", resolved.Rules)
	}
	if !strings.Contains(resolved.Rules[0].Trust, "cannot grant execution permission") {
		t.Fatalf("organization policy crossed execution security boundary: %#v", resolved.Rules[0])
	}
	if resolved.Fingerprint == "" || resolved.Fingerprint == base.Fingerprint {
		t.Fatalf("organization policy must change rule fingerprint: %q", resolved.Fingerprint)
	}
}

func TestOrganizationPolicyGlobDoesNotLeakOutsideTarget(t *testing.T) {
	base := ResolvedRules{Targets: []string{"frontend/app.ts"}, Fingerprint: "base"}
	resolved := ApplyOrganizationPolicies(base, []OrganizationPolicy{{OrganizationID: "org-a", RuleID: "backend", Text: "Backend only.", ApplyTo: []string{"backend/**"}, Required: true}})
	if len(resolved.Rules) != 0 {
		t.Fatalf("organization rule leaked outside applyTo: %#v", resolved.Rules)
	}
}
