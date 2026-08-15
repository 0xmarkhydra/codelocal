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

type QueryEmbedder interface {
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
}

type GeminiEmbedder struct {
	apiKey string
	model  string
	client *http.Client
}

type OpenRouterEmbedder struct {
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
		model = "gemini-embedding-2"
	}
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	return &GeminiEmbedder{apiKey: apiKey, model: model, client: &http.Client{Timeout: timeout}}, nil
}

func (e *GeminiEmbedder) Name() string  { return "gemini" }
func (e *GeminiEmbedder) Model() string { return e.model }

func NewOpenRouterEmbedder(apiKey, model string, timeout time.Duration) (*OpenRouterEmbedder, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, errors.New("openrouter api key is required")
	}
	if strings.TrimSpace(model) == "" {
		model = "google/gemini-embedding-2"
	}
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	return &OpenRouterEmbedder{apiKey: apiKey, model: model, client: &http.Client{Timeout: timeout}}, nil
}

func (e *OpenRouterEmbedder) Name() string  { return "openrouter" }
func (e *OpenRouterEmbedder) Model() string { return e.model }

func (e *OpenRouterEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return e.embed(ctx, texts, "search_document")
}

func (e *OpenRouterEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	vectors, err := e.embed(ctx, []string{text}, "search_query")
	if err != nil || len(vectors) == 0 {
		return nil, err
	}
	return vectors[0], nil
}

func (e *OpenRouterEmbedder) embed(ctx context.Context, texts []string, inputType string) ([][]float32, error) {
	cleaned := make([]string, 0, len(texts))
	for _, text := range texts {
		if value := SanitizeText(text, 8000); value != "" {
			cleaned = append(cleaned, value)
		}
	}
	if len(cleaned) == 0 {
		return nil, nil
	}
	body := struct {
		Model      string   `json:"model"`
		Input      []string `json:"input"`
		Dimensions int      `json:"dimensions"`
		InputType  string   `json:"input_type"`
	}{Model: e.model, Input: cleaned, Dimensions: 768, InputType: inputType}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://openrouter.ai/api/v1/embeddings", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, errors.New("openrouter embedding request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openrouter embedding request failed with http %d", resp.StatusCode)
	}
	var payload struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if len(payload.Data) != len(cleaned) {
		return nil, errors.New("openrouter embedding response was incomplete")
	}
	vectors := make([][]float32, len(cleaned))
	for _, item := range payload.Data {
		if item.Index < 0 || item.Index >= len(vectors) || len(item.Embedding) == 0 {
			return nil, errors.New("openrouter embedding response contained an invalid vector")
		}
		vectors[item.Index] = item.Embedding
	}
	for _, vector := range vectors {
		if len(vector) == 0 {
			return nil, errors.New("openrouter embedding response was incomplete")
		}
	}
	return vectors, nil
}

func (e *GeminiEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return e.embedWithTaskType(ctx, texts, "RETRIEVAL_DOCUMENT")
}

func (e *GeminiEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	vectors, err := e.embedWithTaskType(ctx, []string{text}, "RETRIEVAL_QUERY")
	if err != nil || len(vectors) == 0 {
		return nil, err
	}
	return vectors[0], nil
}

func (e *GeminiEmbedder) embedWithTaskType(ctx context.Context, texts []string, taskType string) ([][]float32, error) {
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
		Model                string  `json:"model"`
		Content              content `json:"content"`
		TaskType             string  `json:"taskType"`
		OutputDimensionality int     `json:"outputDimensionality"`
	}
	body := struct {
		Requests []requestItem `json:"requests"`
	}{Requests: make([]requestItem, 0, len(cleaned))}
	for _, text := range cleaned {
		body.Requests = append(body.Requests, requestItem{
			Model:                "models/" + e.model,
			Content:              content{Parts: []part{{Text: text}}},
			TaskType:             taskType,
			OutputDimensionality: 768,
		})
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
		return nil, errors.New("gemini embedding request failed")
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
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("CODELOCAL_EMBEDDING_PROVIDER")))
	if provider == "" {
		switch {
		case strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")) != "":
			provider = "openrouter"
		case strings.TrimSpace(os.Getenv("GEMINI_API_KEY")) != "":
			provider = "gemini"
		default:
			return nil
		}
	}
	timeout := 12 * time.Second
	if value := strings.TrimSpace(os.Getenv("CODELOCAL_EMBEDDING_TIMEOUT")); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
			timeout = parsed
		}
	}
	var (
		embedder Embedder
		err      error
	)
	switch provider {
	case "openrouter":
		embedder, err = NewOpenRouterEmbedder(os.Getenv("OPENROUTER_API_KEY"), os.Getenv("CODELOCAL_EMBEDDING_MODEL"), timeout)
	case "gemini":
		embedder, err = NewGeminiEmbedder(os.Getenv("GEMINI_API_KEY"), os.Getenv("CODELOCAL_EMBEDDING_MODEL"), timeout)
	default:
		return nil
	}
	if err != nil {
		return nil
	}
	return embedder
}
