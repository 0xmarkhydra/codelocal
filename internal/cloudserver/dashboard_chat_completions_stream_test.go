package cloudserver

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyChatCompletionsStreamKeepsToolsAcrossRounds(t *testing.T) {
	requests := make([]string, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		requests = append(requests, string(raw))
		w.Header().Set("Content-Type", "text/event-stream")
		switch len(requests) {
		case 1:
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"search_project_brain\",\"arguments\":\"{\\\"query\\\":\\\"first\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
		case 2:
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_2\",\"function\":{\"name\":\"search_project_brain\",\"arguments\":\"{\\\"query\\\":\\\"second\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
		default:
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"xong\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		}
	}))
	defer server.Close()

	r := dashboardWithChatMode(httptest.NewRequest(http.MethodPost, "/api/v1/dashboard/chat?stream=1", nil), "agent")
	out := httptest.NewRecorder()
	if err := proxyLLMStream(out, nil, server.URL, "test-key", "custom-model", []map[string]any{{"role": "user", "content": "fix it"}}, dashboardChatTools, r, &Server{}, "user-1"); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 3 {
		t.Fatalf("requests=%d want 3; body=%s", len(requests), out.Body.String())
	}
	for index, body := range requests {
		if !strings.Contains(body, `"tools"`) || !strings.Contains(body, `"search_project_brain"`) {
			t.Fatalf("round %d lost dashboard tools: %s", index+1, body)
		}
	}
	if !strings.Contains(requests[1], `"tool_call_id":"call_1"`) {
		t.Fatalf("second round lost first tool result context: %s", requests[1])
	}
	if !strings.Contains(requests[2], `"tool_call_id":"call_2"`) {
		t.Fatalf("third round lost second tool result context: %s", requests[2])
	}
	if !strings.Contains(out.Body.String(), "xong") {
		t.Fatalf("final streamed answer missing: %s", out.Body.String())
	}
}

func TestCallChatCompletionsWithToolsAcceptsUnexpectedSSE(t *testing.T) {
	requestUsedStream := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		requestUsedStream = strings.Contains(string(raw), `"stream":true`)
		// Some OpenAI-compatible gateways return SSE even when the request did not
		// opt into streaming, and may omit the matching Content-Type header.
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"đã \"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"xong\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	calls, content, err := callChatCompletionsWithTools(server.URL, "test-key", "gemini-3.7-flash-high", []map[string]any{{"role": "user", "content": "continue"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if requestUsedStream {
		t.Fatal("non-stream request unexpectedly enabled streaming")
	}
	if len(calls) != 0 {
		t.Fatalf("calls=%#v want none", calls)
	}
	if content != "đã xong" {
		t.Fatalf("content=%q want %q", content, "đã xong")
	}
}
