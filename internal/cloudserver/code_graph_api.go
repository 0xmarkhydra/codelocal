package cloudserver

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/gateway"
	"github.com/0xmarkhydra/codelocal/internal/project"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

type codeGraphContextDTO struct {
	DeviceID      string `json:"deviceId"`
	DeviceName    string `json:"deviceName"`
	WorkspaceID   string `json:"workspaceId"`
	WorkspaceName string `json:"workspaceName"`
	Status        string `json:"status"`
	RuntimeOnline bool   `json:"runtimeOnline"`
}

type codeGraphRepositoryDTO struct {
	Path        string `json:"path"`
	Branch      string `json:"branch,omitempty"`
	Commit      string `json:"commit,omitempty"`
	Dirty       bool   `json:"dirty"`
	IndexedAt   int64  `json:"indexedAt"`
	FileCount   int    `json:"fileCount"`
	SymbolCount int    `json:"symbolCount"`
}

type codeGraphNodeDTO struct {
	ID             string  `json:"id"`
	Kind           string  `json:"kind"`
	Name           string  `json:"name"`
	Summary        string  `json:"summary,omitempty"`
	Path           string  `json:"path,omitempty"`
	Line           int     `json:"line,omitempty"`
	Column         int     `json:"column,omitempty"`
	RepositoryPath string  `json:"repositoryPath,omitempty"`
	QualifiedName  string  `json:"qualifiedName,omitempty"`
	Provider       string  `json:"provider,omitempty"`
	ResolutionMode string  `json:"resolutionMode,omitempty"`
	Confidence     float64 `json:"confidence"`
	Canonical      bool    `json:"canonical"`
	Selected       bool    `json:"selected,omitempty"`
}

type codeGraphEdgeDTO struct {
	ID             string  `json:"id"`
	From           string  `json:"from"`
	To             string  `json:"to"`
	Relation       string  `json:"relation"`
	Provider       string  `json:"provider,omitempty"`
	ResolutionMode string  `json:"resolutionMode,omitempty"`
	Confidence     float64 `json:"confidence"`
	Count          int     `json:"count,omitempty"`
}

type codeGraphImpactDTO struct {
	DirectCallers     int     `json:"directCallers"`
	DirectCallees     int     `json:"directCallees"`
	PotentialCallers  int     `json:"potentialCallers"`
	AffectedFiles     int     `json:"affectedFiles"`
	EvidenceEdges     int     `json:"evidenceEdges"`
	SemanticEdges     int     `json:"semanticEdges"`
	AverageConfidence float64 `json:"averageConfidence"`
	Risk              string  `json:"risk"`
	Truncated         bool    `json:"truncated"`
}

type codeGraphResourceDTO struct {
	State        string                   `json:"state"`
	Status       string                   `json:"status"`
	View         string                   `json:"view"`
	Query        string                   `json:"query"`
	Depth        int                      `json:"depth"`
	MaxNodes     int                      `json:"maxNodes"`
	Truncated    bool                     `json:"truncated"`
	Context      *codeGraphContextDTO     `json:"context,omitempty"`
	Repositories []codeGraphRepositoryDTO `json:"repositories"`
	SelectedID   string                   `json:"selectedId,omitempty"`
	Nodes        []codeGraphNodeDTO       `json:"nodes"`
	Edges        []codeGraphEdgeDTO       `json:"edges"`
	Impact       *codeGraphImpactDTO      `json:"impact,omitempty"`
}

func codeGraphWorkspaceByPublicID(catalog []gateway.WorkspaceView, deviceID, workspaceID string) *gateway.WorkspaceView {
	deviceID = strings.TrimSpace(deviceID)
	workspaceID = strings.TrimSpace(workspaceID)
	if deviceID != "" && workspaceID != "" {
		for i := range catalog {
			if catalog[i].DeviceID == deviceID && catalog[i].WorkspaceID == workspaceID {
				value := catalog[i]
				return &value
			}
		}
		return nil
	}
	return chooseCodeGraphWorkspace(catalog, "")
}

func codeGraphContext(workspace *gateway.WorkspaceView) *codeGraphContextDTO {
	if workspace == nil {
		return nil
	}
	return &codeGraphContextDTO{
		DeviceID: workspace.DeviceID, DeviceName: workspace.DeviceName,
		WorkspaceID: workspace.WorkspaceID, WorkspaceName: workspace.WorkspaceName,
		Status: dashboardWorkspaceStatus(workspace.Status), RuntimeOnline: workspace.RuntimeOnline,
	}
}

func shortCodeCommit(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 12 {
		return value[:12]
	}
	return value
}

func codeGraphRepositories(values []project.CodeGraphSnapshot) []codeGraphRepositoryDTO {
	out := make([]codeGraphRepositoryDTO, 0, len(values))
	for _, value := range values {
		out = append(out, codeGraphRepositoryDTO{
			Path: value.RepositoryPath, Branch: value.Branch, Commit: shortCodeCommit(value.Commit), Dirty: value.Dirty,
			IndexedAt: value.IndexedAt, FileCount: value.FileCount, SymbolCount: value.SymbolCount,
		})
	}
	return out
}

