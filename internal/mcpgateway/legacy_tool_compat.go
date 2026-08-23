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
func legacyToolAliasFor(name string) (legacyToolAlias, bool) {
	operationID, ok := runtimeOperationID(strings.TrimSpace(name))
	if !ok {
		return legacyToolAlias{}, false
	}
	parts := strings.SplitN(operationID, ".", 2)
	if len(parts) != 2 {
		return legacyToolAlias{}, false
	}
	alias := legacyToolAlias{Tool: parts[0], Action: parts[1]}
	switch operationID {
	case "device.list_active":
		alias.Action = "active"
	case "device.list_paired":
		alias.Action = "paired"
	case "context.task":
		// context is the only compact tool that does not use an action field.
		alias.Action = ""
	}
	return alias, true
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
		alias, ok := legacyToolAliasFor(name)
		if !ok {
			return false
		}
		params["name"] = alias.Tool
		arguments, _ := params["arguments"].(map[string]any)
		if arguments == nil {
			arguments = map[string]any{}
			params["arguments"] = arguments
		}
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
