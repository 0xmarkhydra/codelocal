package cloudserver

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDashboardProtocolForZenModels(t *testing.T) {
	t.Setenv("CODELOCAL_LLM_PROVIDER", "zen")
	baseURL := "https://opencode.ai/zen/v1"
	tests := []struct {
		model string
		want  dashboardLLMProtocol
	}{
		{"muse-spark-1.2-contributor-free", dashboardProtocolResponses},
		{"gpt-5.6-sol", dashboardProtocolResponses},
		{"grok-code", dashboardProtocolResponses},
		{"deepseek-v3.2", dashboardProtocolChatCompletions},
		{"kimi-k2.5", dashboardProtocolChatCompletions},
		{"glm-5", dashboardProtocolChatCompletions},
		{"big-pickle", dashboardProtocolChatCompletions},
		{"claude-sonnet-5", dashboardProtocolUnsupported},
		{"qwen3.5-plus", dashboardProtocolUnsupported},
		{"gemini-3.1-pro", dashboardProtocolUnsupported},
	}
	for _, test := range tests {
		t.Run(test.model, func(t *testing.T) {
			if got := dashboardProtocolForModel(baseURL, test.model); got != test.want {
				t.Fatalf("dashboardProtocolForModel(%q) = %v, want %v", test.model, got, test.want)
			}
		})
	}
}

func TestDashboardProtocolForNonZenProviderUsesChatCompletions(t *testing.T) {
	t.Setenv("CODELOCAL_LLM_PROVIDER", "")
	if got := dashboardProtocolForModel("https://api.openai.com/v1", "custom-model"); got != dashboardProtocolChatCompletions {
		t.Fatalf("got %v, want chat completions", got)
	}
}

func TestWriteDashboardSSEFramesNamedEvent(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeDashboardSSE(recorder, nil, "delta", map[string]any{"delta": "xin chào"})
	body := recorder.Body.String()
	if !strings.HasPrefix(body, "event: delta\ndata: ") {
		t.Fatalf("unexpected SSE frame: %q", body)
	}
	if !strings.HasSuffix(body, "\n\n") {
		t.Fatalf("SSE frame must end with a blank line: %q", body)
	}
}

func TestResponsesInputTranslatesFunctionCallAndOutput(t *testing.T) {
	messages := []map[string]any{
		{"role": "assistant", "content": "", "tool_calls": []map[string]any{{"id": "call_1", "type": "function", "function": map[string]any{"name": "list_devices", "arguments": `{}`}}}},
		{"role": "tool", "content": `{"online":1}`, "tool_call_id": "call_1", "name": "list_devices"},
	}
	input := responsesInput(messages)
	if len(input) != 2 {
		t.Fatalf("len(input) = %d, want 2: %#v", len(input), input)
	}
	if input[0]["type"] != "function_call" || input[0]["call_id"] != "call_1" || input[0]["name"] != "list_devices" {
		t.Fatalf("unexpected function call input: %#v", input[0])
	}
	if input[1]["type"] != "function_call_output" || input[1]["call_id"] != "call_1" {
		t.Fatalf("unexpected function call output: %#v", input[1])
	}
}
