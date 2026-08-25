package cloudserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

type dashboardChatHistoryItem struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	Name       string `json:"name,omitempty"`
}

type dashboardChatRequest struct {
	Message string                     `json:"message"`
	History []dashboardChatHistoryItem `json:"history"`
}

type dashboardToolCall struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Arguments  string `json:"arguments"`
	Result     string `json:"result,omitempty"`
	DurationMs int64  `json:"durationMs,omitempty"`
	Status     string `json:"status"`
}

var dashboardChatTools = []map[string]any{
	{
		"type": "function",
		"function": map[string]any{
			"name":        "list_workspaces",
			"description": "List authorized CodeLocal workspaces",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status": map[string]any{"type": "string", "enum": []string{"all", "active", "offline"}},
				},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "list_devices",
			"description": "List paired devices and online status",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "search_project_brain",
			"description": "Search Project Brain memory/graph",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{"query": map[string]any{"type": "string"}},
				"required":   []string{"query"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "get_workspace_detail",
			"description": "Get detail of a workspace by name or id",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{"workspace": map[string]any{"type": "string"}},
				"required":   []string{"workspace"},
			},
		},
	},
}

func (s *Server) dashboardChatAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		webutil.JSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	var req dashboardChatRequest
	if err := webutil.DecodeJSON(r, 64<<10, &req); err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "missing_message"})
		return
	}
	if len(req.History) > 12 {
		req.History = req.History[len(req.History)-12:]
	}
	apiKey := strings.TrimSpace(os.Getenv("CODELOCAL_LLM_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	baseURL := strings.TrimSpace(os.Getenv("CODELOCAL_LLM_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	model := strings.TrimSpace(os.Getenv("CODELOCAL_LLM_MODEL"))
	if model == "" {
		model = "gpt-4o-mini"
	}
	lower := strings.ToLower(msg)
	if apiKey == "" {
		var tcs []dashboardToolCall
		var reply string
		if strings.Contains(lower, "workspace") {
			tcs = []dashboardToolCall{{ID: "mock_1", Name: "list_workspaces", Arguments: `{"status":"all"}`, Result: `{"total":26,"sample":["codex-mcp","X.com","BIDDI"]}`, DurationMs: 42, Status: "done"}}
			reply = "Đây là workspaces của bạn (Go mock tool call):"
		} else if strings.Contains(lower, "device") || strings.Contains(lower, "máy") {
			tcs = []dashboardToolCall{{ID: "mock_2", Name: "list_devices", Arguments: `{}`, Result: `{"paired":1,"online":1}`, DurationMs: 18, Status: "done"}}
			reply = "Thiết bị đã pair (Go mock):"
		} else if strings.Contains(lower, "brain") || strings.Contains(lower, "memory") {
			tcs = []dashboardToolCall{{ID: "mock_3", Name: "search_project_brain", Arguments: `{"query":` + jsonQuote(msg) + `}`, Result: `{"hits":3}`, DurationMs: 55, Status: "done"}}
			reply = "Kết quả Project Brain (Go mock):"
		} else {
			reply = "CodeLocal Go (mock - chưa gắn CODELOCAL_LLM_API_KEY): đã nhận \"" + msg + "\". Gắn key vào Go env (railway.json) với MODEL=" + model + " để dùng model free qua codelocal."
		}
		webutil.JSON(w, http.StatusOK, map[string]any{"reply": reply, "tool_calls": tcs, "mock": true, "model": model})
		return
	}
	system := "You are CodeLocal assistant on codelocal.cloud/dashboard. Answer concisely in Vietnamese when user speaks Vietnamese. Use tools when user asks about workspaces/devices/Project Brain. No need to go to ChatGPT."
	messages := []map[string]any{{"role": "system", "content": system}}
	for _, h := range req.History {
		m := map[string]any{"role": h.Role, "content": h.Content}
		if h.ToolCallID != "" {
			m["tool_call_id"] = h.ToolCallID
		}
		if h.Name != "" {
			m["name"] = h.Name
		}
		messages = append(messages, m)
	}
	messages = append(messages, map[string]any{"role": "user", "content": msg})
	toolCalls, content, err := callLLMWithTools(baseURL, apiKey, model, messages, dashboardChatTools)
	if err != nil {
		webutil.JSON(w, http.StatusBadGateway, map[string]string{"error": "upstream: " + err.Error()})
		return
	}
	if len(toolCalls) == 0 {
		webutil.JSON(w, http.StatusOK, map[string]any{"reply": content, "model": model, "tool_calls": []dashboardToolCall{}})
		return
	}
	var results []dashboardToolCall
	for _, tc := range toolCalls {
		t0 := time.Now()
		argsMap := map[string]any{}
		_ = json.Unmarshal([]byte(tc.Arguments), &argsMap)
		resStr := execDashboardTool(r, s, identity.User.ID, tc.Name, argsMap)
		results = append(results, dashboardToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments, Result: resStr, DurationMs: time.Since(t0).Milliseconds(), Status: "done"})
	}
	follow := append([]map[string]any{}, messages...)
	toolCallsAny := []map[string]any{}
	for _, tc := range toolCalls {
		toolCallsAny = append(toolCallsAny, map[string]any{"id": tc.ID, "type": "function", "function": map[string]any{"name": tc.Name, "arguments": tc.Arguments}})
	}
	follow = append(follow, map[string]any{"role": "assistant", "content": content, "tool_calls": toolCallsAny})
	for _, tr := range results {
		follow = append(follow, map[string]any{"role": "tool", "content": tr.Result, "tool_call_id": tr.ID, "name": tr.Name})
	}
	_, finalContent, err2 := callLLMWithTools(baseURL, apiKey, model, follow, nil)
	if err2 != nil {
		webutil.JSON(w, http.StatusBadGateway, map[string]any{"error": "upstream2: " + err2.Error(), "tool_calls": results})
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"reply": finalContent, "model": model, "tool_calls": results})
}

