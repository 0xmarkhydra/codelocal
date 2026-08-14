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
