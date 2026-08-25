package cloudserver

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
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
	// 7. Rate limit 20 req/min per user to prevent LLM abuse
	if allowed, _, retry, _ := s.Store.RateLimit(r.Context(), "dashboard-chat", identity.User.ID, 20, 60); !allowed {
		w.Header().Set("Retry-After", fmt.Sprintf("%d", retry))
		webutil.JSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate_limited", "retry_after": retry})
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
	isStream := r.URL.Query().Get("stream") == "1" || strings.Contains(r.Header.Get("Accept"), "text/event-stream")
	if isStream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, _ := w.(http.Flusher)
		writeSSE := func(event string, data any) {
			b, _ := json.Marshal(data)
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(b))
			if flusher != nil {
				flusher.Flush()
			}
		}
		// Mock stream when no key — still shows func call streaming like ChatGPT
		if apiKey == "" {
			lower2 := strings.ToLower(msg)
			var tcs []dashboardToolCall
			var reply string
			if strings.Contains(lower2, "workspace") {
				tcs = []dashboardToolCall{{ID: "mock_1", Name: "list_workspaces", Arguments: `{"status":"all"}`, Result: `{"total":26,"sample":["codex-mcp","X.com","BIDDI"]}`, DurationMs: 42, Status: "done"}}
				reply = "Đây là workspaces của bạn (Go mock stream):"
			} else if strings.Contains(lower2, "device") || strings.Contains(lower2, "máy") {
				tcs = []dashboardToolCall{{ID: "mock_2", Name: "list_devices", Arguments: `{}`, Result: `{"paired":1,"online":1}`, DurationMs: 18, Status: "done"}}
				reply = "Thiết bị đã pair (Go mock stream):"
			} else {
				reply = "CodeLocal Go (mock stream - chưa gắn CODELOCAL_LLM_API_KEY): đã nhận \"" + msg + "\"."
			}
			if len(tcs) > 0 {
				writeSSE("tool_calls", map[string]any{"tool_calls": tcs})
				time.Sleep(120 * time.Millisecond)
			}
			// stream reply by words like opencode text-delta
			for _, wrd := range strings.Split(reply, " ") {
				writeSSE("delta", map[string]any{"delta": wrd + " "})
				time.Sleep(35 * time.Millisecond)
			}
			writeSSE("done", map[string]any{"reply": reply, "tool_calls": tcs, "mock": true, "model": model})
			now2 := time.Now().UnixMilli()
			tcsJSON2, _ := json.Marshal(tcs)
			if err := s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: cloud.RandomHex(16), UserID: identity.User.ID, Role: "user", Content: msg, ToolCalls: json.RawMessage(`[]`), CreatedAt: now2}); err != nil {
				slog.Warn("dashboard chat stream mock save user failed", "error", err)
			}
			if err := s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: cloud.RandomHex(16), UserID: identity.User.ID, Role: "assistant", Content: reply, ToolCalls: json.RawMessage(tcsJSON2), CreatedAt: now2 + 1}); err != nil {
				slog.Warn("dashboard chat stream mock save assistant failed", "error", err)
			}
			return
		}
		// Real LLM stream: proxy OpenAI SSE, handle tool_calls and second call if needed
		system2 := "You are CodeLocal assistant on codelocal.cloud/dashboard. Answer concisely in Vietnamese when user speaks Vietnamese. Use tools when user asks about workspaces/devices/Project Brain."
		msgs := []map[string]any{{"role": "system", "content": system2}}
		for _, h := range req.History {
			m := map[string]any{"role": h.Role, "content": h.Content}
			if h.ToolCallID != "" {
				m["tool_call_id"] = h.ToolCallID
			}
			if h.Name != "" {
				m["name"] = h.Name
			}
			msgs = append(msgs, m)
		}
		msgs = append(msgs, map[string]any{"role": "user", "content": msg})
		if err := s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: cloud.RandomHex(16), UserID: identity.User.ID, Role: "user", Content: msg, ToolCalls: json.RawMessage(`[]`), CreatedAt: time.Now().UnixMilli()}); err != nil {
			slog.Warn("dashboard chat stream save user failed", "error", err)
		}
		if err := proxyLLMStream(w, flusher, baseURL, apiKey, model, msgs, dashboardChatTools, r, s, identity.User.ID); err != nil {
			writeSSE("error", map[string]string{"error": err.Error()})
		}
		return
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
		// persist to backend (not FE localStorage)
		now := time.Now().UnixMilli()
		tcsJSON, _ := json.Marshal(tcs)
		if err := s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: cloud.RandomHex(16), UserID: identity.User.ID, Role: "user", Content: msg, ToolCalls: json.RawMessage(`[]`), CreatedAt: now}); err != nil {
			slog.Warn("dashboard chat save user failed", "error", err, "user", identity.User.ID)
		}
		if err := s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: cloud.RandomHex(16), UserID: identity.User.ID, Role: "assistant", Content: reply, ToolCalls: json.RawMessage(tcsJSON), CreatedAt: now + 1}); err != nil {
			slog.Warn("dashboard chat save assistant failed", "error", err)
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
		now3 := time.Now().UnixMilli()
		if err := s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: cloud.RandomHex(16), UserID: identity.User.ID, Role: "user", Content: msg, ToolCalls: json.RawMessage(`[]`), CreatedAt: now3}); err != nil {
			slog.Warn("dashboard chat save no-tool user failed", "error", err)
		}
		if err := s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: cloud.RandomHex(16), UserID: identity.User.ID, Role: "assistant", Content: content, ToolCalls: json.RawMessage(`[]`), CreatedAt: now3 + 1}); err != nil {
			slog.Warn("dashboard chat save no-tool assistant failed", "error", err)
		}
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
	now4 := time.Now().UnixMilli()
	tcsJSON4, _ := json.Marshal(results)
	// truncate tool result if too large for DB (avoid 1MB JSONB)
	if len(tcsJSON4) > 5000 {
		tcsJSON4 = tcsJSON4[:5000]
	}
	if err := s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: cloud.RandomHex(16), UserID: identity.User.ID, Role: "user", Content: msg, ToolCalls: json.RawMessage(`[]`), CreatedAt: now4}); err != nil {
		slog.Warn("dashboard chat save final user failed", "error", err)
	}
	if err := s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: cloud.RandomHex(16), UserID: identity.User.ID, Role: "assistant", Content: finalContent, ToolCalls: json.RawMessage(tcsJSON4), CreatedAt: now4 + 1}); err != nil {
		slog.Warn("dashboard chat save final assistant failed", "error", err)
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"reply": finalContent, "model": model, "tool_calls": results})
}

