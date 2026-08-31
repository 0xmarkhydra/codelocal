package mcpgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const legacyToolCompatibilityMaxBody = 4 << 20

type legacyToolAlias struct {
	Tool   string
	Action string
}

type staleToolSchemaContextKey struct{}

type legacyCompatibilityResponseWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newLegacyCompatibilityResponseWriter() *legacyCompatibilityResponseWriter {
	return &legacyCompatibilityResponseWriter{header: make(http.Header)}
}

func (w *legacyCompatibilityResponseWriter) Header() http.Header { return w.header }

func (w *legacyCompatibilityResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *legacyCompatibilityResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(p)
}

func addLegacyCompatibilityNotice(raw []byte, originalTool string) ([]byte, bool) {
	var envelope map[string]any
	if json.Unmarshal(raw, &envelope) != nil || envelope == nil {
		return raw, false
	}
	result, _ := envelope["result"].(map[string]any)
	if result == nil {
		return raw, false
	}
	notice := staleToolSchemaNotice(originalTool)
	content, _ := result["content"].([]any)
	for _, item := range content {
		entry, _ := item.(map[string]any)
		text, _ := entry["text"].(string)
		if strings.Contains(text, "CODELOCAL_TOOL_SCHEMA_STALE") {
			return raw, false
		}
	}
	result["content"] = append([]any{map[string]any{"type": "text", "text": notice}}, content...)
	structured, _ := result["structuredContent"].(map[string]any)
	if structured == nil {
		structured = map[string]any{}
	}
	structured["codeLocalCompatibility"] = map[string]any{"toolSurface": PublicToolSurface(), "translated": true, "reconnectRecommended": false}
	result["structuredContent"] = structured
	rewritten, err := json.Marshal(envelope)
	if err != nil {
		return raw, false
	}
	return rewritten, true
}

func writeBufferedCompatibilityResponse(w http.ResponseWriter, buffered *legacyCompatibilityResponseWriter, originalTool string) {
	for key, values := range buffered.header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.Header().Del("Content-Length")
	body := buffered.body.Bytes()
	if rewritten, changed := addLegacyCompatibilityNotice(body, originalTool); changed {
		body = rewritten
	}
	status := buffered.status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func staleToolSchemaFromContext(ctx context.Context) string {
	value, _ := ctx.Value(staleToolSchemaContextKey{}).(string)
	return strings.TrimSpace(value)
}

func singleToolCall(raw []byte) (name string, id any, ok bool) {
	var envelope map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&envelope) != nil || envelope == nil {
		return "", nil, false
	}
	method, _ := envelope["method"].(string)
	if method != "tools/call" {
		return "", nil, false
	}
	params, _ := envelope["params"].(map[string]any)
	if params == nil {
		return "", envelope["id"], false
	}
	name, _ = params["name"].(string)
	return strings.TrimSpace(name), envelope["id"], true
}

func writeUnknownToolCompatibilityError(w http.ResponseWriter, id any, tool string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    -32601,
			"message": unknownToolSchemaMessage(tool),
			"data": map[string]any{
				"code":        "CODELOCAL_TOOL_SCHEMA_MISMATCH",
				"tool":        tool,
				"toolSurface": PublicToolSurface(),
				"action":      "reconnect_codelocal_mcp",
			},
		},
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		_, _ = fmt.Fprint(w, `{"jsonrpc":"2.0","error":{"code":-32601,"message":"CodeLocal MCP tool schema mismatch"},"id":null}`)
	}
}

// legacyToolAliasFor maps the old granular MCP tool surface onto the compact
// public surface without registering the old names in tools/list. This keeps
// already-open ChatGPT threads working after a gateway deploy while new
// sessions only discover the compact tool set.
func publicAliasForOperationID(operationID string) (legacyToolAlias, bool) {
	parts := strings.SplitN(strings.TrimSpace(operationID), ".", 2)
	if len(parts) != 2 {
		return legacyToolAlias{}, false
	}
	domain, action := parts[0], parts[1]
	switch domain {
	case "device":
		mapped := map[string]string{"list_active": "devices", "list_paired": "paired_devices", "rename": "rename_device", "revoke": "revoke_device"}[action]
		return legacyToolAlias{Tool: "workspace", Action: mapped}, mapped != ""
	case "memory":
		mapped := map[string]string{"remember": "remember", "recall": "recall"}[action]
		return legacyToolAlias{Tool: "workspace", Action: mapped}, mapped != ""
	case "skills":
		if action == "list" {
			return legacyToolAlias{Tool: "workspace", Action: "skills"}, true
		}
	case "project":
		mapped := map[string]string{"info": "project_info", "map": "project_map", "instructions": "instructions"}[action]
		return legacyToolAlias{Tool: "context", Action: mapped}, mapped != ""
	case "context":
		if action == "task" {
			return legacyToolAlias{Tool: "context", Action: "task"}, true
		}
	case "dependency":
		mapped := map[string]string{"inspect": "dependency_inspect", "read": "dependency_read", "search": "dependency_search"}[action]
		return legacyToolAlias{Tool: "context", Action: mapped}, mapped != ""
	case "lsp":
		if action == "info" {
			action = "lsp_info"
		}
		return legacyToolAlias{Tool: "context", Action: action}, true
	case "process":
		if action == "list" {
			action = "process_list"
		}
		return legacyToolAlias{Tool: "terminal", Action: action}, true
	case "approvals":
		mapped := map[string]string{"list": "approvals", "revoke": "revoke_approval", "reset": "reset_approvals"}[action]
		return legacyToolAlias{Tool: "workspace", Action: mapped}, mapped != ""
	case "security":
		mapped := map[string]string{"info": "security", "smoke_test": "security_smoke_test"}[action]
		return legacyToolAlias{Tool: "workspace", Action: mapped}, mapped != ""
	case "workspace", "read", "search", "edit", "verify", "git", "terminal", "mcp", "browser", "computer":
		return legacyToolAlias{Tool: domain, Action: action}, true
	}
	return legacyToolAlias{}, false
}

