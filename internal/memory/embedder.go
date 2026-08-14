package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Embedder interface {
	Name() string
	Model() string
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type GeminiEmbedder struct {
	apiKey string
	model  string
	client *http.Client
}

func NewGeminiEmbedder(apiKey, model string, timeout time.Duration) (*GeminiEmbedder, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, errors.New("gemini api key is required")
	}
	if strings.TrimSpace(model) == "" {
		model = "gemini-embedding-001"
	}
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	return &GeminiEmbedder{apiKey: apiKey, model: model, client: &http.Client{Timeout: timeout}}, nil
}

func (e *GeminiEmbedder) Name() string  { return "gemini" }
func (e *GeminiEmbedder) Model() string { return e.model }

func (e *GeminiEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	cleaned := make([]string, 0, len(texts))
	for _, text := range texts {
		if value := SanitizeText(text, 8000); value != "" {
			cleaned = append(cleaned, value)
		}
	}
	if len(cleaned) == 0 {
		return nil, nil
	}
	type part struct {
		Text string `json:"text"`
	}
	type content struct {
		Parts []part `json:"parts"`
	}
	type requestItem struct {
		Model    string  `json:"model"`
		Content  content `json:"content"`
		TaskType string  `json:"taskType"`
	}
	body := struct {
		Requests []requestItem `json:"requests"`
	}{Requests: make([]requestItem, 0, len(cleaned))}
	for _, text := range cleaned {
		body.Requests = append(body.Requests, requestItem{Model: "models/" + e.model, Content: content{Parts: []part{{Text: text}}}, TaskType: "RETRIEVAL_DOCUMENT"})
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:batchEmbedContents?key=%s", url.PathEscape(e.model), url.QueryEscape(e.apiKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gemini embedding request failed with http %d", resp.StatusCode)
	}
	var payload struct {
		Embeddings []struct {
			Values []float32 `json:"values"`
		} `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if len(payload.Embeddings) != len(cleaned) {
		return nil, errors.New("gemini embedding response was incomplete")
	}
	vectors := make([][]float32, 0, len(payload.Embeddings))
	for _, item := range payload.Embeddings {
		if len(item.Values) == 0 {
			return nil, errors.New("gemini embedding response contained an empty vector")
		}
		vectors = append(vectors, item.Values)
	}
	return vectors, nil
}

func EmbedderFromEnv() Embedder {
	if strings.TrimSpace(os.Getenv("CODELOCAL_MEMORY_ENABLED")) != "1" {
		return nil
	}
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("CODELOCAL_EMBEDDING_PROVIDER")))
	if provider == "" {
		provider = "gemini"
	}
	if provider != "gemini" {
		return nil
	}
	timeout := 12 * time.Second
	if value := strings.TrimSpace(os.Getenv("CODELOCAL_EMBEDDING_TIMEOUT")); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
			timeout = parsed
		}
	}
	embedder, err := NewGeminiEmbedder(os.Getenv("GEMINI_API_KEY"), os.Getenv("CODELOCAL_EMBEDDING_MODEL"), timeout)
	if err != nil {
		return nil
	}
	return embedder
}
