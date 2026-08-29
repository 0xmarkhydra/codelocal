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

func TestArtifactDeduplicatesRepeatedKnowledgeDeterministically(t *testing.T) {
	manifest := Manifest{
		ID:        "dedupe",
		Name:      "Dedupe",
		Version:   "1.0.0",
		Publisher: "user",
		Scope:     ScopePersonal,
		Kind:      KindKnowledge,
		Quality:   0.5,
	}
	chunks := []KnowledgeChunk{
		{ID: "z-copy", SkillID: manifest.ID, SkillVersion: manifest.Version, Content: "Same reusable rule.", Tags: []string{"second"}},
		{ID: "a-copy", SkillID: manifest.ID, SkillVersion: manifest.Version, Content: "Same reusable rule.", Tags: []string{"first"}},
	}
	artifact, err := BuildArtifact(manifest, chunks)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Manifest.ChunkCount != 1 || len(artifact.Chunks) != 1 {
		t.Fatalf("expected one canonical chunk, got %#v", artifact.Manifest)
	}
	chunk := artifact.Chunks[0]
	if chunk.ID != "a-copy" {
		t.Fatalf("expected deterministic lowest-id representative, got %q", chunk.ID)
	}
	if len(chunk.Tags) != 2 || chunk.Tags[0] != "first" || chunk.Tags[1] != "second" {
		t.Fatalf("expected merged tags, got %#v", chunk.Tags)
	}
	reversed := []KnowledgeChunk{chunks[1], chunks[0]}
	artifactB, err := BuildArtifact(manifest, reversed)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Manifest.ContentHash != artifactB.Manifest.ContentHash {
		t.Fatal("deduplication must be deterministic across source order")
	}
}

func TestArtifactValidationRejectsInjectedDuplicateContent(t *testing.T) {
	manifest := Manifest{
		ID:        "dedupe-validation",
		Name:      "Dedupe Validation",
		Version:   "1.0.0",
		Publisher: "user",
		Scope:     ScopePersonal,
		Kind:      KindKnowledge,
		Quality:   0.5,
	}
	registry, _ := NewRegistry(manifest)
	artifact, err := BuildArtifact(manifest, []KnowledgeChunk{{
		ID: "one", SkillID: manifest.ID, SkillVersion: manifest.Version, Content: "Unique rule.",
	}})
	if err != nil {
		t.Fatal(err)
	}
	duplicate := artifact.Chunks[0]
	duplicate.ID = "two"
	artifact.Chunks = append(artifact.Chunks, duplicate)
	artifact.Manifest.ChunkCount = len(artifact.Chunks)
	if err := ValidateArtifact(artifact, registry); err == nil {
		t.Fatal("artifact validation must reject injected duplicate knowledge")
	}
}
