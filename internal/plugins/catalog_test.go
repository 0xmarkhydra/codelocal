package plugins

import "testing"

func TestBuiltinCatalogManifestsValidate(t *testing.T) {
	catalog := BuiltinCatalog()
	if len(catalog) < 3 {
		t.Fatalf("catalog size=%d want at least 3", len(catalog))
	}
	seen := map[string]bool{}
	for _, entry := range catalog {
		if err := ValidateManifest(entry.Manifest); err != nil {
			t.Fatalf("manifest %s invalid: %v", entry.Manifest.ID, err)
		}
		if seen[entry.Manifest.ID] {
			t.Fatalf("duplicate plugin id %q", entry.Manifest.ID)
		}
		seen[entry.Manifest.ID] = true
		if _, err := ManifestHash(entry.Manifest); err != nil {
			t.Fatalf("manifest hash failed for %s: %v", entry.Manifest.ID, err)
		}
		capabilities := ManifestCapabilities(entry.Manifest)
		if len(capabilities) == 0 {
			t.Fatalf("plugin %s exposes no capabilities", entry.Manifest.ID)
		}
	}
}

func TestManifestCapabilitiesAreUniqueAndSorted(t *testing.T) {
	entry, ok := FindBuiltin("github")
	if !ok {
		t.Fatal("github plugin missing")
	}
	capabilities := ManifestCapabilities(entry.Manifest)
	for i := 1; i < len(capabilities); i++ {
		if capabilities[i-1] >= capabilities[i] {
			t.Fatalf("capabilities not unique/sorted: %#v", capabilities)
		}
	}
}
