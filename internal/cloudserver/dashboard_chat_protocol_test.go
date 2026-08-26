package cloudserver

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/gateway"
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

func TestDashboardChatSystemPromptUsesThanhGiongAndAutoRouting(t *testing.T) {
	prompt := dashboardChatSystemPrompt(nil, false)
	for _, token := range []string{"Thánh Gióng", "public AI model of CodeLocal", "Your model name is always Thánh Gióng", "Never disclose", "Auto", "authorized workspace", "never ask the user whether to wake"} {
		if !strings.Contains(prompt, token) {
			t.Fatalf("auto prompt missing %q: %s", token, prompt)
		}
	}
	if strings.Contains(prompt, "unless the user explicitly asks") {
		t.Fatalf("prompt must not allow underlying model disclosure: %s", prompt)
	}
}

func TestDashboardPublicModelNameIsThanhGiong(t *testing.T) {
	if dashboardPublicModelName != "Thánh Gióng" {
		t.Fatalf("dashboard public model = %q, want Thánh Gióng", dashboardPublicModelName)
	}
}

func TestDashboardChatSystemPromptPinsSelectedWorkspace(t *testing.T) {
	prompt := dashboardChatSystemPrompt(&dashboardChatWorkspace{
		DeviceID:      "device-1",
		WorkspaceID:   "workspace-1",
		WorkspaceName: "MediaUpload",
	}, false)
	for _, token := range []string{"MediaUpload", "workspace-1", "device-1", "primary project context", "already activated"} {
		if !strings.Contains(prompt, token) {
			t.Fatalf("manual workspace prompt missing %q: %s", token, prompt)
		}
	}
}

func TestDashboardChatFindWorkspaceMatchesProjectName(t *testing.T) {
	catalog := []gateway.WorkspaceView{
		{WorkspaceID: "MediaUpload-1", WorkspaceName: "MediaUpload", ProjectName: "MediaUpload"},
		{WorkspaceID: "MMON-Trading-2", WorkspaceName: "MMON Trading", ProjectName: "MMON Trading"},
	}
	got := dashboardChatFindWorkspace(catalog, "vào MMON Trading tóm tắt dự án giúp tôi")
	if got == nil || got.WorkspaceID != "MMON-Trading-2" {
		t.Fatalf("got %#v, want MMON Trading", got)
	}
}

func TestDashboardChatCurrentWorkspaceUsesMostRecentActive(t *testing.T) {
	catalog := []gateway.WorkspaceView{
		{WorkspaceID: "old", WorkspaceName: "Old", Status: "active", LastSeenAt: 10},
		{WorkspaceID: "idle", WorkspaceName: "Idle", Status: "sleeping", LastSeenAt: 30},
		{WorkspaceID: "new", WorkspaceName: "New", Status: "active", LastSeenAt: 20},
	}
	got := dashboardChatCurrentWorkspace(catalog)
	if got == nil || got.WorkspaceID != "new" {
		t.Fatalf("got %#v, want newest active workspace", got)
	}
}

func TestDashboardWorkspaceToolViewHidesSleepingLifecycle(t *testing.T) {
	view := dashboardWorkspaceToolView(gateway.WorkspaceView{WorkspaceID: "workspace-1", WorkspaceName: "MediaUpload", Status: "sleeping", RuntimeOnline: true, Authorized: true})
	if view["status"] != "idle" {
		t.Fatalf("status = %#v, want idle", view["status"])
	}
	if view["runtimeOnline"] != true || view["authorized"] != true {
		t.Fatalf("unexpected runtime fields: %#v", view)
	}
}

func TestDashboardPromptRequiresRealRuntimeExecution(t *testing.T) {
	prompt := dashboardChatSystemPrompt(&dashboardChatWorkspace{WorkspaceID: "workspace-1", WorkspaceName: "BitArena"}, false)
	for _, token := range []string{"runtime execution tools", "Never claim that you read, edited, ran, tested, or verified"} {
		if !strings.Contains(prompt, token) {
			t.Fatalf("execution prompt missing %q: %s", token, prompt)
		}
	}
}

func TestDashboardChatExposesExecutionTools(t *testing.T) {
	wanted := map[string]bool{"read_project_file": false, "edit_project_file": false, "run_project_command": false, "verify_project_changes": false}
	for _, tool := range dashboardChatTools {
		fn, _ := tool["function"].(map[string]any)
		name, _ := fn["name"].(string)
		if _, ok := wanted[name]; ok {
			wanted[name] = true
		}
	}
	for name, found := range wanted {
		if !found {
			t.Fatalf("dashboard chat missing execution tool %q", name)
		}
	}
}

func TestDashboardRuntimeToolSpecMapsToNativeRuntime(t *testing.T) {
	tests := []struct {
		name       string
		args       map[string]any
		wantTool   string
		wantEffect bool
	}{
		{"read_project_file", map[string]any{"path": "README.md"}, "read_file", false},
		{"read_project_file", map[string]any{"path": "README.md", "startLine": 1, "endLine": 10}, "read_file_range", false},
		{"search_project_code", map[string]any{"query": "TODO"}, "search_code", false},
		{"edit_project_file", map[string]any{"path": "a.go", "oldText": "a", "newText": "b"}, "edit_file", true},
		{"write_project_file", map[string]any{"path": "a.go", "content": "x"}, "write_file", true},
		{"apply_project_patch", map[string]any{"patch": "diff --git"}, "apply_patch", true},
		{"run_project_command", map[string]any{"command": "go test ./..."}, "run_command", true},
		{"verify_project_changes", map[string]any{}, "verify_changes", false},
	}
	for _, test := range tests {
		t.Run(test.name+test.wantTool, func(t *testing.T) {
			tool, effect, _, ok := dashboardRuntimeToolSpec(test.name, test.args)
			if !ok || tool != test.wantTool || effect != test.wantEffect {
				t.Fatalf("mapping = (%q,%v,%v), want (%q,%v,true)", tool, effect, ok, test.wantTool, test.wantEffect)
			}
		})
	}
}