func codeGraphNodes(values []project.CodeGraphNode) ([]codeGraphNodeDTO, map[string]string) {
	out := make([]codeGraphNodeDTO, 0, len(values))
	ids := make(map[string]string, len(values))
	for index, value := range values {
		id := "n" + strconv.Itoa(index+1)
		ids[value.ID] = id
		out = append(out, codeGraphNodeDTO{
			ID: id, Kind: value.Kind, Name: value.Name, Summary: value.Summary, Path: value.Path, Line: value.Line, Column: value.Column,
			RepositoryPath: value.RepositoryPath, QualifiedName: value.QualifiedName, Provider: value.Provider,
			ResolutionMode: value.ResolutionMode, Confidence: graphUnit(value.Confidence), Canonical: value.Canonical, Selected: value.Selected,
		})
	}
	return out, ids
}

func codeGraphEdges(values []project.CodeGraphEdge, ids map[string]string) []codeGraphEdgeDTO {
	out := make([]codeGraphEdgeDTO, 0, len(values))
	for _, value := range values {
		from, fromOK := ids[value.From]
		to, toOK := ids[value.To]
		if !fromOK || !toOK || from == to {
			continue
		}
		out = append(out, codeGraphEdgeDTO{
			ID: "e" + strconv.Itoa(len(out)+1), From: from, To: to, Relation: value.Relation,
			Provider: value.Provider, ResolutionMode: value.ResolutionMode, Confidence: graphUnit(value.Confidence), Count: max(0, value.Count),
		})
	}
	return out
}

func codeGraphImpact(value *project.CodeGraphImpact) *codeGraphImpactDTO {
	if value == nil {
		return nil
	}
	return &codeGraphImpactDTO{
		DirectCallers: max(0, value.DirectCallers), DirectCallees: max(0, value.DirectCallees),
		PotentialCallers: max(0, value.PotentialCallers), AffectedFiles: max(0, value.AffectedFiles),
		EvidenceEdges: max(0, value.EvidenceEdges), SemanticEdges: max(0, value.SemanticEdges),
		AverageConfidence: graphUnit(value.AverageConfidence), Risk: value.Risk, Truncated: value.Truncated,
	}
}

func buildCodeGraphResource(view project.CodeGraphView, workspace *gateway.WorkspaceView, state string) codeGraphResourceDTO {
	nodes, ids := codeGraphNodes(view.Nodes)
	selectedID := ids[view.SelectedID]
	return codeGraphResourceDTO{
		State: state, Status: view.Status, View: view.View, Query: view.Query, Depth: view.Depth,
		MaxNodes: min(max(0, view.MaxNodes), codeGraphMaxVisibleNodes), Truncated: view.Truncated,
		Context: codeGraphContext(workspace), Repositories: codeGraphRepositories(view.Snapshots),
		SelectedID: selectedID, Nodes: nodes, Edges: codeGraphEdges(view.Edges, ids), Impact: codeGraphImpact(view.Impact),
	}
}

func (s *Server) codeGraphResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	catalog, err := s.Workspaces.Catalog(r.Context(), identity.User.ID)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "code_graph_workspaces_unavailable"})
		return
	}
	workspace := codeGraphWorkspaceByPublicID(catalog, boundedCodeGraphParam(r.URL.Query().Get("deviceId"), 160), boundedCodeGraphParam(r.URL.Query().Get("workspaceId"), 200))
	viewMode := codeGraphDisplayView(r)
	depth := codeGraphDepth(r)
	view := project.CodeGraphView{Status: "unavailable", View: viewMode, Depth: depth, MaxNodes: codeGraphMaxVisibleNodes, Nodes: []project.CodeGraphNode{}, Edges: []project.CodeGraphEdge{}, Snapshots: []project.CodeGraphSnapshot{}}
	state := "no_workspace"
	if workspace == nil {
		webutil.JSON(w, http.StatusOK, buildCodeGraphResource(view, nil, state))
		return
	}
	if !workspace.RuntimeOnline {
		webutil.JSON(w, http.StatusOK, buildCodeGraphResource(view, workspace, "offline"))
		return
	}

	queryCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	loaded, loadedState, loadErr := s.loadCodeGraph(
		queryCtx, identity.User.ID, *workspace,
		boundedCodeGraphParam(r.URL.Query().Get("repositoryPath"), 240), codeGraphSymbol(r), viewMode, depth,
	)
	if loadErr != nil {
		view.Status = loadedState
		webutil.JSON(w, http.StatusOK, buildCodeGraphResource(view, workspace, loadedState))
		return
	}
	webutil.JSON(w, http.StatusOK, buildCodeGraphResource(loaded, workspace, "current"))
}
