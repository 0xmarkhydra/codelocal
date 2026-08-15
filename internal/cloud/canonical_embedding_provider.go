package cloud

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	longmemory "github.com/0xmarkhydra/codelocal/internal/memory"
)

type memoryCanonicalEmbeddingProvider struct {
	embedder longmemory.Embedder
	version  string
}

func (p *memoryCanonicalEmbeddingProvider) Model() CanonicalEmbeddingModel {
	return CanonicalEmbeddingModel{Provider: p.embedder.Name(), Model: p.embedder.Model(), Version: p.version}
}

func (p *memoryCanonicalEmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return p.embedder.Embed(ctx, texts)
}

func (p *memoryCanonicalEmbeddingProvider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	query, ok := p.embedder.(longmemory.QueryEmbedder)
	if !ok {
		return nil, errors.New("canonical embedding provider does not support query embeddings")
	}
	return query.EmbedQuery(ctx, text)
}

func canonicalEmbeddingProviderFromEnv() (CanonicalEmbeddingProvider, error) {
	if !canonicalEmbeddingEnabled() {
		return nil, nil
	}
	version := strings.TrimSpace(os.Getenv("CODELOCAL_CANONICAL_EMBEDDING_MODEL_VERSION"))
	if version == "" {
		return nil, errors.New("CODELOCAL_CANONICAL_EMBEDDING_MODEL_VERSION is required when canonical embeddings are enabled")
	}
	embedder := longmemory.EmbedderFromEnv()
	if embedder == nil {
		return nil, errors.New("canonical embeddings are enabled but no embedding provider is configured")
	}
	provider := &memoryCanonicalEmbeddingProvider{embedder: embedder, version: version}
	if _, ok := normalizeCanonicalEmbeddingModel(provider.Model()); !ok {
		return nil, errors.New("canonical embedding provider metadata is invalid")
	}
	return provider, nil
}

func canonicalEmbeddingMaintenanceTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv("CODELOCAL_CANONICAL_EMBEDDING_MAINTENANCE_TIMEOUT"))
	if value == "" {
		return 12 * time.Second
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed < time.Second || parsed > 30*time.Second {
		return 12 * time.Second
	}
	return parsed
}

func (s *Store) ensureCanonicalEmbeddingVectorSchema(ctx context.Context) error {
	if s == nil || s.DB == nil || s.canonicalEmbeddingProvider == nil || !canonicalEmbeddingEnabled() {
		return nil
	}
	_, err := s.DB.Exec(ctx, canonicalKnowledgeEmbeddingVectorSchemaSQL)
	return err
}
