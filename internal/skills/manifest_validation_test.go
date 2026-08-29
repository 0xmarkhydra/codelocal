package skills

import "testing"

func TestManifestRejectsNonCanonicalIdentity(t *testing.T) {
	base := BuiltinManifests()[0]
	for _, tc := range []struct {
		name string
		edit func(*Manifest)
	}{
		{name: "id whitespace", edit: func(m *Manifest) { m.ID = " " + m.ID }},
		{name: "version whitespace", edit: func(m *Manifest) { m.Version += " " }},
		{name: "name newline", edit: func(m *Manifest) { m.Name += "\nspoof" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifest := base
			tc.edit(&manifest)
			if err := manifest.Validate(); err == nil {
				t.Fatalf("non-canonical manifest accepted: %#v", manifest)
			}
		})
	}
}

func TestManifestRejectsUnknownAndDuplicateCapabilities(t *testing.T) {
	base := BuiltinManifests()[0]
	base.Kind = KindHybrid
	base.Capabilities = []Capability{CapabilityShell, Capability("root_everything")}
	if err := base.Validate(); err == nil {
		t.Fatal("unknown capability must be rejected instead of receiving implicit risk semantics")
	}

	base.Capabilities = []Capability{CapabilityShell, CapabilityShell}
	if err := base.Validate(); err == nil {
		t.Fatal("duplicate capabilities must be rejected")
	}
}
