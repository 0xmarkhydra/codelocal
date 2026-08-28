package skills

import "testing"

func TestPackageImporterActivatesPersonalSkill(t *testing.T) {
	manifest := Manifest{
		ID:        "personal-copy",
		Name:      "Personal Copy",
		Version:   "1.0.0",
		Publisher: "user",
		Scope:     ScopePersonal,
		Kind:      KindKnowledge,
		Quality:   0.5,
	}
	pkg := testPackage(t, manifest)
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	importer, err := NewPackageImporter(registry)
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Import(pkg, UserImportPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != ImportActive {
		t.Fatalf("expected active personal import, got %s", result.Disposition)
	}
	if current, ok := registry.Get(manifest.ID); !ok || current.Version != manifest.Version {
		t.Fatal("personal import should become current in the private registry")
	}
}

func TestPackageImporterStagesCommunitySkill(t *testing.T) {
	manifest := Manifest{
		ID:         "community-review",
		Name:       "Community Review",
		Version:    "1.0.0",
		Publisher:  "alice",
		Scope:      ScopeCommunity,
		Kind:       KindKnowledge,
		Quality:    0.5,
		SourceURL:  "https://github.com/example/community-review",
		SourceRef:  "abc123",
		SourceHash: "tree123",
		License:    "MIT",
	}
	pkg := testPackage(t, manifest)
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	importer, err := NewPackageImporter(registry)
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Import(pkg, UserImportPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != ImportCandidate {
		t.Fatalf("expected candidate community import, got %s", result.Disposition)
	}
	if _, ok := registry.Get(manifest.ID); ok {
		t.Fatal("community candidate must not enter stable routing before promotion")
	}
	if _, ok := registry.GetVersion(manifest.ID, manifest.Version); !ok {
		t.Fatal("community candidate version should remain reviewable")
	}
}

func TestPackageImporterRejectsUserSystemSkill(t *testing.T) {
	manifest := BuiltinManifests()[0]
	pkg, err := BuildPackage(manifest, mustBuiltinArtifact(t, manifest))
	if err != nil {
		t.Fatal(err)
	}
	registry, _ := NewRegistry()
	importer, _ := NewPackageImporter(registry)
	if _, err := importer.Import(pkg, UserImportPolicy()); err == nil {
		t.Fatal("user import must not create system skill")
	}
}

func testPackage(t *testing.T, manifest Manifest) Package {
	t.Helper()
	chunk := KnowledgeChunk{
		ID:           "knowledge",
		SkillID:      manifest.ID,
		SkillVersion: manifest.Version,
		Content:      "Reusable knowledge.",
		Source:       manifest.SourceURL,
		SourceRef:    manifest.SourceRef,
		SourceHash:   manifest.SourceHash,
	}
	artifact, err := BuildArtifact(manifest, []KnowledgeChunk{chunk})
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := BuildPackage(manifest, artifact)
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

func mustBuiltinArtifact(t *testing.T, manifest Manifest) Artifact {
	t.Helper()
	artifact, err := BuildArtifact(manifest, BuiltinKnowledge())
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}
