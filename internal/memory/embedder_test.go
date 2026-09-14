package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGeminiEmbedderBatchResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		var payload struct {
			Requests []struct {
				TaskType             string `json:"taskType"`
				OutputDimensionality int    `json:"outputDimensionality"`
			} `json:"requests"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Requests) != 2 {
			t.Fatalf("request count=%d want=2", len(payload.Requests))
		}
		if payload.Requests[0].TaskType != "RETRIEVAL_DOCUMENT" || payload.Requests[0].OutputDimensionality != 768 {
			t.Fatalf("unexpected embedding request: %#v", payload.Requests[0])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embeddings":[{"values":[0.1,0.2]},{"values":[0.3,0.4]}]}`))
	}))
	defer server.Close()

	embedder, err := NewGeminiEmbedder("test-key", "gemini-embedding-001", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	embedder.client = server.Client()
	// Override transport so the production URL is routed to the local server.
	baseTransport := embedder.client.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}
	embedder.client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		clone := req.Clone(req.Context())
		clone.URL.Scheme = "http"
		clone.URL.Host = strings.TrimPrefix(server.URL, "http://")
		return baseTransport.RoundTrip(clone)
	})
	vectors, err := embedder.Embed(context.Background(), []string{"first", "second"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 2 || len(vectors[0]) != 2 || vectors[1][1] != 0.4 {
		t.Fatalf("unexpected vectors: %#v", vectors)
	}
}

func TestGeminiEmbedderQueryUsesRetrievalQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Requests []struct {
				TaskType             string `json:"taskType"`
				OutputDimensionality int    `json:"outputDimensionality"`
			} `json:"requests"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Requests) != 1 || payload.Requests[0].TaskType != "RETRIEVAL_QUERY" || payload.Requests[0].OutputDimensionality != 768 {
			t.Fatalf("unexpected query embedding request: %#v", payload.Requests)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embeddings":[{"values":[0.5,0.6]}]}`))
	}))
	defer server.Close()

	embedder, err := NewGeminiEmbedder("test-key", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if embedder.Model() != "gemini-embedding-2" {
		t.Fatalf("unexpected default model: %s", embedder.Model())
	}
	baseTransport := server.Client().Transport
	embedder.client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		clone := req.Clone(req.Context())
		clone.URL.Scheme = "http"
		clone.URL.Host = strings.TrimPrefix(server.URL, "http://")
		return baseTransport.RoundTrip(clone)
	})
	vector, err := embedder.EmbedQuery(context.Background(), "find similar oauth issue")
	if err != nil {
		t.Fatal(err)
	}
	if len(vector) != 2 || vector[1] != 0.6 {
		t.Fatalf("unexpected query vector: %#v", vector)
	}
}

func TestOpenRouterEmbedderUsesGeminiEmbeddingAndSearchTypes(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if got := r.Header.Get("Authorization"); got != "Bearer test-openrouter-key" {
			t.Fatalf("unexpected auth header")
		}
		var payload struct {
			Model      string   `json:"model"`
			Input      []string `json:"input"`
			Dimensions int      `json:"dimensions"`
			InputType  string   `json:"input_type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		wantType := "search_document"
		if calls == 2 {
			wantType = "search_query"
		}
		if payload.Model != "google/gemini-embedding-2" || payload.Dimensions != 768 || payload.InputType != wantType {
			t.Fatalf("unexpected openrouter payload: %#v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2],"index":0}]}`))
	}))
	defer server.Close()

	embedder, err := NewOpenRouterEmbedder("test-openrouter-key", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	baseTransport := server.Client().Transport
	embedder.client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		clone := req.Clone(req.Context())
		clone.URL.Scheme = "http"
		clone.URL.Host = strings.TrimPrefix(server.URL, "http://")
		return baseTransport.RoundTrip(clone)
	})
	vectors, err := embedder.Embed(context.Background(), []string{"memory document"})
	if err != nil || len(vectors) != 1 || vectors[0][1] != 0.2 {
		t.Fatalf("unexpected document vectors=%#v err=%v", vectors, err)
	}
	vector, err := embedder.EmbedQuery(context.Background(), "memory query")
	if err != nil || len(vector) != 2 || vector[0] != 0.1 {
		t.Fatalf("unexpected query vector=%#v err=%v", vector, err)
	}
}

