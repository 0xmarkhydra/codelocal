package mcpgateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

type fakeCanonicalSemanticHybridReader struct {
	fakeCanonicalHybridReader
	semanticReadiness      cloud.CanonicalSemanticReadiness
	semanticReadinessErr   error
	semanticHits           []cloud.CanonicalSemanticKnowledgeHit
	semanticRecallErr      error
	semanticReadinessCalls int
	semanticRecallCalls    int
}

func (f *fakeCanonicalSemanticHybridReader) CanonicalSemanticReadiness(_ context.Context, userID, projectID string) (cloud.CanonicalSemanticReadiness, error) {
	f.semanticReadinessCalls++
	if f.semanticReadinessErr != nil {
		return cloud.CanonicalSemanticReadiness{}, f.semanticReadinessErr
	}
	value := f.semanticReadiness
	value.UserID, value.ProjectID = userID, projectID
	return value, nil
}

func (f *fakeCanonicalSemanticHybridReader) RecallCanonicalKnowledgeSemantic(_ context.Context, _ cloud.CanonicalKnowledgeRecallInput, _ string) ([]cloud.CanonicalSemanticKnowledgeHit, error) {
	f.semanticRecallCalls++
	if f.semanticRecallErr != nil {
		return nil, f.semanticRecallErr
	}
	return append([]cloud.CanonicalSemanticKnowledgeHit(nil), f.semanticHits...), nil
}

func semanticHybridHit(id, summary string, confidence, similarity float64) cloud.CanonicalSemanticKnowledgeHit {
	return cloud.CanonicalSemanticKnowledgeHit{CanonicalKnowledgeHit: hybridCanonicalHit(id, summary, confidence), Similarity: similarity}
}

func TestCanonicalSemanticHybridDefaultsOff(t *testing.T) {
	t.Setenv("CODELOCAL_CANONICAL_SEMANTIC_HYBRID", "")
	if canonicalSemanticHybridEnabled() {
		t.Fatal("semantic hybrid canary must default off")
	}
	t.Setenv("CODELOCAL_CANONICAL_SEMANTIC_HYBRID", "1")
	if !canonicalSemanticHybridEnabled() {
		t.Fatal("semantic hybrid canary did not honor explicit opt-in")
	}
	if knowledgeV2SemanticHybridTimeout != cloud.CanonicalEmbeddingShadowSlowDuration+50*time.Millisecond {
		t.Fatalf("semantic live timeout=%s shadow slow threshold=%s", knowledgeV2SemanticHybridTimeout, cloud.CanonicalEmbeddingShadowSlowDuration)
	}
}

func TestGuardedSemanticHybridRequiresReadinessBeforeSemanticRecall(t *testing.T) {
	legacy := hybridLegacyRecords(3)
	reader := &fakeCanonicalSemanticHybridReader{semanticReadiness: cloud.CanonicalSemanticReadiness{Status: "collecting"}}
	got, applied, reason := runGuardedSemanticHybridRecall(context.Background(), reader, cloud.CanonicalKnowledgeRecallInput{UserID: "user-a", ProjectID: "project-a", Limit: 6}, "payment timeout", legacy)
	if applied || reason != "semantic_hybrid_not_ready" || reader.semanticReadinessCalls != 1 || reader.semanticRecallCalls != 0 || !sameMemoryRecordIDs(got, legacy) {
		t.Fatalf("semantic readiness gate failed closed incorrectly: applied=%v reason=%q readiness=%d recall=%d got=%#v", applied, reason, reader.semanticReadinessCalls, reader.semanticRecallCalls, got)
	}
}

func TestGuardedSemanticHybridUsesOnlyHighSimilarityHighConfidenceClaimsAndKeepsMaxTwo(t *testing.T) {
	legacy := hybridLegacyRecords(6)
	legacy[0].Summary = "Keep controllers thin."
	reader := &fakeCanonicalSemanticHybridReader{
		semanticReadiness: cloud.CanonicalSemanticReadiness{Status: "ready"},
		semanticHits: []cloud.CanonicalSemanticKnowledgeHit{
			semanticHybridHit("duplicate", " keep controllers thin ;", .99, .95),
			semanticHybridHit("weak-sim", "Weak semantic similarity", .99, .79),
			semanticHybridHit("weak-confidence", "Weak canonical confidence", .94, .99),
			semanticHybridHit("one", "Payment writes require transactions.", .99, .93),
			semanticHybridHit("two", "External API calls require timeouts.", .97, .88),
			semanticHybridHit("three", "Third semantic claim stays out.", .99, .99),
		},
	}
	got, applied, reason := runGuardedSemanticHybridRecall(context.Background(), reader, cloud.CanonicalKnowledgeRecallInput{UserID: "user-a", ProjectID: "project-a", Limit: 6}, "payment timeout", legacy)
	if !applied || reason != "semantic_hybrid_ready" || reader.semanticRecallCalls != 1 || len(got) != 6 {
		t.Fatalf("semantic hybrid did not apply safely: applied=%v reason=%q recall=%d got=%#v", applied, reason, reader.semanticRecallCalls, got)
	}
	if got[4].ID != "canonical:one" || got[5].ID != "canonical:two" {
		t.Fatalf("unexpected semantic canonical supplements: %#v", got)
	}
	for _, record := range got {
		if record.ID == "canonical:duplicate" || record.ID == "canonical:weak-sim" || record.ID == "canonical:weak-confidence" || record.ID == "canonical:three" {
			t.Fatalf("ineligible semantic canonical record entered live context: %#v", record)
		}
	}
}

func TestGuardedSemanticHybridErrorsFailSoftToLegacy(t *testing.T) {
	legacy := hybridLegacyRecords(2)
	cases := []struct {
		name   string
		reader *fakeCanonicalSemanticHybridReader
		reason string
	}{
		{name: "readiness", reader: &fakeCanonicalSemanticHybridReader{semanticReadinessErr: errors.New("metrics unavailable")}, reason: "semantic_hybrid_readiness_error"},
		{name: "recall", reader: &fakeCanonicalSemanticHybridReader{semanticReadiness: cloud.CanonicalSemanticReadiness{Status: "ready"}, semanticRecallErr: errors.New("provider unavailable")}, reason: "semantic_hybrid_recall_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, applied, reason := runGuardedSemanticHybridRecall(context.Background(), tc.reader, cloud.CanonicalKnowledgeRecallInput{UserID: "user-a", ProjectID: "project-a", Limit: 6}, "query", legacy)
			if applied || reason != tc.reason || !sameMemoryRecordIDs(got, legacy) {
				t.Fatalf("semantic hybrid failure was not fail-soft: applied=%v reason=%q got=%#v", applied, reason, got)
			}
		})
	}
}
