package mcpgateway

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// PublicToolSurfaceVersion is a coarse compatibility generation for the
// model-facing CodeLocal schema. The hash below changes automatically whenever
// a public tool name/schema/behavior hint changes; bump this version only when a
// release intentionally changes the compatibility generation.
const PublicToolSurfaceVersion = 1

type ToolSurfaceInfo struct {
	Version int    `json:"version"`
	Hash    string `json:"hash"`
	Count   int    `json:"count"`
}

type toolSurfaceFingerprint struct {
	Name        string          `json:"name"`
	Schema      json.RawMessage `json:"schema"`
	ReadOnly    bool            `json:"readOnly"`
	Destructive bool            `json:"destructive"`
	OpenWorld   bool            `json:"openWorld"`
}

func annotationFlag(value *bool) bool {
	return value != nil && *value
}

func canonicalSchema(raw json.RawMessage) json.RawMessage {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return raw
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return canonical
}

var (
	toolSurfaceOnce  sync.Once
	toolSurfaceCache ToolSurfaceInfo
	toolNamesOnce    sync.Once
	toolNamesCache   map[string]struct{}
)

func PublicToolSurface() ToolSurfaceInfo {
	toolSurfaceOnce.Do(func() {
		defs := compactToolDefinitions()
		fingerprints := make([]toolSurfaceFingerprint, 0, len(defs))
		for _, def := range defs {
			annotations := def.Annotations
			fingerprint := toolSurfaceFingerprint{Name: def.Name, Schema: canonicalSchema(def.Schema)}
			if annotations != nil {
				fingerprint.ReadOnly = annotations.ReadOnlyHint
				fingerprint.Destructive = annotationFlag(annotations.DestructiveHint)
				fingerprint.OpenWorld = annotationFlag(annotations.OpenWorldHint)
			}
			fingerprints = append(fingerprints, fingerprint)
		}
		sort.Slice(fingerprints, func(i, j int) bool { return fingerprints[i].Name < fingerprints[j].Name })
		raw, _ := json.Marshal(fingerprints)
		sum := sha256.Sum256(raw)
		toolSurfaceCache = ToolSurfaceInfo{Version: PublicToolSurfaceVersion, Hash: hex.EncodeToString(sum[:]), Count: len(fingerprints)}
	})
	return toolSurfaceCache
}

func currentPublicToolNames() map[string]struct{} {
	toolNamesOnce.Do(func() {
		toolNamesCache = make(map[string]struct{}, len(compactToolDefinitions()))
		for _, def := range compactToolDefinitions() {
			toolNamesCache[def.Name] = struct{}{}
		}
	})
	return toolNamesCache
}

func toolSurfaceSummary() string {
	surface := PublicToolSurface()
	return fmt.Sprintf("CodeLocal tool surface v%d (%d tools, sha256:%s)", surface.Version, surface.Count, surface.Hash)
}

func staleToolSchemaNotice(originalTool string) string {
	surface := PublicToolSurface()
	tool := strings.TrimSpace(originalTool)
	if tool == "" {
		tool = "an outdated tool"
	}
	return strings.Join([]string{
		"[CODELOCAL_TOOL_SCHEMA_STALE]",
		"IMPORTANT FOR MCP CLIENT: Tell the user this MCP session is using an older CodeLocal tool schema and should reconnect or refresh CodeLocal after the current request.",
		fmt.Sprintf("The MCP client called legacy CodeLocal tool %q; CodeLocal translated it for compatibility.", tool),
		fmt.Sprintf("Current tool surface: v%d, %d tools, sha256:%s", surface.Version, surface.Count, surface.Hash),
		"Reconnect or refresh CodeLocal in the current AI client to load the latest tools/actions. A normal reconnect does not remove local pairing or workspace grants.",
		"[/CODELOCAL_TOOL_SCHEMA_STALE]",
	}, "\n")
}

func unknownToolSchemaMessage(tool string) string {
	return fmt.Sprintf("Unknown CodeLocal MCP tool %q. %s. This usually means the current MCP session has a stale or mismatched schema. Reconnect or refresh CodeLocal in the current AI client; if the local runtime is also outdated, run `npm i -g codelocal@latest` and then `codelocal`.", strings.TrimSpace(tool), toolSurfaceSummary())
}

type toolSchemaMismatchError struct{ message string }

func (e *toolSchemaMismatchError) Error() string { return e.message }

func unsupportedActionSchemaError(action string) error {
	return &toolSchemaMismatchError{message: fmt.Sprintf("unsupported action %q. %s. If the MCP client selected this action from a cached schema, reconnect or refresh CodeLocal in the current AI client", strings.TrimSpace(action), toolSurfaceSummary())}
}

func appendCompatibilityNotice(result *mcp.CallToolResult, notice string) *mcp.CallToolResult {
	if result == nil || strings.TrimSpace(notice) == "" {
		return result
	}
	result.Content = append([]mcp.Content{&mcp.TextContent{Text: notice}}, result.Content...)
	if root, ok := result.StructuredContent.(map[string]any); ok && root != nil {
		root["codeLocalCompatibility"] = map[string]any{"toolSurface": PublicToolSurface(), "reconnectRecommended": true}
		result.StructuredContent = root
	}
	return result
}
