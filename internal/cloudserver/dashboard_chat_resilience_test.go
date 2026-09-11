package cloudserver

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