func execDashboardTool(r *http.Request, s *Server, userID, name string, args map[string]any) string {
	switch name {
	case "list_workspaces":
		ws, err := s.Workspaces.Catalog(r.Context(), userID)
		if err != nil {
			return `{"error":"workspaces_unavailable"}`
		}
		b, _ := json.Marshal(map[string]any{"total": len(ws), "workspaces": ws})
		return string(b)
	case "list_devices":
		devs, err := s.Store.ListDevices(r.Context(), userID)
		if err != nil {
			return `{"error":"devices_unavailable"}`
		}
		for i := range devs {
			devs[i].SecretHash = ""
		}
		b, _ := json.Marshal(map[string]any{"devices": devs})
		return string(b)
	case "search_project_brain":
		q, _ := args["query"].(string)
		return `{"query":` + jsonQuote(q) + `,"hits":[{"path":"web/src/app/dashboard","score":0.92}],"note":"Go mock - will query Project Brain index"}`
	case "get_workspace_detail":
		wk, _ := args["workspace"].(string)
		return `{"workspace":` + jsonQuote(wk) + `,"note":"Go mock - lookup via Workspaces.Catalog"}`
	default:
		return `{"error":"unknown tool ` + name + `"}`
	}
}

type llmToolCall struct {
	ID        string
	Name      string
	Arguments string
}

func callLLMWithTools(baseURL, apiKey, model string, messages []map[string]any, tools []map[string]any) ([]llmToolCall, string, error) {
	body := map[string]any{"model": model, "messages": messages, "temperature": 0.7}
	if len(tools) > 0 {
		body["tools"] = tools
		body["tool_choice"] = "auto"
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", &httpError{Status: resp.StatusCode, Body: string(raw)}
	}
	var data struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, "", err
	}
	if len(data.Choices) == 0 {
		return nil, "", &httpError{Status: 502, Body: "empty choices"}
	}
	m := data.Choices[0].Message
	var tcs []llmToolCall
	for _, tc := range m.ToolCalls {
		tcs = append(tcs, llmToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
	}
	return tcs, m.Content, nil
}

type httpError struct {
	Status int
	Body   string
}

func (e *httpError) Error() string { return fmt.Sprintf("http %d: %s", e.Status, e.Body) }

func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
