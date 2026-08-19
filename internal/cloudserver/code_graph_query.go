package cloudserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/gateway"
	"github.com/0xmarkhydra/codelocal/internal/project"
)

const codeGraphMaxVisibleNodes = 180

func codeGraphDepth(r *http.Request) int {
	value, _ := strconv.Atoi(r.URL.Query().Get("depth"))
	if value < 1 {
		return 1
	}
	if value > 3 {
		return 3
	}
	return value
}

func boundedCodeGraphParam(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if maxRunes > 0 && len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return value
}

func codeGraphSymbol(r *http.Request) string {
	return boundedCodeGraphParam(r.URL.Query().Get("symbol"), 160)
}

func codeGraphDisplayView(r *http.Request) string {
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("view")), "files") {
		return "files"
	}
	return "architecture"
}

func chooseCodeGraphWorkspace(catalog []gateway.WorkspaceView, key string) *gateway.WorkspaceView {
	key = strings.TrimSpace(key)
	if key != "" {
		for i := range catalog {
			if catalog[i].Key == key {
				value := catalog[i]
				return &value
			}
		}
	}
	for _, wanted := range []string{"active", "sleeping", "device_offline"} {
		for i := range catalog {
			if catalog[i].Status == wanted {
				value := catalog[i]
				return &value
			}
		}
	}
	return nil
}

func decodeCodeGraph(value any) (project.CodeGraphView, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return project.CodeGraphView{}, err
	}
	var view project.CodeGraphView
	if err := json.Unmarshal(raw, &view); err != nil {
		return project.CodeGraphView{}, err
	}
	return view, nil
}

func (s *Server) loadCodeGraph(ctx context.Context, userID string, workspace gateway.WorkspaceView, repository, symbol, viewMode string, depth int) (project.CodeGraphView, string, error) {
	active, err := s.Workspaces.Activate(ctx, userID, workspace.Key)
	if err != nil {
		return project.CodeGraphView{}, "offline", err
	}
	result, err := s.Hub.Call(ctx, userID, active.Key, "dashboard-code-graph", "get_code_graph", map[string]any{
		"query": symbol, "repository": repository, "view": viewMode, "depth": depth, "maxNodes": codeGraphMaxVisibleNodes,
	}, false, "")
	if err != nil {
		return project.CodeGraphView{}, "unavailable", err
	}
	if !result.OK {
		status := "unavailable"
		lower := strings.ToLower(result.Error)
		if strings.Contains(lower, "unknown") || strings.Contains(lower, "unsupported") || strings.Contains(lower, "get_code_graph") {
			status = "unsupported"
		}
		return project.CodeGraphView{}, status, fmt.Errorf("%s", first(result.Error, result.ErrorCode, "code graph query failed"))
	}
	view, err := decodeCodeGraph(result.Result)
	return view, "current", err
}
