package cloudserver

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDashboardLLMRouteWithContextAddsActivePoolFallbackModels(t *testing.T) {
	pool := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"gpt-5.6-sol","owned_by":"pool"},{"id":"gpt-5.6-alt","owned_by":"pool"},{"id":"qwen3.8-flash","owned_by":"pool"}]}`)
	}))
	defer pool.Close()

	t.Setenv("CODELOCAL_AI_POOL_BASE_URL", pool.URL)
	t.Setenv("CODELOCAL_AI_POOL_API_KEY", "test-key")
	t.Setenv("CODELOCAL_AI_POOL_ENABLED", "1")
	t.Setenv("CODELOCAL_AI_POOL_MODEL", "gpt-5.6-sol")

	route := dashboardLLMRouteWithContext(context.Background(), "gpt-5.6-sol", false, true)
	if len(route) < 2 {
		t.Fatalf("route=%#v want selected model plus at least one fallback", route)
	}
	if route[0].Model != "gpt-5.6-sol" {
		t.Fatalf("first model=%q want selected model", route[0].Model)
	}
	foundSameFamilyFallback := false
	for _, target := range route[1:] {
		if target.Model == "gpt-5.6-alt" {
			foundSameFamilyFallback = true
			break
		}
	}
	if !foundSameFamilyFallback {
		t.Fatalf("route=%#v missing same-family pool fallback", route)
	}
}

func TestChatCompletionsStreamDetectsTruncatedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
	}))
	defer server.Close()

	round, err := callChatCompletionsStreamWithTools(context.Background(), server.URL, "test-key", "model", []map[string]any{{"role": "user", "content": "test"}}, nil, dashboardChatCompletionsStreamCallbacks{})
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err=%v want unexpected EOF", err)
	}
	if !round.Progressed || round.Content != "partial" {
		t.Fatalf("partial stream state lost: %#v", round)
	}
}

func TestResponsesStreamDetectsTruncatedToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"id\":\"item_1\",\"type\":\"function_call\",\"call_id\":\"call_1\",\"name\":\"read_file\"}}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.function_call_arguments.delta\",\"output_index\":0,\"item_id\":\"item_1\",\"delta\":\"{\\\"path\\\":\\\"README\"\"}\n\n")
	}))
	defer server.Close()

	round, err := callResponsesStreamWithTools(context.Background(), server.URL, "test-key", "model", []map[string]any{{"role": "user", "content": "test"}}, nil, dashboardResponsesStreamCallbacks{})
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err=%v want unexpected EOF", err)
	}
	if !round.Progressed {
		t.Fatalf("truncated tool stream should retain progress: %#v", round)
	}
}
