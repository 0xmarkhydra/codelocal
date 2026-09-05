package plugins

import (
	"encoding/json"
	"strings"
	"testing"
)

func validPublicManifest() Manifest {
	return Manifest{
		SchemaVersion:    SchemaVersion,
		ID:               "github-productivity",
		Name:             "GitHub Productivity",
		Version:          "1.0.0",
		Publisher:        Publisher{ID: "github", Name: "GitHub", Verified: true},
		Description:      "Work with repositories, issues and pull requests.",
		Categories:       []string{"developer-tools", "productivity"},
		Scope:            ScopeCommunity,
		Distribution:     DistributionPublic,
		PrivacyPolicyURL: "https://example.com/privacy",
		TermsURL:         "https://example.com/terms",
		Components: []Component{
			{
				Kind: ComponentApp,
				ID:   "github",
				App: &AppDefinition{
					Transport:    TransportMCPHTTP,
					Endpoint:     "https://example.com/mcp",
					Auth:         AuthDefinition{Kind: AuthOAuth2},
					Capabilities: []Capability{CapabilityExternalRead, CapabilityExternalWrite},
				},
			},
			{
				Kind:  ComponentSkill,
				ID:    "github-pr-review",
				Skill: &SkillReference{SkillID: "github-pr-review", Version: "1.0.0"},
			},
		},
	}
}

func TestValidateManifestAcceptsValidPublicPlugin(t *testing.T) {
	if err := ValidateManifest(validPublicManifest()); err != nil {
		t.Fatalf("ValidateManifest() error = %v", err)
	}
}

func TestValidateManifestRequiresCanonicalIdentity(t *testing.T) {
	for name, mutate := range map[string]func(*Manifest){
		"empty id":      func(m *Manifest) { m.ID = "" },
		"empty name":    func(m *Manifest) { m.Name = "" },
		"empty version": func(m *Manifest) { m.Version = "" },
		"bad id":        func(m *Manifest) { m.ID = "Git Hub" },
		"bad version":   func(m *Manifest) { m.Version = "v1" },
	} {
		t.Run(name, func(t *testing.T) {
			manifest := validPublicManifest()
			mutate(&manifest)
			if err := ValidateManifest(manifest); err == nil {
				t.Fatal("ValidateManifest() unexpectedly succeeded")
			}
		})
	}
}