func TestEmbedderFromEnvAutoDetectsOpenRouterBeforeGemini(t *testing.T) {
	t.Setenv("CODELOCAL_EMBEDDING_PROVIDER", "")
	t.Setenv("OPENROUTER_API_KEY", "router-key")
	t.Setenv("GEMINI_API_KEY", "gemini-key")
	t.Setenv("CODELOCAL_EMBEDDING_MODEL", "")
	embedder := EmbedderFromEnv()
	if embedder == nil || embedder.Name() != "openrouter" || embedder.Model() != "google/gemini-embedding-2" {
		t.Fatalf("unexpected auto-detected embedder: %#v", embedder)
	}
}

func TestGeminiEmbedderErrorDoesNotExposeAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	const secret = "super-secret-key"
	embedder, err := NewGeminiEmbedder(secret, "gemini-embedding-001", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	baseTransport := server.Client().Transport
	embedder.client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		clone := req.Clone(req.Context())
		clone.URL.Scheme = "http"
		clone.URL.Host = strings.TrimPrefix(server.URL, "http://")
		return baseTransport.RoundTrip(clone)
	})
	_, err = embedder.Embed(context.Background(), []string{"hello"})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked API key: %v", err)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestEmbedderFromEnvPrefersLocalBaseURL(t *testing.T) {
	t.Setenv("CODELOCAL_EMBEDDING_PROVIDER", "")
	t.Setenv("CODELOCAL_EMBEDDING_BASE_URL", "http://127.0.0.1:20129")
	t.Setenv("OPENROUTER_API_KEY", "router-key")
	t.Setenv("GEMINI_API_KEY", "gemini-key")
	t.Setenv("CODELOCAL_EMBEDDING_MODEL", "")
	embedder := EmbedderFromEnv()
	if embedder == nil || embedder.Name() != "local" {
		t.Fatalf("local base url should win over hosted providers: %#v", embedder)
	}
	if embedder.Model() != "multilingual-e5-small" {
		t.Fatalf("model=%q want multilingual-e5-small", embedder.Model())
	}
}

func TestLocalEmbedderSendsTaskPrefixesAndParsesOutOfOrderData(t *testing.T) {
	var gotDocumentType, gotQueryType string
	var gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("path=%q want /v1/embeddings", r.URL.Path)
		}
		var body struct {
			Model     string   `json:"model"`
			Input     []string `json:"input"`
			InputType string   `json:"input_type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		gotModel = body.Model
		if len(body.Input) == 2 {
			gotDocumentType = body.InputType
			// Deliberately return the higher index first: the embedder must
			// reorder by `index` instead of trusting response order.
			_, _ = w.Write([]byte(`{"data":[{"index":1,"embedding":[0,0.2]},{"index":0,"embedding":[0.1,0]}]}`))
			return
		}
		gotQueryType = body.InputType
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[0.3,0.4]}]}`))
	}))
	defer server.Close()

	embedder, err := NewLocalEmbedder(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	vectors, err := embedder.Embed(context.Background(), []string{"a", "b"})
	if err != nil || len(vectors) != 2 {
		t.Fatalf("vectors=%#v err=%v", vectors, err)
	}
	if vectors[0][0] != 0.1 || vectors[1][1] != 0.2 {
		t.Fatalf("vector index reordering failed: %#v", vectors)
	}
	query, err := embedder.EmbedQuery(context.Background(), "q")
	if err != nil || len(query) != 2 || query[0] != 0.3 {
		t.Fatalf("query=%#v err=%v", query, err)
	}
	if gotDocumentType != "search_document" || gotQueryType != "search_query" {
		t.Fatalf("input types document=%q query=%q", gotDocumentType, gotQueryType)
	}
	if gotModel != "multilingual-e5-small" {
		t.Fatalf("model=%q want default multilingual-e5-small", gotModel)
	}
}

func TestLocalEmbedderRejectsBadBaseURL(t *testing.T) {
	if _, err := NewLocalEmbedder("ftp://example.com", "", time.Second); err == nil {
		t.Fatal("expected non-http scheme to be rejected")
	}
	if _, err := NewLocalEmbedder("", "", time.Second); err == nil {
		t.Fatal("expected empty base url to be rejected")
	}
}
