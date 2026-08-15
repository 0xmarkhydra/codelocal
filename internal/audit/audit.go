package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/security"
	"github.com/0xmarkhydra/codelocal/internal/state"
)

type Event struct {
	TS           string `json:"ts,omitempty"`
	Event        string `json:"event"`
	RequestID    string `json:"requestId,omitempty"`
	MCPSessionID string `json:"mcpSessionId,omitempty"`
	WorkspaceKey string `json:"workspaceKey,omitempty"`
	Tool         string `json:"tool,omitempty"`
	ProcessID    string `json:"processId,omitempty"`
	ApprovalID   string `json:"approvalId,omitempty"`
	Status       string `json:"status,omitempty"`
	RiskLevel    string `json:"riskLevel,omitempty"`
	Detail       any    `json:"detail,omitempty"`
}

var secretKeyRE = regexp.MustCompile(`(?i)(?:api[_-]?key|access[_-]?key|private[_-]?key|token|secret|password|passwd|authorization|cookie|credentialSecret)`)
var redactTextKeyRE = regexp.MustCompile(`(?i)command|detail|query`)

// Free-form input text can contain passwords, OTPs, messages, form values or
// other private content. Audit the presence/size of such payloads, never the
// payload itself.
var sizeOnlyKeyRE = regexp.MustCompile(`(?i)^(?:content|patch|input|text|oldText|newText)$`)

func Enabled() bool { return os.Getenv("CODELOCAL_AUDIT_FILE") != "0" }

func Path() string {
	if value := os.Getenv("CODELOCAL_AUDIT_PATH"); value != "" {
		return value
	}
	return filepath.Join(state.Dir(), "audit.jsonl")
}

func sanitize(value any, key string) any {
	if value == nil {
		return nil
	}
	if err, ok := value.(error); ok {
		return map[string]any{"name": reflect.TypeOf(err).String(), "message": sanitize(err.Error(), "detail")}
	}
	switch v := value.(type) {
	case string:
		if sizeOnlyKeyRE.MatchString(key) {
			return fmt.Sprintf("[%d bytes]", len([]byte(v)))
		}
		if redactTextKeyRE.MatchString(key) {
			v = security.RedactCommand(v)
			if len(v) > 2000 {
				v = v[:2000]
			}
			return v
		}
		if len(v) > 4000 {
			return v[:4000]
		}
		return v
	case []string:
		out := make([]any, len(v))
		for i := range v {
			out[i] = sanitize(v[i], key)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = sanitize(v[i], key)
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for childKey, child := range v {
			if secretKeyRE.MatchString(childKey) {
				out[childKey] = "[REDACTED]"
			} else {
				out[childKey] = sanitize(child, childKey)
			}
		}
		return out
	default:
		return value
	}
}

func Write(event Event) {
	if !Enabled() {
		return
	}
	if event.TS == "" {
		event.TS = time.Now().UTC().Format(time.RFC3339Nano)
	}
	record := map[string]any{
		"ts": event.TS, "event": event.Event, "requestId": event.RequestID, "mcpSessionId": event.MCPSessionID,
		"workspaceKey": event.WorkspaceKey, "tool": event.Tool, "processId": event.ProcessID, "approvalId": event.ApprovalID,
		"status": event.Status, "riskLevel": event.RiskLevel, "detail": sanitize(event.Detail, "detail"),
	}
	for key, value := range record {
		if value == "" || value == nil {
			delete(record, key)
		}
	}
	_ = state.AppendJSONL(Path(), record)
}
