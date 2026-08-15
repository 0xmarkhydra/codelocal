package cloud

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
)

type fakeCanonicalEmbeddingProvider struct {
	model  CanonicalEmbeddingModel
	vector []float32
	err    error
	calls  int
}

func (f *fakeCanonicalEmbeddingProvider) Model() CanonicalEmbeddingModel { return f.model }
func (f *fakeCanonicalEmbeddingProvider) Embed(_ context.Context, _ string) ([]float32, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return append([]float32(nil), f.vector...), nil
}

func TestCanonicalEmbeddingSourceTextIsDeterministicAndSecretSafe(t *testing.T) {
	text, ok := canonicalEmbeddingSourceText("decision", "database-writes", "Use transactional outbox for durable writes.")
	if !ok || text != "type: decision\nkey: database-writes\nsummary: Use transactional outbox for durable writes." {
		t.Fatalf("unexpected canonical embedding source: %q ok=%v", text, ok)
	}
	if hash := canonicalEmbeddingContentHash(text); len(hash) != 64 || hash != canonicalEmbeddingContentHash(text) {
		t.Fatalf("unstable embedding content hash: %q", hash)
	}

	withoutSecretKey, ok := canonicalEmbeddingSourceText("fact", "api_key=super-secret-token", "Use bounded retries.")
	if !ok || strings.Contains(strings.ToLower(withoutSecretKey), "api_key") || strings.Contains(withoutSecretKey, "super-secret-token") {
		t.Fatalf("secret-like stable key entered embedding text: %q ok=%v", withoutSecretKey, ok)
	}
	if _, ok := canonicalEmbeddingSourceText("fact", "safe-key", "Authorization: Bearer super-secret-token"); ok {
		t.Fatal("secret-like canonical summary must not be embedded")
	}
}

func TestCanonicalEmbeddingVectorLiteralValidatesDimensionsAndFiniteValues(t *testing.T) {
	literal, ok := canonicalEmbeddingVectorLiteral([]float32{0.25, -1.5, 2})
	if !ok || literal != "[0.25,-1.5,2]" {
		t.Fatalf("unexpected vector literal: %q ok=%v", literal, ok)
	}
	for _, vector := range [][]float32{
		nil,
		{float32(math.Inf(1))},
		{float32(math.NaN())},
	} {
		if _, ok := canonicalEmbeddingVectorLiteral(vector); ok {
			t.Fatalf("invalid vector accepted: %#v", vector)
		}
	}
}

func TestCanonicalEmbeddingModelMetadataFailsClosed(t *testing.T) {
	valid, ok := normalizeCanonicalEmbeddingModel(CanonicalEmbeddingModel{Provider: " Local ", Model: "semantic-v1", Version: "2026-08"})
	if !ok || valid.Provider != "local" || valid.Model != "semantic-v1" || valid.Version != "2026-08" {
		t.Fatalf("valid model metadata rejected: %#v ok=%v", valid, ok)
	}
	for _, model := range []CanonicalEmbeddingModel{
		{},
		{Provider: "local", Model: "", Version: "1"},
		{Provider: "local", Model: "semantic", Version: "api_key=secret"},
	} {
		if _, ok := normalizeCanonicalEmbeddingModel(model); ok {
			t.Fatalf("unsafe/incomplete model metadata accepted: %#v", model)
		}
	}
}

func TestCanonicalEmbeddingFeatureDefaultsOff(t *testing.T) {
	t.Setenv("CODELOCAL_CANONICAL_EMBEDDINGS", "")
	if canonicalEmbeddingEnabled() {
		t.Fatal("canonical embeddings must be default-off")
	}
	store := &Store{}
	provider := &fakeCanonicalEmbeddingProvider{model: CanonicalEmbeddingModel{Provider: "local", Model: "semantic", Version: "1"}, vector: []float32{1, 2}}
	stats, err := store.RebuildCanonicalKnowledgeEmbeddingsProject(context.Background(), provider, "user", "project")
	if err != nil || stats.Status != "disabled" || provider.calls != 0 {
		t.Fatalf("disabled projection performed work: stats=%#v calls=%d err=%v", stats, provider.calls, err)
	}
}

func TestCanonicalEmbeddingMigrationStoresModelDimensionRevisionAndDerivedState(t *testing.T) {
	lower := strings.ToLower(canonicalKnowledgeEmbeddingMigrationSQL)
	for _, required := range []string{
		"codelocal_knowledge_embeddings",
		"revision_id text not null",
		"provider text not null",
		"model text not null",
		"model_version text not null",
		"dimensions integer not null",
		"content_hash text not null",
		"embedding vector not null",
		"references codelocal_knowledge_objects(user_id,knowledge_id)",
		"references codelocal_knowledge_revisions(user_id,revision_id)",
		"codelocal_knowledge_embedding_projection_state",
		"source_revision_count",
		"projected_revision_count",
		"projected_at",
	} {
		if !strings.Contains(lower, required) {
			t.Fatalf("canonical embedding migration missing %q", required)
		}
	}
	for _, forbidden := range []string{"drop table", "truncate table"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("canonical embedding migration unexpectedly destructive: %q", forbidden)
		}
	}
}

func TestCanonicalEmbeddingSourceQueryUsesOnlyActivePrivateCanonicalRevision(t *testing.T) {
	lower := strings.ToLower(canonicalEmbeddingSourceSQL)
	for _, required := range []string{
		"o.user_id=$1",
		"o.project_id=$2",
		"o.privacy_classification='private_project'",
		"o.status='active'",
		"r.revision_id=o.active_revision_id",
		"limit $3",
	} {
		if !strings.Contains(lower, required) {
			t.Fatalf("canonical embedding source query missing %q: %s", required, lower)
		}
	}
}

func TestCanonicalEmbeddingUpsertIsRevisionAndModelScoped(t *testing.T) {
	lower := strings.ToLower(canonicalEmbeddingUpsertSQL)
	for _, required := range []string{
		"on conflict(user_id,project_id,revision_id,provider,model,model_version)",
		"content_hash is distinct from excluded.content_hash",
		"dimensions is distinct from excluded.dimensions",
		"$10::vector",
	} {
		if !strings.Contains(lower, required) {
			t.Fatalf("canonical embedding upsert missing %q: %s", required, lower)
		}
	}
}

func TestCanonicalEmbeddingProviderErrorContract(t *testing.T) {
	provider := &fakeCanonicalEmbeddingProvider{err: errors.New("provider unavailable")}
	if _, err := provider.Embed(context.Background(), "safe"); err == nil || provider.calls != 1 {
		t.Fatalf("fake provider contract broken: calls=%d err=%v", provider.calls, err)
	}
}