func TestDashboardTransientLLMErrorsAreRetryable(t *testing.T) {
	for _, status := range []int{408, 425, 429, 500, 502, 503, 504} {
		if !dashboardIsTransientLLMError(&httpError{Status: status}) {
			t.Fatalf("status %d should be retryable", status)
		}
	}
	for _, status := range []int{400, 401, 403, 404, 422} {
		if dashboardIsTransientLLMError(&httpError{Status: status}) {
			t.Fatalf("status %d should not be retryable", status)
		}
	}
}

func TestDashboardToolProgressFingerprintCanonicalizesArguments(t *testing.T) {
	a := dashboardToolProgressFingerprint(llmToolCall{Name: "read_project_file", Arguments: `{"path":"README.md","startLine":1}`}, `{"ok":true}`)
	b := dashboardToolProgressFingerprint(llmToolCall{Name: "read_project_file", Arguments: `{"startLine":1,"path":"README.md"}`}, `{"ok":true}`)
	if a != b {
		t.Fatalf("equivalent tool arguments produced different fingerprints: %q != %q", a, b)
	}
}

func TestDashboardFinalSynthesisDisablesMoreToolWork(t *testing.T) {
	messages := dashboardFinalSynthesisMessages([]map[string]any{{"role": "user", "content": "fix it"}}, "repeated tool calls")
	last := messages[len(messages)-1]
	content, _ := last["content"].(string)
	for _, token := range []string{"Do not call any more tools", "Never expose internal orchestration limits", "repeated tool calls"} {
		if !strings.Contains(content, token) {
			t.Fatalf("final synthesis prompt missing %q: %s", token, content)
		}
	}
}

func TestDashboardFallbackReplyNeverExposesHTTP508(t *testing.T) {
	reply := dashboardFallbackReply([]dashboardToolCall{{Name: "read_project_file", Status: "done"}})
	if strings.Contains(reply, "508") || strings.Contains(strings.ToLower(reply), "tool loop") {
		t.Fatalf("fallback leaked internal orchestration error: %s", reply)
	}
}

func TestResponsesNativeStreamEmitsTextDeltas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), `"stream":true`) {
			t.Errorf("responses request did not enable native streaming: %s", raw)
		}
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Errorf("Accept = %q, want text/event-stream", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"Xin\"}\n\n")
		_, _ = io.WriteString(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\" chào\"}\n\n")
		_, _ = io.WriteString(w, "event: response.output_text.done\ndata: {\"type\":\"response.output_text.done\",\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"text\":\"Xin chào\"}\n\n")
	}))
	defer server.Close()

	var deltas []string
	round, err := callResponsesStreamWithTools(context.Background(), server.URL, "test-key", "muse-spark-1.2-contributor-free", []map[string]any{{"role": "user", "content": "hi"}}, nil, dashboardResponsesStreamCallbacks{
		OnText: func(delta string) { deltas = append(deltas, delta) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(deltas, ""); got != "Xin chào" {
		t.Fatalf("streamed text = %q, want Xin chào", got)
	}
	if round.Content != "Xin chào" || !round.Progressed || len(round.ToolCalls) != 0 {
		t.Fatalf("unexpected stream round: %#v", round)
	}
}

func TestResponsesNativeStreamCollectsFunctionCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"id\":\"item_1\",\"type\":\"function_call\",\"call_id\":\"call_1\",\"name\":\"read_project_file\",\"arguments\":\"\"}}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"item_1\",\"output_index\":0,\"delta\":\"{\\\"path\\\":\"}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.function_call_arguments.done\",\"item_id\":\"item_1\",\"output_index\":0,\"name\":\"read_project_file\",\"arguments\":\"{\\\"path\\\":\\\"README.md\\\"}\"}\n\n")
	}))
	defer server.Close()

	var toolDeltas int
	round, err := callResponsesStreamWithTools(context.Background(), server.URL, "test-key", "muse-spark-1.2-contributor-free", []map[string]any{{"role": "user", "content": "read"}}, dashboardChatTools, dashboardResponsesStreamCallbacks{
		OnToolDelta: func(index int, id, name, arguments string) { toolDeltas++ },
	})
	if err != nil {
		t.Fatal(err)
	}
	if toolDeltas == 0 {
		t.Fatal("expected streamed tool deltas")
	}
	if len(round.ToolCalls) != 1 {
		t.Fatalf("tool calls = %#v, want one", round.ToolCalls)
	}
	call := round.ToolCalls[0]
	if call.ID != "call_1" || call.Name != "read_project_file" || call.Arguments != `{"path":"README.md"}` {
		t.Fatalf("unexpected function call: %#v", call)
	}
}
