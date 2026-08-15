package mcpgateway

import (
	"errors"
	"testing"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func TestCanonicalSemanticShadowDefaultsOffAndRequiresShadowReadMode(t *testing.T) {
	t.Setenv("CODELOCAL_CANONICAL_EMBEDDING_SHADOW", "")
	if canonicalSemanticShadowEnabled() {
		t.Fatal("canonical semantic shadow must default off")
	}
	t.Setenv("CODELOCAL_CANONICAL_EMBEDDING_SHADOW", "1")
	if !canonicalSemanticShadowEnabled() {
		t.Fatal("canonical semantic shadow did not honor explicit opt-in")
	}
	t.Setenv("CODELOCAL_KNOWLEDGE_V2_READ_MODE", "hybrid")
	if knowledgeV2ReadMode() == knowledgeV2ReadModeShadow {
		t.Fatal("test expected hybrid mode")
	}
}

func TestCanonicalSemanticShadowSampleStoresOnlyAggregateCounts(t *testing.T) {
	input := cloud.CanonicalKnowledgeRecallInput{UserID: "user-a", ProjectID: "project-a"}
	hits := []cloud.CanonicalSemanticKnowledgeHit{
		{Similarity: .91}, {Similarity: .80}, {Similarity: .79},
	}
	sample := canonicalSemanticShadowSample(input, hits, 321*time.Millisecond, nil)
	if sample.UserID != "user-a" || sample.ProjectID != "project-a" || sample.SemanticHits != 3 || sample.HighSimilarityHits != 2 || sample.DurationMillis != 321 || sample.Errored {
		t.Fatalf("unexpected semantic shadow sample: %#v", sample)
	}
	failed := canonicalSemanticShadowSample(input, nil, 25*time.Millisecond, errors.New("boom"))
	if !failed.Errored || failed.SemanticHits != 0 || failed.HighSimilarityHits != 0 {
		t.Fatalf("unexpected failed semantic shadow sample: %#v", failed)
	}
}
