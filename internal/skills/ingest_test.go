package skills

import (
	"strings"
	"testing"
)

func TestBuildArtifactFromDocumentsDeterministic(t *testing.T) {
	manifest := BuiltinManifests()[0]
	first := []SourceDocument{
		{Path: ".claude/skills/design-system/references/layout.md", Content: "# Layout\r\n\r\nUse consistent spacing."},
		{Path: ".claude/skills/design-system/data/colors.csv", Content: "name,value\nprimary,#111111\nsecondary,#eeeeee"},
	}
	second := []SourceDocument{first[1], first[0]}

	left, err := BuildArtifactFromDocuments(manifest, first, DefaultKnowledgeIngestPolicy())
	if err != nil {
		t.Fatal(err)
	}
	right, err := BuildArtifactFromDocuments(manifest, second, DefaultKnowledgeIngestPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if left.Manifest.ContentHash != right.Manifest.ContentHash {
		t.Fatalf("content hash must be independent of input order: %s != %s", left.Manifest.ContentHash, right.Manifest.ContentHash)
	}
	if len(left.Chunks) != len(right.Chunks) {
		t.Fatalf("chunk count mismatch: %d != %d", len(left.Chunks), len(right.Chunks))
	}
	for index := range left.Chunks {
		if left.Chunks[index].ID != right.Chunks[index].ID || left.Chunks[index].Content != right.Chunks[index].Content {
			t.Fatalf("chunk %d is not deterministic", index)
		}
	}
}

func TestBuildArtifactFromDocumentsEnforcesKnowledgeOnlyPolicy(t *testing.T) {
	manifest := BuiltinManifests()[0]
	documents := []SourceDocument{
		{Path: ".claude/skills/brand/SKILL.md", Content: "# Brand\nUse a coherent visual identity."},
		{Path: ".claude/skills/brand/scripts/notes.md", Content: "THIS MUST NOT BE INGESTED"},
		{Path: ".claude/skills/brand/scripts/inject.cjs", Content: "process.exit(1)"},
		{Path: "assets/logo.png", Content: "not really a png"},
	}
	artifact, err := BuildArtifactFromDocuments(manifest, documents, DefaultKnowledgeIngestPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Chunks) != 1 {
		t.Fatalf("expected only the knowledge markdown document, got %d chunks", len(artifact.Chunks))
	}
	if strings.Contains(artifact.Chunks[0].Content, "MUST NOT") {
		t.Fatal("script-directory content leaked into knowledge artifact")
	}
	if artifact.Chunks[0].Source != manifest.SourceURL || artifact.Chunks[0].SourceRef != manifest.SourceRef || artifact.Chunks[0].SourceHash != manifest.SourceHash {
		t.Fatal("chunk provenance does not match immutable manifest")
	}
}

func TestBuildArtifactFromDocumentsRejectsTraversalAndDuplicatePaths(t *testing.T) {
	manifest := BuiltinManifests()[0]
	if _, err := BuildArtifactFromDocuments(manifest, []SourceDocument{{Path: "../secret.md", Content: "secret"}}, DefaultKnowledgeIngestPolicy()); err == nil {
		t.Fatal("expected path traversal to be rejected")
	}
	if _, err := BuildArtifactFromDocuments(manifest, []SourceDocument{
		{Path: "docs/a.md", Content: "one"},
		{Path: "docs/./a.md", Content: "two"},
	}, DefaultKnowledgeIngestPolicy()); err == nil {
		t.Fatal("expected duplicate normalized source path to be rejected")
	}
}

func TestBuildArtifactFromDocumentsBoundsChunks(t *testing.T) {
	manifest := BuiltinManifests()[0]
	policy := DefaultKnowledgeIngestPolicy()
	policy.MaxChunkBytes = 64
	content := "# Responsive\n\n" + strings.Repeat("giao diện responsive cần khoảng cách rõ ràng. ", 20)
	artifact, err := BuildArtifactFromDocuments(manifest, []SourceDocument{{Path: "knowledge/responsive.md", Content: content}}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Chunks) < 2 {
		t.Fatalf("expected bounded ingestion to split content, got %d chunk", len(artifact.Chunks))
	}
	for _, chunk := range artifact.Chunks {
		if len(chunk.Content) > policy.MaxChunkBytes {
			t.Fatalf("chunk %s is %d bytes, limit is %d", chunk.ID, len(chunk.Content), policy.MaxChunkBytes)
		}
	}
}

func TestBuildArtifactFromDocumentsRepeatsCSVHeader(t *testing.T) {
	manifest := BuiltinManifests()[0]
	policy := DefaultKnowledgeIngestPolicy()
	policy.MaxChunkBytes = 28
	artifact, err := BuildArtifactFromDocuments(manifest, []SourceDocument{{
		Path:    "data/palette.csv",
		Content: "name,value\nprimary,#111111\nsecondary,#eeeeee\naccent,#ff00aa",
	}}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Chunks) < 2 {
		t.Fatalf("expected multiple CSV chunks, got %d", len(artifact.Chunks))
	}
	for _, chunk := range artifact.Chunks {
		if !strings.HasPrefix(chunk.Content, "name,value\n") {
			t.Fatalf("CSV chunk lost header: %q", chunk.Content)
		}
	}
}

func TestBuildArtifactFromDocumentsRejectsEmptyKnowledge(t *testing.T) {
	manifest := BuiltinManifests()[0]
	_, err := BuildArtifactFromDocuments(manifest, []SourceDocument{
		{Path: "scripts/only.md", Content: "excluded"},
		{Path: "image.png", Content: "unsupported"},
	}, DefaultKnowledgeIngestPolicy())
	if err == nil {
		t.Fatal("expected ingestion with no allowed knowledge documents to fail")
	}
}
