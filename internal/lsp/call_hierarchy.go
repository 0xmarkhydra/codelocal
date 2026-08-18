package lsp

import (
	"context"
	"encoding/json"
	"path/filepath"
)

func hierarchyItems(value any) []map[string]any {
	out := []map[string]any{}
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if entry, ok := item.(map[string]any); ok {
				out = append(out, entry)
			}
		}
	case map[string]any:
		out = append(out, typed)
	}
	return out
}

func hierarchyPosition(entry map[string]any) (line, column int) {
	rangeValue := entry["selectionRange"]
	if rangeValue == nil {
		rangeValue = entry["range"]
	}
	rangeMap, _ := rangeValue.(map[string]any)
	start, _ := rangeMap["start"].(map[string]any)
	if value, ok := start["line"].(float64); ok {
		line = int(value) + 1
	}
	if value, ok := start["character"].(float64); ok {
		column = int(value) + 1
	}
	return line, column
}

func normalizeHierarchyItem(item map[string]any, provider string) map[string]any {
	out := map[string]any{}
	for key, value := range item {
		if key == "data" {
			continue
		}
		out[key] = value
	}
	uri, _ := item["uri"].(string)
	out["provider"] = provider
	out["resolutionMode"] = "lsp"
	out["confidence"] = 1.0
	out["uri"] = uri
	out["path"] = uriPath(uri)
	if line, column := hierarchyPosition(item); line > 0 {
		out["line"] = line
		if column > 0 {
			out["column"] = column
		}
	}
	return out
}

func (m *Manager) prepareCallHierarchy(ctx context.Context, path string, line, column int) (*Client, []map[string]any, error) {
	client, err := m.clientFor(ctx, path)
	if err != nil || client == nil {
		return client, nil, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, err
	}
	if err := client.ensureOpen(absolute); err != nil {
		return nil, nil, err
	}
	raw, err := client.request(ctx, "textDocument/prepareCallHierarchy", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(absolute)},
		"position":     position(line, column),
	})
	if err != nil {
		return client, nil, err
	}
	return client, hierarchyItems(decodeAny(raw)), nil
}

func (m *Manager) IncomingCalls(ctx context.Context, path string, line, column int) ([]map[string]any, error) {
	client, prepared, err := m.prepareCallHierarchy(ctx, path, line, column)
	if err != nil || client == nil || len(prepared) == 0 {
		return nil, err
	}
	out := []map[string]any{}
	for _, item := range prepared {
		raw, requestErr := client.request(ctx, "callHierarchy/incomingCalls", map[string]any{"item": item})
		if requestErr != nil {
			return nil, requestErr
		}
		values, _ := decodeAny(raw).([]any)
		for _, value := range values {
			relation, ok := value.(map[string]any)
			if !ok {
				continue
			}
			from, ok := relation["from"].(map[string]any)
			if !ok {
				continue
			}
			entry := normalizeHierarchyItem(from, client.spec.ID)
			entry["relation"] = "CALLS"
			entry["direction"] = "incoming"
			entry["fromRanges"] = relation["fromRanges"]
			out = append(out, entry)
		}
	}
	return out, nil
}

func (m *Manager) OutgoingCalls(ctx context.Context, path string, line, column int) ([]map[string]any, error) {
	client, prepared, err := m.prepareCallHierarchy(ctx, path, line, column)
	if err != nil || client == nil || len(prepared) == 0 {
		return nil, err
	}
	out := []map[string]any{}
	for _, item := range prepared {
		raw, requestErr := client.request(ctx, "callHierarchy/outgoingCalls", map[string]any{"item": item})
		if requestErr != nil {
			return nil, requestErr
		}
		values, _ := decodeAny(raw).([]any)
		for _, value := range values {
			relation, ok := value.(map[string]any)
			if !ok {
				continue
			}
			to, ok := relation["to"].(map[string]any)
			if !ok {
				continue
			}
			entry := normalizeHierarchyItem(to, client.spec.ID)
			entry["relation"] = "CALLS"
			entry["direction"] = "outgoing"
			entry["fromRanges"] = relation["fromRanges"]
			out = append(out, entry)
		}
	}
	return out, nil
}

func decodeHierarchyJSON(raw json.RawMessage) []map[string]any {
	return hierarchyItems(decodeAny(raw))
}
