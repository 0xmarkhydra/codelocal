package cloudserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/gateway"
	"github.com/0xmarkhydra/codelocal/internal/project"
	"github.com/0xmarkhydra/codelocal/internal/ui"
	"github.com/0xmarkhydra/codelocal/internal/webauth"
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

func codeGraphRepository(r *http.Request) string {
	return boundedCodeGraphParam(r.URL.Query().Get("repository"), 240)
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
				copy := catalog[i]
				return &copy
			}
		}
	}
	for _, wanted := range []string{"active", "sleeping", "device_offline"} {
		for i := range catalog {
			if catalog[i].Status == wanted {
				copy := catalog[i]
				return &copy
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

func codeGraphWorkspaceOptions(catalog []gateway.WorkspaceView, selected string) string {
	var out strings.Builder
	for _, workspace := range catalog {
		label := workspace.WorkspaceName + " · " + workspace.DeviceName + " · " + strings.ReplaceAll(workspace.Status, "_", " ")
		selectedAttr := ""
		if workspace.Key == selected {
			selectedAttr = " selected"
		}
		out.WriteString(`<option value="` + ui.Escape(workspace.Key) + `"` + selectedAttr + `>` + ui.Escape(label) + `</option>`)
	}
	return out.String()
}

func codeGraphRepositoryOptions(snapshots []project.CodeGraphSnapshot, selected string) string {
	var out strings.Builder
	for _, snapshot := range snapshots {
		value := snapshot.RepositoryID
		label := snapshot.RepositoryPath
		if label == "." {
			label = "Primary repository"
		}
		selectedAttr := ""
		if selected == value || selected == snapshot.RepositoryPath || (selected == "" && out.Len() == 0) {
			selectedAttr = " selected"
		}
		out.WriteString(`<option value="` + ui.Escape(value) + `"` + selectedAttr + `>` + ui.Escape(label) + `</option>`)
	}
	return out.String()
}

func codeGraphContextControls(catalog []gateway.WorkspaceView, workspace *gateway.WorkspaceView, graph project.CodeGraphView, repository, symbol, viewMode string, depth int) string {
	if workspace == nil {
		return ""
	}
	repositorySelect := ""
	if len(graph.Snapshots) > 0 {
		repositorySelect = `<label>Repository<select name="repository" onchange="this.form.submit()">` + codeGraphRepositoryOptions(graph.Snapshots, repository) + `</select></label>`
	}
	architectureSelected, filesSelected := "", ""
	if viewMode == "files" {
		filesSelected = " selected"
	} else {
		architectureSelected = " selected"
	}
	viewSelect := `<label>View<select name="view" onchange="this.form.submit()"><option value="architecture"` + architectureSelected + `>Architecture</option><option value="files"` + filesSelected + `>Files</option></select></label>`
	projectName := first(workspace.ProjectName, workspace.WorkspaceName)
	return `<form class="codegraph-context" method="get" action="/dashboard/code-graph"><label>Project<span>` + ui.Escape(projectName) + `</span></label><label>Checkout<select name="workspace" onchange="this.form.submit()">` + codeGraphWorkspaceOptions(catalog, workspace.Key) + `</select></label>` + repositorySelect + viewSelect + `<input type="hidden" name="symbol" value="` + ui.Escape(symbol) + `"><input type="hidden" name="depth" value="` + strconv.Itoa(depth) + `></form>`
}

func codeGraphStatus(view project.CodeGraphView, workspace *gateway.WorkspaceView, state string) string {
	if workspace == nil {
		return `<div class="codegraph-state offline"><strong>No authorized project yet</strong><span>Run <code>codelocal .</code> inside a project first. Code Graph never scans folders you have not explicitly authorized.</span></div>`
	}
	if state != "current" {
		title, copy := "Code Graph unavailable", "The local runtime could not provide this graph right now. Project Brain remains available."
		if state == "offline" {
			title, copy = "Local runtime offline", "Start CodeLocal on "+workspace.DeviceName+" to inspect the current source graph. No raw source graph is stored in Cloud as a fallback."
		} else if state == "unsupported" {
			title, copy = "Runtime update required", "This device is connected with an older runtime that does not expose bounded Code Graph queries. Update CodeLocal, then reopen this page."
		}
		return `<div class="codegraph-state ` + ui.Escape(state) + `"><strong>` + ui.Escape(title) + `</strong><span>` + ui.Escape(copy) + `</span></div>`
	}
	if view.Status == "ambiguous" {
		return `<div class="codegraph-state unavailable"><strong>Multiple symbols match this query</strong><span>CodeLocal will not guess which symbol you meant. Choose a more specific name such as <code>Store.Save</code>, or inspect one of the candidate nodes below.</span></div>`
	}
	snapshot := view.Snapshot
	if snapshot == nil && len(view.Snapshots) > 0 {
		snapshot = &view.Snapshots[0]
	}
	if snapshot == nil {
		return `<div class="codegraph-state"><strong>Runtime connected</strong><span>No repository snapshot is available for this checkout yet.</span></div>`
	}
	status := "CURRENT"
	if snapshot.Dirty {
		status = "CURRENT · WORKING TREE"
	}
	commit := snapshot.Commit
	if len(commit) > 9 {
		commit = commit[:9]
	}
	return `<div class="codegraph-state current"><span class="codegraph-status-dot"></span><div><strong>` + status + `</strong><span>` + ui.Escape(first(snapshot.Branch, "detached")) + ` · ` + ui.Escape(commit) + ` · ` + strconv.Itoa(snapshot.FileCount) + ` files · ` + strconv.Itoa(snapshot.SymbolCount) + ` symbols · bounded to ` + strconv.Itoa(view.MaxNodes) + ` visible nodes</span></div><code>` + ui.Escape(snapshot.Revision) + `</code></div>`
}

func codeGraphImpactCard(view project.CodeGraphView) string {
	impact := view.Impact
	if impact == nil {
		return ""
	}
	confidence := strconv.Itoa(int(impact.AverageConfidence*100 + .5))
	copy := "Bounded impact from the visible call neighborhood; it is not a claim that every repository path was analyzed."
	if impact.Truncated {
		copy = "The bounded slice hit a safety limit, so risk stays conservative. Expand depth carefully or inspect specific callers for more evidence."
	}
	return `<div class="codegraph-impact ` + ui.Escape(impact.Risk) + `"><div><span class="codegraph-impact-kicker">Impact Analysis · ` + ui.Escape(strings.ToUpper(impact.Risk)) + `</span><strong>` + strconv.Itoa(impact.DirectCallers) + ` direct callers · ` + strconv.Itoa(impact.PotentialCallers) + ` potential upstream callers · ` + strconv.Itoa(impact.AffectedFiles) + ` files</strong><small>` + ui.Escape(copy) + `</small></div><div class="codegraph-impact-evidence"><span>` + strconv.Itoa(impact.DirectCallees) + `<small>callees</small></span><span>` + strconv.Itoa(impact.SemanticEdges) + `/` + strconv.Itoa(impact.EvidenceEdges) + `<small>semantic edges</small></span><span>` + confidence + `%<small>avg confidence</small></span></div></div>`
}

func codeGraphStyles() string {
	return `<style>
.page-codegraph .main{max-width:none;padding-top:24px}.page-codegraph .main>.top{max-width:1600px}.page-codegraph .main>.codegraph-wrap{max-width:1600px;margin:auto}.codegraph-wrap{display:grid;gap:12px}.codegraph-context{display:flex;align-items:end;gap:10px;flex-wrap:wrap;padding:13px 14px;border:1px solid #182842;border-radius:15px;background:linear-gradient(180deg,#0a121e,#08101a);box-shadow:0 14px 36px rgba(0,0,0,.14)}.codegraph-context label{display:grid;gap:6px;min-width:180px;color:#62748d;font-size:8.5px;font-weight:780;text-transform:uppercase;letter-spacing:.1em}.codegraph-context label>span{height:39px;display:flex;align-items:center;padding:0 12px;border:1px solid #1c2c45;border-radius:10px;background:#08111d;color:#d4dfed;font-size:11px;font-weight:660;text-transform:none;letter-spacing:0}.codegraph-context select{height:39px;min-width:190px;border:1px solid #1d2d47;border-radius:10px;background:#08111d;color:#d4dfed;padding:0 11px;font-size:11px;outline:none}.codegraph-context select:focus{border-color:#4d7ff4;box-shadow:0 0 0 3px rgba(77,127,244,.12)}.codegraph-state{min-height:58px;display:flex;align-items:center;gap:11px;padding:11px 14px;border:1px solid #17263c;border-radius:14px;background:#08101a}.codegraph-state>div{min-width:0;display:grid;gap:3px}.codegraph-state strong{color:#dce7f5;font-size:10.5px;font-weight:760;letter-spacing:.025em}.codegraph-state span{color:#6f819a;font-size:9.5px;line-height:1.5}.codegraph-state>code{margin-left:auto;max-width:340px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;border-color:#1a2a42;background:#060d17;color:#8ca3c1}.codegraph-state.offline,.codegraph-state.unsupported,.codegraph-state.unavailable{display:grid;gap:5px;border-color:#3f3420;background:#151108}.codegraph-state.offline strong,.codegraph-state.unsupported strong,.codegraph-state.unavailable strong{color:#e4c98f}.codegraph-status-dot{width:7px;height:7px;flex:0 0 7px;border-radius:50%;background:#4bd6a0;box-shadow:0 0 14px rgba(75,214,160,.62)}.codegraph-impact{display:flex;align-items:center;justify-content:space-between;gap:18px;padding:14px 15px;border:1px solid #1b2d47;border-radius:15px;background:linear-gradient(120deg,#0a1320,#08101a)}.codegraph-impact>div:first-child{display:grid;gap:4px;min-width:0}.codegraph-impact-kicker{font-size:8.5px;font-weight:800;letter-spacing:.1em;text-transform:uppercase;color:#6f87aa}.codegraph-impact strong{font-size:11px;color:#d7e2ef}.codegraph-impact small{font-size:9px;line-height:1.55;color:#72839a}.codegraph-impact.high{border-color:#4a2630;background:linear-gradient(120deg,#151018,#100b11)}.codegraph-impact.high .codegraph-impact-kicker{color:#ff8492}.codegraph-impact.medium{border-color:#493a20;background:linear-gradient(120deg,#16120a,#100e09)}.codegraph-impact.medium .codegraph-impact-kicker{color:#efbc68}.codegraph-impact-evidence{display:flex;gap:8px;flex-wrap:wrap;justify-content:flex-end}.codegraph-impact-evidence>span{min-width:82px;padding:8px 10px;border:1px solid #1d2d46;border-radius:10px;background:#08111c;color:#c9d5e4;font-size:11px;font-weight:760;text-align:center}.codegraph-impact-evidence small{display:block;margin-top:2px;font-size:7.5px;font-weight:600;color:#687a92}
@media(max-width:760px){.page-codegraph .main{padding-top:18px}.codegraph-context{align-items:stretch}.codegraph-context label,.codegraph-context select{width:100%;min-width:0}.codegraph-state.current{align-items:flex-start;flex-wrap:wrap}.codegraph-state>code{width:100%;max-width:none;margin-left:17px}.codegraph-impact{align-items:flex-start;flex-direction:column}.codegraph-impact-evidence{justify-content:flex-start;width:100%}}
</style>`
}

func codeGraphNeural(view project.CodeGraphView, symbol string, depth int) string {
	filters := []ui.NeuralGraphFilter{{Value: "all", Label: "All nodes"}, {Value: "symbol", Label: "Symbols"}, {Value: "file", Label: "Files"}, {Value: "external", Label: "Dependencies"}}
	legend := []ui.NeuralGraphLegend{{Label: "Symbol", Color: "#6d9cff"}, {Label: "File", Color: "#50d9a6"}, {Label: "External", Color: "#a98bff"}}
	help := "Neural Code Graph · semantic edges glow strongest · weak fallbacks are dashed"
	if view.View == "architecture" && symbol == "" {
		filters = []ui.NeuralGraphFilter{{Value: "all", Label: "All regions"}, {Value: "module", Label: "Modules"}, {Value: "external", Label: "Dependencies"}}
		legend = []ui.NeuralGraphLegend{{Label: "Module", Color: "#59c9df"}, {Label: "External", Color: "#a98bff"}}
		help = "Architecture view · structural module regions · brighter pulses show aggregated import traffic"
	}
	return ui.NeuralGraph(ui.NeuralGraphOptions{
		Mode: "code", Data: view, Query: symbol, Depth: depth, RemoteSearch: true,
		SearchPlaceholder: "Find a function, method, file or symbol…", InspectorLabel: "Code inspector",
		EmptyTitle: "No relationships in this slice", EmptyCopy: "Search for a symbol or choose another repository. CodeLocal only renders bounded evidence returned by the active local runtime.",
		Help: help, Filters: filters, Legend: legend,
	})
}

func (s *Server) codeGraphDashboard(w http.ResponseWriter, r *http.Request, identity *webauth.Identity) {
	catalog, _ := s.Workspaces.Catalog(r.Context(), identity.User.ID)
	workspace := chooseCodeGraphWorkspace(catalog, r.URL.Query().Get("workspace"))
	repository := codeGraphRepository(r)
	symbol, depth, viewMode := codeGraphSymbol(r), codeGraphDepth(r), codeGraphDisplayView(r)
	view := project.CodeGraphView{Status: "unavailable", View: viewMode, Depth: depth, MaxNodes: codeGraphMaxVisibleNodes, Nodes: []project.CodeGraphNode{}, Edges: []project.CodeGraphEdge{}}
	state := "offline"
	if workspace != nil && workspace.RuntimeOnline {
		queryCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		loaded, loadedState, err := s.loadCodeGraph(queryCtx, identity.User.ID, *workspace, repository, symbol, viewMode, depth)
		cancel()
		state = loadedState
		if err == nil {
			view = loaded
		} else if state == "current" {
			state = "unavailable"
		}
	}
	if repository == "" && view.Snapshot != nil {
		repository = view.Snapshot.RepositoryID
	}
	body := codeGraphStyles() + `<div class="codegraph-wrap">` + codeGraphContextControls(catalog, workspace, view, repository, symbol, viewMode, depth) + codeGraphStatus(view, workspace, state)
	if state == "current" {
		body += codeGraphImpactCard(view) + codeGraphNeural(view, symbol, depth)
	}
	body += `</div>`
	writeHTML(w, ui.DashboardPage(ui.DashboardOptions{
		Title: "Code Graph", Active: "codegraph", Email: identity.User.Email, CSRF: identity.CSRF,
		Subtitle: "A live, revision-aware neural map derived from the selected local checkout. Raw source and the full graph stay on your machine.",
		Body:     body, IsAdmin: cloud.IsAdminEmail(identity.User.Email),
	}))
}

func codeGraphHref(workspace gateway.WorkspaceView) string {
	return "/dashboard/code-graph?workspace=" + url.QueryEscape(workspace.Key)
}
