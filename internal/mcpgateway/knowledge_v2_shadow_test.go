package mcpgateway

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	longmemory "github.com/0xmarkhydra/codelocal/internal/memory"
)

type fakeCanonicalKnowledgeReader struct {
	hits  []cloud.CanonicalKnowledgeHit
	err   error
	input cloud.CanonicalKnowledgeRecallInput
}

func (f *fakeCanonicalKnowledgeReader) RecallCanonicalKnowledge(_ context.Context, input cloud.CanonicalKnowledgeRecallInput) ([]cloud.CanonicalKnowledgeHit, error) {
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	return append([]cloud.CanonicalKnowledgeHit(nil), f.hits...), nil
}

func TestKnowledgeV2ReadModeDefaultsToShadowAndInvalidFailsClosed(t *testing.T) {
	t.Setenv("CODELOCAL_KNOWLEDGE_V2_READ_MODE", "")
	if got := knowledgeV2ReadMode(); got != knowledgeV2ReadModeShadow {
		t.Fatalf("default knowledge v2 read mode=%q want shadow", got)
	}
	t.Setenv("CODELOCAL_KNOWLEDGE_V2_READ_MODE", "off")
	if got := knowledgeV2ReadMode(); got != knowledgeV2ReadModeOff {
		t.Fatalf("explicit off mode=%q", got)
	}
	t.Setenv("CODELOCAL_KNOWLEDGE_V2_READ_MODE", "prefer-v2")
	if got := knowledgeV2ReadMode(); got != knowledgeV2ReadModeOff {
		t.Fatalf("unsupported read mode must fail closed, got %q", got)
	}
}

func TestCanonicalShadowRecallDoesNotMutateLegacyOutput(t *testing.T) {
	legacy := []longmemory.Record{{ID: "legacy-a", Summary: "Keep legacy output exactly as ranked."}}
	before := append([]longmemory.Record(nil), legacy...)
	reader := &fakeCanonicalKnowledgeReader{hits: []cloud.CanonicalKnowledgeHit{
		{Knowledge: cloud.CanonicalKnowledge{KnowledgeID: "knw-a", RepositoryID: "repo-a", Branch: "feat/a", Confidence: .97}},
		{Knowledge: cloud.CanonicalKnowledge{KnowledgeID: "knw-b", Confidence: .91}},
	}}
	input := cloud.CanonicalKnowledgeRecallInput{UserID: "user-a", ProjectID: "project-a", RepositoryIDs: []string{"repo-a"}, Branch: "feat/a", Limit: 6}
	metrics, err := runCanonicalShadowRecall(context.Background(), reader, input, len(legacy))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(legacy, before) {
		t.Fatalf("shadow recall mutated legacy output: before=%#v after=%#v", before, legacy)
	}
	if !reflect.DeepEqual(reader.input, input) {
		t.Fatalf("shadow recall changed resolved scope: got=%#v want=%#v", reader.input, input)
	}
	if metrics.LegacyCount != 1 || metrics.CanonicalCount != 2 || metrics.RepositoryScoped != 1 || metrics.BranchScoped != 1 || metrics.HighConfidence != 1 {
		t.Fatalf("unexpected compact shadow metrics: %#v", metrics)
	}
}

func TestCanonicalShadowMetricsContainCountsNotKnowledgeText(t *testing.T) {
	hits := []cloud.CanonicalKnowledgeHit{{Knowledge: cloud.CanonicalKnowledge{RepositoryID: "repo-secret", Branch: "private-branch", Confidence: .99}}}
	metrics := canonicalShadowMetricsForHits(3, hits, 12*time.Millisecond)
	if metrics.LegacyCount != 3 || metrics.CanonicalCount != 1 || metrics.DurationMilliseconds != 12 {
		t.Fatalf("unexpected shadow metrics: %#v", metrics)
	}
	// The metric schema intentionally has no summary, stable key, repository id,
	// branch name or other private knowledge payload fields.
	shape := reflect.TypeOf(metrics)
	allowed := map[string]bool{
		"LegacyCount": true, "CanonicalCount": true, "RepositoryScoped": true,
		"BranchScoped": true, "HighConfidence": true, "DurationMilliseconds": true,
	}
	if shape.NumField() != len(allowed) {
		t.Fatalf("shadow metric schema unexpectedly grew: %d fields", shape.NumField())
	}
	for i := 0; i < shape.NumField(); i++ {
		if !allowed[shape.Field(i).Name] {
			t.Fatalf("shadow metric leaked unexpected field %q", shape.Field(i).Name)
		}
	}
}

func TestCanonicalShadowFailureStillProducesAggregateErrorSample(t *testing.T) {
	reader := &fakeCanonicalKnowledgeReader{err: errors.New("shadow unavailable")}
	input := cloud.CanonicalKnowledgeRecallInput{UserID: "user-a", ProjectID: "project-a", Limit: 6}
	metrics, err := runCanonicalShadowRecall(context.Background(), reader, input, 4)
	if err == nil {
		t.Fatal("expected shadow recall error")
	}
	if metrics.LegacyCount != 4 || metrics.CanonicalCount != 0 || metrics.DurationMilliseconds < 0 {
		t.Fatalf("error path lost bounded aggregate metrics: %#v", metrics)
	}
	sample := canonicalShadowSample(input, metrics, err)
	if !sample.Errored || sample.UserID != "user-a" || sample.ProjectID != "project-a" || sample.LegacyCount != 4 || sample.CanonicalCount != 0 {
		t.Fatalf("shadow failure sample is not compact/project-scoped: %#v", sample)
	}
}

func TestCanonicalShadowSuccessSampleContainsOnlyCounts(t *testing.T) {
	input := cloud.CanonicalKnowledgeRecallInput{UserID: "user-a", ProjectID: "project-a", RepositoryIDs: []string{"private-repo"}, Branch: "secret-branch", Limit: 6}
	metrics := canonicalShadowMetrics{LegacyCount: 2, CanonicalCount: 3, HighConfidence: 2, DurationMilliseconds: 11}
	sample := canonicalShadowSample(input, metrics, nil)
	if sample.UserID != "user-a" || sample.ProjectID != "project-a" || sample.LegacyCount != 2 || sample.CanonicalCount != 3 || sample.HighConfidenceHits != 2 || sample.DurationMillis != 11 || sample.Errored {
		t.Fatalf("unexpected success shadow sample: %#v", sample)
	}
	shape := reflect.TypeOf(sample)
	for _, forbidden := range []string{"Summary", "StableKey", "RepositoryID", "RepositoryIDs", "Branch", "Subject", "Object"} {
		if _, ok := shape.FieldByName(forbidden); ok {
			t.Fatalf("shadow sample leaked private field %q", forbidden)
		}
	}
}
