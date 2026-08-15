package mcpgateway

import (
	"context"
	"errors"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	longmemory "github.com/0xmarkhydra/codelocal/internal/memory"
)

type fakeCanonicalHybridReader struct {
	readiness      cloud.KnowledgeV2Readiness
	readinessErr   error
	hits           []cloud.CanonicalKnowledgeHit
	recallErr      error
	readinessCalls int
	recallCalls    int
}

func (f *fakeCanonicalHybridReader) KnowledgeV2Readiness(_ context.Context, userID, projectID string) (cloud.KnowledgeV2Readiness, error) {
	f.readinessCalls++
	if f.readinessErr != nil {
		return cloud.KnowledgeV2Readiness{}, f.readinessErr
	}
	value := f.readiness
	value.UserID = userID
	value.ProjectID = projectID
	return value, nil
}

func (f *fakeCanonicalHybridReader) RecallCanonicalKnowledge(_ context.Context, _ cloud.CanonicalKnowledgeRecallInput) ([]cloud.CanonicalKnowledgeHit, error) {
	f.recallCalls++
	if f.recallErr != nil {
		return nil, f.recallErr
	}
	return append([]cloud.CanonicalKnowledgeHit(nil), f.hits...), nil
}

func hybridLegacyRecords(count int) []longmemory.Record {
	out := make([]longmemory.Record, 0, count)
	for index := 0; index < count; index++ {
		out = append(out, longmemory.Record{ID: "legacy-" + string(rune('a'+index)), Summary: "legacy knowledge " + string(rune('a'+index))})
	}
	return out
}

func hybridCanonicalHit(id, summary string, confidence float64) cloud.CanonicalKnowledgeHit {
	return cloud.CanonicalKnowledgeHit{
		Knowledge: cloud.CanonicalKnowledge{
			UserID: "user-a", KnowledgeID: id, ProjectID: "project-a", KnowledgeType: "decision",
			Status: cloud.KnowledgeStatusActive, PrivacyClassification: cloud.KnowledgeClassPrivateProject,
			Confidence: confidence, Importance: .8, UpdatedAt: 200,
		},
		Revision: cloud.CanonicalKnowledgeRevision{RevisionID: "rev-" + id, Summary: summary, CreatedAt: 100},
	}
}

func TestKnowledgeV2ReadModeAcceptsExplicitHybrid(t *testing.T) {
	t.Setenv("CODELOCAL_KNOWLEDGE_V2_READ_MODE", "hybrid")
	if got := knowledgeV2ReadMode(); got != knowledgeV2ReadModeHybrid {
		t.Fatalf("hybrid read mode=%q", got)
	}
}

func TestGuardedHybridDoesNotQueryCanonicalUntilReadinessIsReady(t *testing.T) {
	legacy := hybridLegacyRecords(3)
	reader := &fakeCanonicalHybridReader{readiness: cloud.KnowledgeV2Readiness{Status: "collecting"}}
	got, applied, reason := runGuardedHybridRecall(context.Background(), reader, cloud.CanonicalKnowledgeRecallInput{UserID: "user-a", ProjectID: "project-a", Limit: 6}, legacy)
	if applied || reason != "hybrid_not_ready" || reader.readinessCalls != 1 || reader.recallCalls != 0 {
		t.Fatalf("not-ready gate leaked into canonical recall: applied=%v reason=%q readiness=%d recall=%d", applied, reason, reader.readinessCalls, reader.recallCalls)
	}
	if !sameMemoryRecordIDs(got, legacy) {
		t.Fatalf("not-ready gate changed legacy output: %#v", got)
	}
}

func TestGuardedHybridSupplementsAtMostTwoHighConfidenceNonDuplicateClaims(t *testing.T) {
	legacy := hybridLegacyRecords(6)
	legacy[0].Summary = "Keep controllers thin."
	reader := &fakeCanonicalHybridReader{
		readiness: cloud.KnowledgeV2Readiness{Status: "ready"},
		hits: []cloud.CanonicalKnowledgeHit{
			hybridCanonicalHit("duplicate", " keep   controllers thin ;", .99),
			hybridCanonicalHit("low", "Low confidence canonical fact", .94),
			hybridCanonicalHit("one", "Payment writes require transactions.", .99),
			hybridCanonicalHit("two", "External API calls require timeouts.", .97),
			hybridCanonicalHit("three", "This third canonical hit must stay out.", .99),
		},
	}
	got, applied, reason := runGuardedHybridRecall(context.Background(), reader, cloud.CanonicalKnowledgeRecallInput{UserID: "user-a", ProjectID: "project-a", Limit: 6}, legacy)
	if !applied || reason != "hybrid_ready" || reader.recallCalls != 1 {
		t.Fatalf("ready hybrid did not apply: applied=%v reason=%q recall=%d", applied, reason, reader.recallCalls)
	}
	if len(got) != 6 {
		t.Fatalf("hybrid output len=%d want 6: %#v", len(got), got)
	}
	for index := 0; index < 4; index++ {
		if got[index].ID != legacy[index].ID {
			t.Fatalf("legacy relevance order changed at %d: got=%q want=%q", index, got[index].ID, legacy[index].ID)
		}
	}
	if got[4].ID != "canonical:one" || got[5].ID != "canonical:two" {
		t.Fatalf("unexpected canonical supplement: %#v", got)
	}
	for _, record := range got {
		if record.ID == "canonical:duplicate" || record.ID == "canonical:low" || record.ID == "canonical:three" {
			t.Fatalf("ineligible canonical record entered live context: %#v", record)
		}
	}
	if got[4].SourceType != "knowledge_v2" || got[4].Scope != longmemory.ScopeProject || got[4].Lifecycle != longmemory.LifecycleActive {
		t.Fatalf("canonical conversion lost durable metadata: %#v", got[4])
	}
}

func TestGuardedHybridErrorsFailSoftToLegacy(t *testing.T) {
	legacy := hybridLegacyRecords(2)
	cases := []struct {
		name   string
		reader *fakeCanonicalHybridReader
		reason string
	}{
		{name: "readiness", reader: &fakeCanonicalHybridReader{readinessErr: errors.New("health unavailable")}, reason: "hybrid_readiness_error"},
		{name: "recall", reader: &fakeCanonicalHybridReader{readiness: cloud.KnowledgeV2Readiness{Status: "ready"}, recallErr: errors.New("canonical unavailable")}, reason: "hybrid_recall_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, applied, reason := runGuardedHybridRecall(context.Background(), tc.reader, cloud.CanonicalKnowledgeRecallInput{UserID: "user-a", ProjectID: "project-a", Limit: 6}, legacy)
			if applied || reason != tc.reason || !sameMemoryRecordIDs(got, legacy) {
				t.Fatalf("hybrid failure was not fail-soft: applied=%v reason=%q got=%#v", applied, reason, got)
			}
		})
	}
}

func TestMergeHybridNoEligibleCanonicalPreservesLegacyWithoutTruncation(t *testing.T) {
	legacy := hybridLegacyRecords(6)
	hits := []cloud.CanonicalKnowledgeHit{hybridCanonicalHit("low", "not strong enough", .90)}
	got := mergeHybridCanonicalRecords(legacy, hits, 6)
	if len(got) != len(legacy) || !sameMemoryRecordIDs(got, legacy) {
		t.Fatalf("no-op hybrid unexpectedly changed legacy: %#v", got)
	}
}