func TestValidateManifestRejectsDuplicateComponentID(t *testing.T) {
	manifest := validPublicManifest()
	manifest.Components[1].ID = manifest.Components[0].ID
	if err := ValidateManifest(manifest); err == nil || !strings.Contains(err.Error(), "duplicate component id") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateManifestRejectsUnsupportedComponentKind(t *testing.T) {
	manifest := validPublicManifest()
	manifest.Components = []Component{{Kind: ComponentKind("magic"), ID: "magic"}}
	if err := ValidateManifest(manifest); err == nil || !strings.Contains(err.Error(), "unsupported component kind") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateManifestRejectsPublicInsecureMCPHTTP(t *testing.T) {
	manifest := validPublicManifest()
	manifest.Components[0].App.Endpoint = "http://example.com/mcp"
	if err := ValidateManifest(manifest); err == nil || !strings.Contains(err.Error(), "insecure mcp http endpoint") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateManifestAllowsPersonalLoopbackMCPHTTP(t *testing.T) {
	manifest := validPublicManifest()
	manifest.Scope = ScopePersonal
	manifest.Distribution = DistributionPrivate
	manifest.PrivacyPolicyURL = ""
	manifest.TermsURL = ""
	manifest.Components = []Component{{
		Kind: ComponentApp,
		ID:   "local-dev",
		App: &AppDefinition{
			Transport: TransportMCPHTTP,
			Endpoint:  "http://127.0.0.1:3001/mcp",
			Auth:      AuthDefinition{Kind: AuthNone},
		},
	}}
	if err := ValidateManifest(manifest); err != nil {
		t.Fatalf("error = %v", err)
	}
}

func TestManifestSchemaDoesNotContainRawCredentialFields(t *testing.T) {
	raw, err := json.Marshal(validPublicManifest())
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, forbidden := range []string{`"token":`, `"accessToken":`, `"refreshToken":`, `"apiKey":`} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("manifest unexpectedly serialized credential field %s", forbidden)
		}
	}
}

func TestManifestHashIsDeterministic(t *testing.T) {
	manifest := validPublicManifest()
	first, err := ManifestHash(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		next, err := ManifestHash(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if next != first {
			t.Fatalf("hash changed: %s != %s", next, first)
		}
	}
}

func TestValidateManifestRejectsDuplicateAndUnknownCapabilities(t *testing.T) {
	for name, capabilities := range map[string][]Capability{
		"duplicate": {CapabilityExternalRead, CapabilityExternalRead},
		"unknown":   {Capability("root_everything")},
	} {
		t.Run(name, func(t *testing.T) {
			manifest := validPublicManifest()
			manifest.Components[0].App.Capabilities = capabilities
			if err := ValidateManifest(manifest); err == nil {
				t.Fatal("unexpected success")
			}
		})
	}
}

func TestValidateManifestRequiresPrivacyForPublicExternalCapabilities(t *testing.T) {
	manifest := validPublicManifest()
	manifest.PrivacyPolicyURL = ""
	if err := ValidateManifest(manifest); err == nil || !strings.Contains(err.Error(), "requires privacyPolicyUrl") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateManifestRejectsPublicLocalStdio(t *testing.T) {
	manifest := validPublicManifest()
	manifest.Components = []Component{{
		Kind: ComponentApp,
		ID:   "local-tool",
		App: &AppDefinition{
			Transport: TransportMCPStdioLocal,
			Command:   "node",
			Args:      []string{"server.mjs"},
			Auth:      AuthDefinition{Kind: AuthNone},
		},
	}}
	if err := ValidateManifest(manifest); err == nil || !strings.Contains(err.Error(), "public plugin cannot use mcp_stdio_local") {
		t.Fatalf("error = %v", err)
	}
}

func TestManifestHashChangesForMeaningfulManifestChange(t *testing.T) {
	before := validPublicManifest()
	after := validPublicManifest()
	after.Components[0].App.Capabilities = append(after.Components[0].App.Capabilities, CapabilityExternalDelete)
	beforeHash, err := ManifestHash(before)
	if err != nil {
		t.Fatal(err)
	}
	afterHash, err := ManifestHash(after)
	if err != nil {
		t.Fatal(err)
	}
	if beforeHash == afterHash {
		t.Fatal("hash did not change after capability change")
	}
}

func TestValidateManifestRejectsMismatchedComponentPayload(t *testing.T) {
	manifest := validPublicManifest()
	manifest.Components[0].Skill = &SkillReference{SkillID: "also-skill", Version: "1.0.0"}
	if err := ValidateManifest(manifest); err == nil || !strings.Contains(err.Error(), "must define only app") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateManifestAcceptsPrivateLocalStdio(t *testing.T) {
	manifest := validPublicManifest()
	manifest.Scope = ScopePersonal
	manifest.Distribution = DistributionPrivate
	manifest.PrivacyPolicyURL = ""
	manifest.TermsURL = ""
	manifest.Components = []Component{{
		Kind: ComponentApp,
		ID:   "local-tool",
		App: &AppDefinition{
			Transport:    TransportMCPStdioLocal,
			Command:      "node",
			Args:         []string{"server.mjs"},
			Auth:         AuthDefinition{Kind: AuthEnvReference},
			Capabilities: []Capability{CapabilityLocalShell},
		},
	}}
	if err := ValidateManifest(manifest); err != nil {
		t.Fatalf("error = %v", err)
	}
}
