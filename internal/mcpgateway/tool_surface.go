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

// PublicToolSurfaceVersion is the compatibility generation of the public MCP
// contract. Generation 3 deliberately adds the cloud-native Blog tool while
// preserving the complete generation-2 tool set byte-for-byte at its legacy
// surface/contract boundaries.
const (
	PublicToolSurfaceVersion = 3
	// Freeze the MCP-facing implementation identity at the value already
	// advertised by the production gateway. Application releases may advance
	// independently without invalidating an existing AI-client binding.
	PublicMCPImplementationVersion = "1.5.16"

	PinnedLegacyPublicToolSurfaceVersion = 2
	PinnedLegacyPublicToolSurfaceHash    = "780206fb4c6f4b53162bc3080060d1b14978900bf29bdbda50edfad366e0864c"
	PinnedLegacyPublicToolContractHash   = "2f236697108144b7bf9d2e5296c6e20fd021d4739f0bce298c663bd4cce49cca"

	// Backward source-compatibility aliases. Generation-3 tests intentionally
	// use the Legacy names so future readers do not mistake these for the hash
	// of the 21-tool generation.
	PinnedPublicToolSurfaceHash  = PinnedLegacyPublicToolSurfaceHash
	PinnedPublicToolContractHash = PinnedLegacyPublicToolContractHash
)

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

type toolContractFingerprint struct {
	Name            string          `json:"name"`
	Title           string          `json:"title"`
	Description     string          `json:"description"`
	Schema          json.RawMessage `json:"schema"`
	AnnotationTitle string          `json:"annotationTitle"`
	ReadOnly        bool            `json:"readOnly"`
	Destructive     bool            `json:"destructive"`
	OpenWorld       bool            `json:"openWorld"`
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

func toolSurfaceFingerprintForDefinitions(defs []compactToolDef) []toolSurfaceFingerprint {
	fingerprints := make([]toolSurfaceFingerprint, 0, len(defs))
	for _, def := range defs {
		fingerprint := toolSurfaceFingerprint{Name: def.Name, Schema: canonicalSchema(def.Schema)}
		if def.Annotations != nil {
			fingerprint.ReadOnly = def.Annotations.ReadOnlyHint
			fingerprint.Destructive = annotationFlag(def.Annotations.DestructiveHint)
			fingerprint.OpenWorld = annotationFlag(def.Annotations.OpenWorldHint)
		}
		fingerprints = append(fingerprints, fingerprint)
	}
	sort.Slice(fingerprints, func(i, j int) bool { return fingerprints[i].Name < fingerprints[j].Name })
	return fingerprints
}

func toolSurfaceHashForDefinitions(defs []compactToolDef) string {
	raw, _ := json.Marshal(toolSurfaceFingerprintForDefinitions(defs))
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func toolContractHashForDefinitions(defs []compactToolDef, instructions string) string {
	fingerprints := make([]toolContractFingerprint, 0, len(defs))
	for _, def := range defs {
		fingerprint := toolContractFingerprint{
			Name:        def.Name,
			Title:       def.Title,
			Description: def.Description,
			Schema:      canonicalSchema(def.Schema),
		}
		if def.Annotations != nil {
			fingerprint.AnnotationTitle = def.Annotations.Title
			fingerprint.ReadOnly = def.Annotations.ReadOnlyHint
			fingerprint.Destructive = annotationFlag(def.Annotations.DestructiveHint)
			fingerprint.OpenWorld = annotationFlag(def.Annotations.OpenWorldHint)
		}
		fingerprints = append(fingerprints, fingerprint)
	}
	sort.Slice(fingerprints, func(i, j int) bool { return fingerprints[i].Name < fingerprints[j].Name })
	payload := struct {
		ImplementationVersion string                    `json:"implementationVersion"`
		Instructions          string                    `json:"instructions"`
		Tools                 []toolContractFingerprint `json:"tools"`
	}{
		ImplementationVersion: PublicMCPImplementationVersion,
		Instructions:          instructions,
		Tools:                 fingerprints,
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func legacyPublicToolDefinitions() []compactToolDef {
	defs := compactToolDefinitions()
	legacy := make([]compactToolDef, 0, len(defs))
	for _, def := range defs {
		if def.Name != "blog" {
			legacy = append(legacy, def)
		}
	}
	return legacy
}

func legacyToolSurfaceSummary() string {
	return fmt.Sprintf("CodeLocal tool surface v%d (%d tools, sha256:%s)", PinnedLegacyPublicToolSurfaceVersion, 20, PinnedLegacyPublicToolSurfaceHash)
}

func legacyPublicMCPInstructions() string {
	return compactOrchestrationInstructions + "\n\nCompatibility: " + legacyToolSurfaceSummary() + ". Legacy tool calls that CodeLocal can translate remain supported without user action. Only CODELOCAL_TOOL_SCHEMA_MISMATCH means the client requested a contract CodeLocal cannot translate."
}

func legacyPublicToolSurfaceHash() string {
	return toolSurfaceHashForDefinitions(legacyPublicToolDefinitions())
}

func legacyPublicToolContractHash() string {
	return toolContractHashForDefinitions(legacyPublicToolDefinitions(), legacyPublicMCPInstructions())
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
		toolSurfaceCache = ToolSurfaceInfo{
			Version: PublicToolSurfaceVersion,
			Hash:    toolSurfaceHashForDefinitions(defs),
			Count:   len(defs),
		}
	})
	return toolSurfaceCache
}

func publicToolContractHash() string {
	return toolContractHashForDefinitions(compactToolDefinitions(), publicMCPInstructions())
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

func publicMCPInstructions() string {
	return compactOrchestrationInstructions + "\n\nCompatibility: " + toolSurfaceSummary() + ". Legacy tool calls that CodeLocal can translate remain supported without user action. Only CODELOCAL_TOOL_SCHEMA_MISMATCH means the client requested a contract CodeLocal cannot translate."
}

func staleToolSchemaNotice(originalTool string) string {
	surface := PublicToolSurface()
	tool := strings.TrimSpace(originalTool)
	if tool == "" {
		tool = "an older tool"
	}
	return strings.Join([]string{
		"[CODELOCAL_TOOL_SCHEMA_STALE]",
		fmt.Sprintf("The MCP client called legacy CodeLocal tool %q; CodeLocal translated it for compatibility.", tool),
		fmt.Sprintf("Current tool surface: v%d, %d tools, sha256:%s", surface.Version, surface.Count, surface.Hash),
		"Compatibility translation succeeded. Continue the workflow normally; no reconnect, refresh, new chat, or local client update is required for this request.",
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
		root["codeLocalCompatibility"] = map[string]any{"toolSurface": PublicToolSurface(), "translated": true, "reconnectRecommended": false}
		result.StructuredContent = root
	}
	return result
}
