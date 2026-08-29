package skills

import (
	"strings"
	"testing"
)

func TestBuildArtifactFromDocumentsBudgetedRejectsTotalOverflow(t *testing.T) {
	manifest := Manifest{
		ID:        "budgeted",
		Name:      "Budgeted",
		Version:   "1.0.0",
		Publisher: "user",
		Scope:     ScopePersonal,
		Kind:      KindKnowledge,
		Quality:   0.5,
	}
	policy := DefaultKnowledgeIngestPolicy()
	policy.MaxDocumentBytes = 1024
	documents := []SourceDocument{
		{Path: "a.md", Content: strings.Repeat("a", 80)},
		{Path: "b.md", Content: strings.Repeat("b", 80)},
	}
	if _, err := BuildArtifactFromDocumentsBudgeted(manifest, documents, policy, 100); err == nil {
		t.Fatal("expected total ingestion budget overflow to fail")
	}
}

func TestBuildArtifactFromDocumentsBudgetedAcceptsBoundedDataset(t *testing.T) {
	manifest := Manifest{
		ID:        "bounded",
		Name:      "Bounded",
		Version:   "1.0.0",
		Publisher: "user",
		Scope:     ScopePersonal,
		Kind:      KindKnowledge,
		Quality:   0.5,
	}
	policy := DefaultKnowledgeIngestPolicy()
	policy.MaxDocumentBytes = 1024
	artifact, err := BuildArtifactFromDocumentsBudgeted(manifest, []SourceDocument{{
		Path: "knowledge.md", Content: strings.Repeat("safe ", 40),
	}}, policy, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Manifest.ChunkCount == 0 {
		t.Fatal("expected bounded dataset to produce knowledge chunks")
	}
}
