package skills

import "testing"

func TestArtifactIsDeterministicAndValid(t *testing.T) {
	manifest := BuiltinManifests()[0]
	registry, err := NewRegistry(manifest)
	if err != nil {
		t.Fatal(err)
	}
	chunks := BuiltinKnowledge()
	artifactA, err := BuildArtifact(manifest, chunks)
	if err != nil {
		t.Fatal(err)
	}
	reversed := append([]KnowledgeChunk(nil), chunks...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	artifactB, err := BuildArtifact(manifest, reversed)
	if err != nil {
		t.Fatal(err)
	}
	if artifactA.Manifest.ContentHash == "" || artifactA.Manifest.ContentHash != artifactB.Manifest.ContentHash {
		t.Fatalf("artifact hash must be deterministic: %q != %q", artifactA.Manifest.ContentHash, artifactB.Manifest.ContentHash)
	}
	if err := ValidateArtifact(artifactA, registry); err != nil {
		t.Fatalf("valid artifact rejected: %v", err)
	}
}

func TestArtifactRejectsTampering(t *testing.T) {
	manifest := BuiltinManifests()[0]
	registry, _ := NewRegistry(manifest)
	artifact, err := BuildArtifact(manifest, BuiltinKnowledge())
	if err != nil {
		t.Fatal(err)
	}
	artifact.Chunks[0].Content += " tampered"
	if err := ValidateArtifact(artifact, registry); err == nil {
		t.Fatal("tampered artifact must fail content hash validation")
	}
}

func TestArtifactRejectsUnpinnedSharedSkill(t *testing.T) {
	manifest := BuiltinManifests()[0]
	manifest.SourceRef = ""
	if _, err := BuildArtifact(manifest, BuiltinKnowledge()); err == nil {
		t.Fatal("shared skill without pinned source ref must be rejected")
	}
}

func TestArtifactRejectsCrossVersionKnowledge(t *testing.T) {
	manifest := BuiltinManifests()[0]
	chunks := BuiltinKnowledge()
	chunks[0].SkillVersion = "9.9.9"
	if _, err := BuildArtifact(manifest, chunks); err == nil {
		t.Fatal("artifact must reject knowledge from a different skill version")
	}
}

func TestArtifactKnowledgeStoreHydratesPortableKnowledge(t *testing.T) {
	manifest := BuiltinManifests()[0]
	registry, _ := NewRegistry(manifest)
	artifact, err := BuildArtifact(manifest, BuiltinKnowledge())
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewArtifactKnowledgeStore(registry, artifact)
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(registry, store)
	plan := engine.Plan(TaskContext{Query: "improve dashboard accessibility", Intents: []string{"design_ui", "audit_ux"}, Signals: []string{"dashboard", "accessibility"}})
	if len(plan.Knowledge) == 0 || plan.Knowledge[0].Chunk.SkillVersion != manifest.Version {
		t.Fatalf("portable store did not serve selected version: %#v", plan.Knowledge)
	}
}