func (s *Server) dashboardChatHistoryAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodDelete {
		_ = s.Store.ClearDashboardChatHistory(r.Context(), identity.User.ID)
		webutil.JSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	msgs, err := s.Store.ListDashboardChatHistory(r.Context(), identity.User.ID, 50)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "history_unavailable"})
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"messages": msgs})
}

func execDashboardTool(r *http.Request, s *Server, userID, name string, args map[string]any) string {
	const maxToolResult = 5000
	trunc := func(b []byte) string {
		if len(b) > maxToolResult {
			return string(b[:maxToolResult]) + `,"truncated":true}`
		}
		return string(b)
	}
	switch name {
	case "list_workspaces":
		ws, err := s.Workspaces.Catalog(r.Context(), userID)
		if err != nil {
			return `{"error":"workspaces_unavailable"}`
		}
		b, _ := json.Marshal(map[string]any{"total": len(ws), "workspaces": ws})
		return trunc(b)
	case "list_devices":
		devs, err := s.Store.ListDevices(r.Context(), userID)
		if err != nil {
			return `{"error":"devices_unavailable"}`
		}
		for i := range devs {
			devs[i].SecretHash = ""
		}
		b, _ := json.Marshal(map[string]any{"devices": devs})
		return trunc(b)
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

func proxyLLMStream(w http.ResponseWriter, flusher http.Flusher, baseURL, apiKey, model string, messages []map[string]any, tools []map[string]any, r *http.Request, s *Server, userID string) error {
	body := map[string]any{"model": model, "messages": messages, "temperature": 0.7, "stream": true, "stream_options": map[string]any{"include_usage": true}}
	if len(tools) > 0 {
		body["tools"] = tools
		body["tool_choice"] = "auto"
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return &httpError{Status: resp.StatusCode, Body: string(raw)}
	}
	writeRaw := func(data string) {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		if flusher != nil {
			flusher.Flush()
		}
	}
	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)
	toolCallsByIndex := map[int]*llmToolCall{}
	var fullContent strings.Builder
	finishedWithToolCalls := false
	for scanner.Scan() {
		select {
		case <-r.Context().Done():
			return r.Context().Err()
		default:
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   *string `json:"content"`
					ToolCalls []struct {
						Index    int     `json:"index"`
						ID       *string `json:"id"`
						Function *struct {
							Name      *string `json:"name"`
							Arguments *string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		if delta.Content != nil && *delta.Content != "" {
			fullContent.WriteString(*delta.Content)
			b2, _ := json.Marshal(map[string]any{"delta": *delta.Content})
			writeRaw(string(b2))
		}
		for _, tc := range delta.ToolCalls {
			idx := tc.Index
			if _, ok := toolCallsByIndex[idx]; !ok {
				toolCallsByIndex[idx] = &llmToolCall{}
			}
			if tc.ID != nil {
				toolCallsByIndex[idx].ID = *tc.ID
			}
			if tc.Function != nil {
				if tc.Function.Name != nil {
					toolCallsByIndex[idx].Name = *tc.Function.Name
				}
				if tc.Function.Arguments != nil {
					toolCallsByIndex[idx].Arguments += *tc.Function.Arguments
				}
			}
			// stream tool call delta as event
			b2, _ := json.Marshal(map[string]any{"tool_calls": []map[string]any{{"index": idx, "id": toolCallsByIndex[idx].ID, "name": toolCallsByIndex[idx].Name, "arguments": toolCallsByIndex[idx].Arguments}}})
			_, _ = fmt.Fprintf(w, "event: tool_delta\ndata: %s\n\n", string(b2))
			if flusher != nil {
				flusher.Flush()
			}
		}
		if chunk.Choices[0].FinishReason != nil && *chunk.Choices[0].FinishReason == "tool_calls" {
			finishedWithToolCalls = true
		}
	}
	if len(toolCallsByIndex) > 0 && finishedWithToolCalls {
		var tcs []llmToolCall
		for i := 0; i < len(toolCallsByIndex); i++ {
			if tc, ok := toolCallsByIndex[i]; ok {
				tcs = append(tcs, *tc)
			}
		}
		// execute tools and stream final answer like opencode second call
		var results []dashboardToolCall
		for _, tc := range tcs {
			t0 := time.Now()
			argsMap := map[string]any{}
			_ = json.Unmarshal([]byte(tc.Arguments), &argsMap)
			resStr := execDashboardTool(r, s, userID, tc.Name, argsMap)
			results = append(results, dashboardToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments, Result: resStr, DurationMs: time.Since(t0).Milliseconds(), Status: "done"})
			b2, _ := json.Marshal(map[string]any{"tool_calls": results})
			_, _ = fmt.Fprintf(w, "event: tool_calls\ndata: %s\n\n", string(b2))
			if flusher != nil {
				flusher.Flush()
			}
		}
		// second LLM call streamed
		follow := append([]map[string]any{}, messages...)
		toolCallsAny := []map[string]any{}
		for _, tc := range tcs {
			toolCallsAny = append(toolCallsAny, map[string]any{"id": tc.ID, "type": "function", "function": map[string]any{"name": tc.Name, "arguments": tc.Arguments}})
		}
		follow = append(follow, map[string]any{"role": "assistant", "content": fullContent.String(), "tool_calls": toolCallsAny})
		for _, tr := range results {
			follow = append(follow, map[string]any{"role": "tool", "content": tr.Result, "tool_call_id": tr.ID, "name": tr.Name})
		}
		body2 := map[string]any{"model": model, "messages": follow, "temperature": 0.7, "stream": true, "stream_options": map[string]any{"include_usage": true}}
		b2, _ := json.Marshal(body2)
		req2, _ := http.NewRequest(http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(b2))
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("Authorization", "Bearer "+apiKey)
		resp2, err := client.Do(req2)
		if err != nil {
			return err
		}
		defer resp2.Body.Close()
		var secondContent strings.Builder
		scanner2 := bufio.NewScanner(resp2.Body)
		scanner2.Buffer(buf, 1024*1024)
		for scanner2.Scan() {
			select {
			case <-r.Context().Done():
				return r.Context().Err()
			default:
			}
			line := strings.TrimSpace(scanner2.Text())
			if line == "" || !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "[DONE]" {
				break
			}
			var ch struct {
				Choices []struct {
					Delta struct {
						Content *string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			_ = json.Unmarshal([]byte(payload), &ch)
			if len(ch.Choices) > 0 && ch.Choices[0].Delta.Content != nil {
				secondContent.WriteString(*ch.Choices[0].Delta.Content)
				b3, _ := json.Marshal(map[string]any{"delta": *ch.Choices[0].Delta.Content})
				writeRaw(string(b3))
			}
		}
		tcsJSON, _ := json.Marshal(results)
		_ = s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: cloud.RandomHex(12), UserID: userID, Role: "assistant", Content: secondContent.String(), ToolCalls: json.RawMessage(tcsJSON), CreatedAt: time.Now().UnixMilli()})
	}
	// persist assistant for non-tool stream
	if len(toolCallsByIndex) == 0 {
		_ = s.Store.SaveDashboardChatMessage(r.Context(), cloud.DashboardChatMessage{ID: cloud.RandomHex(12), UserID: userID, Role: "assistant", Content: fullContent.String(), ToolCalls: json.RawMessage(`[]`), CreatedAt: time.Now().UnixMilli()})
	}
	_, _ = fmt.Fprintf(w, "event: done\ndata: %s\n\n", `{"done":true}`)
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
