package mcpgateway

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

const legacyToolCompatibilityMaxBody = 4 << 20

type legacyToolAlias struct {
	Tool   string
	Action string
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
// sessions while stale per-thread tool schemas continue to function.
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
		if rewritten, changed := rewriteLegacyToolCall(raw); changed {
			body = rewritten
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		r.Header.Del("Content-Length")
		next.ServeHTTP(w, r)
	})
}