func legacyToolAliasFor(name string) (legacyToolAlias, bool) {
	operationID, ok := runtimeOperationID(strings.TrimSpace(name))
	if !ok {
		return legacyToolAlias{}, false
	}
	return publicAliasForOperationID(operationID)
}

func legacyCompactToolAliasFor(name string, args map[string]any) (legacyToolAlias, bool) {
	action, _ := args["action"].(string)
	action = strings.TrimSpace(action)
	var operationID string
	switch strings.TrimSpace(name) {
	case "device":
		operationID = map[string]string{"active": "device.list_active", "paired": "device.list_paired", "rename": "device.rename", "revoke": "device.revoke"}[action]
	case "project":
		operationID = map[string]string{"info": "project.info", "map": "project.map", "instructions": "project.instructions"}[action]
	case "dependency":
		operationID = map[string]string{"inspect": "dependency.inspect", "read": "dependency.read", "search": "dependency.search"}[action]
	case "lsp":
		if action != "" {
			operationID = "lsp." + action
		}
	case "process":
		if action != "" {
			operationID = "process." + action
		}
	case "approvals":
		if action != "" {
			operationID = "approvals." + action
		}
	case "security":
		if action != "" {
			operationID = "security." + action
		}
	default:
		return legacyToolAlias{}, false
	}
	if operationID == "" {
		return legacyToolAlias{}, false
	}
	return publicAliasForOperationID(operationID)
}

func rewriteLegacyToolEnvelope(value any) bool {
	switch envelope := value.(type) {
	case []any:
		changed := false
		for _, item := range envelope {
			if rewriteLegacyToolEnvelope(item) {
				changed = true
			}
		}
		return changed
	case map[string]any:
		method, _ := envelope["method"].(string)
		if method != "tools/call" {
			return false
		}
		params, _ := envelope["params"].(map[string]any)
		if params == nil {
			return false
		}
		name, _ := params["name"].(string)
		arguments, _ := params["arguments"].(map[string]any)
		if arguments == nil {
			arguments = map[string]any{}
			params["arguments"] = arguments
		}
		// Generation four exposed context(taskHint=...) without an action field.
		// Generation five keeps the same tool name but makes every grouped tool
		// action-driven, so translate the cached v4 shape in place.
		if strings.TrimSpace(name) == "context" {
			if _, hasAction := arguments["action"]; !hasAction {
				if taskHint, _ := arguments["taskHint"].(string); strings.TrimSpace(taskHint) != "" {
					arguments["action"] = "task"
					return true
				}
			}
		}
		alias, ok := legacyCompactToolAliasFor(name, arguments)
		if !ok {
			alias, ok = legacyToolAliasFor(name)
		}
		if !ok {
			return false
		}
		params["name"] = alias.Tool
		if alias.Action != "" {
			// A legacy tool has one fixed semantic operation. Always overwrite an
			// accidental action field so the compatibility path cannot change it.
			arguments["action"] = alias.Action
		}
		return true
	default:
		return false
	}
}

func rewriteLegacyToolCall(raw []byte) ([]byte, bool) {
	var envelope any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&envelope); err != nil {
		return raw, false
	}
	if !rewriteLegacyToolEnvelope(envelope) {
		return raw, false
	}
	rewritten, err := json.Marshal(envelope)
	if err != nil {
		return raw, false
	}
	return rewritten, true
}

// LegacyToolCallCompatibility rewrites only old tools/call requests. It does
// not alter tools/list, so the compact MCP surface stays small for new ChatGPT
// sessions while stale per-thread tool schemas continue to function. Rewritten
// calls are marked in request context so the tool result can tell ChatGPT/user
// that reconnecting the MCP will refresh the cached schema.
func LegacyToolCallCompatibility(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Body == nil || r.ContentLength < 0 || r.ContentLength > legacyToolCompatibilityMaxBody {
			next.ServeHTTP(w, r)
			return
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		_ = r.Body.Close()
		body := raw
		originalTool, requestID, isToolCall := singleToolCall(raw)
		legacyTranslated := false
		if rewritten, changed := rewriteLegacyToolCall(raw); changed {
			body = rewritten
			legacyTranslated = true
			if translatedTool, _, ok := singleToolCall(body); ok && translatedTool != "" && r.Header.Get("Mcp-Name") != "" {
				r.Header.Set("Mcp-Name", translatedTool)
			}
			r = r.WithContext(context.WithValue(r.Context(), staleToolSchemaContextKey{}, originalTool))
		} else if isToolCall && originalTool != "" {
			if _, current := currentPublicToolNames()[originalTool]; !current {
				writeUnknownToolCompatibilityError(w, requestID, originalTool)
				return
			}
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		r.Header.Del("Content-Length")
		if !legacyTranslated {
			next.ServeHTTP(w, r)
			return
		}
		buffered := newLegacyCompatibilityResponseWriter()
		next.ServeHTTP(buffered, r)
		writeBufferedCompatibilityResponse(w, buffered, originalTool)
	})
}
