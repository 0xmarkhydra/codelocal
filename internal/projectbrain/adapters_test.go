package projectbrain

import "testing"

func TestBuiltInAdaptersAreDeclarativeAndStable(t *testing.T) {
	registry := NewAdapterRegistry()
	items := registry.List()
	if len(items) < 7 {
		t.Fatalf("built-in adapters=%d", len(items))
	}
	for _, item := range items {
		if !allowedDeclarativeParser(item.ParserKind) || item.Digest == "" || item.Digest != AdapterManifestDigest(item) {
			t.Fatalf("invalid built-in adapter: %#v", item)
		}
	}
}

func TestExternalAdapterRequiresVerifiedSignatureMetadata(t *testing.T) {
	manifest := AdapterManifest{ID: "windsurf-rules", Version: "1", Provider: "windsurf", SourceType: "rule", Patterns: []string{".windsurf/rules/**/*.md"}, Classification: "private_project", ParserKind: ParserMarkdownRules, Signer: "trusted-publisher", Signature: "signature"}
	manifest.Digest = AdapterManifestDigest(manifest)
	registry := NewAdapterRegistry()
	if err := registry.Register(manifest, AdapterTrust{}); err == nil {
		t.Fatal("unverified external manifest must be rejected")
	}
	if err := registry.Register(manifest, AdapterTrust{Signer: "trusted-publisher", SignatureVerified: true}); err != nil {
		t.Fatalf("verified declarative manifest rejected: %v", err)
	}
}

func TestAdapterManifestRejectsExecutableParserKind(t *testing.T) {
	manifest := AdapterManifest{ID: "unsafe", Version: "1", Provider: "unsafe", SourceType: "rule", Patterns: []string{"RULES.md"}, Classification: "private_project", ParserKind: "shell_script", Signer: "trusted", Signature: "signature"}
	manifest.Digest = AdapterManifestDigest(manifest)
	if _, err := validateAdapterManifest(manifest, AdapterTrust{Signer: "trusted", SignatureVerified: true}); err == nil {
		t.Fatal("adapter registry must not accept arbitrary executable parser code")
	}
}
