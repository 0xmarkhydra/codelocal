package projectbrain

import (
	"strings"
	"testing"
)

func TestExportBundleExcludesLocalPrivateAndSanitizesSecrets(t *testing.T) {
	manifest := NewManifest([]Source{
		{Path: "AGENTS.md", Provider: "agents", SourceType: "instructions", Classification: "private_project", ContentHash: "safe-hash", ParserFingerprint: "parser", AdapterVersion: "1", ParserVersion: "1", SemanticNormalizerVersion: "1"},
		{Path: "CLAUDE.local.md", Provider: "claude", SourceType: "instructions", Classification: "local_private", ContentHash: "local-hash", ParserFingerprint: "parser", AdapterVersion: "1", ParserVersion: "1", SemanticNormalizerVersion: "1"},
	})
	resolved := ResolvedRules{Fingerprint: "rules", Rules: []CanonicalRule{
		{ID: "project", Text: "Never log API_KEY=secret-value", Authority: AuthorityProject, AuthorityRank: authorityRank(AuthorityProject), SourcePath: "AGENTS.md", Required: true},
		{ID: "local", Text: "local-only text", Authority: AuthorityUserPreference, AuthorityRank: authorityRank(AuthorityUserPreference), SourcePath: "CLAUDE.local.md", LocalOnly: true},
	}}
	bundle := NewExportBundle(manifest, resolved, []ExportExperience{{ExperienceID: "exp", Objective: "Fix auth password: secret-pass", Outcome: "succeeded", VerificationSummary: "Bearer secret-token tests passed"}})
	if len(bundle.Sources) != 1 || bundle.Sources[0].Path != "AGENTS.md" {
		t.Fatalf("local-private source exported: %#v", bundle.Sources)
	}
	if len(bundle.Rules) != 1 || strings.Contains(bundle.Rules[0].Text, "secret-value") || strings.Contains(bundle.Rules[0].Text, "local-only") {
		t.Fatalf("unsafe rules exported: %#v", bundle.Rules)
	}
	if len(bundle.Experiences) != 1 || strings.Contains(bundle.Experiences[0].Objective, "secret-pass") || strings.Contains(bundle.Experiences[0].VerificationSummary, "secret-token") {
		t.Fatalf("unsafe Experience exported: %#v", bundle.Experiences)
	}
	if !strings.Contains(bundle.PrivacyPolicy, "no raw source") {
		t.Fatalf("export privacy contract missing: %q", bundle.PrivacyPolicy)
	}
}
