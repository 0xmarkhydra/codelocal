package cloudserver

import (
	"net/http"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/ui"
	"github.com/0xmarkhydra/codelocal/internal/webauth"
)

func statValue(stats map[string]int, key string) int {
	if stats == nil {
		return 0
	}
	return stats[key]
}

func knowledgeStats(graph cloud.KnowledgeGraph) string {
	return `<div class="grid" style="margin-bottom:14px">` +
		ui.MetricCard("Projects", statValue(graph.Stats, "projects"), "Logical projects in Project Brain") +
		ui.MetricCard("Repositories", statValue(graph.Stats, "repositories"), "Repository identities linked to knowledge") +
		ui.MetricCard("Relationships", statValue(graph.Stats, "edges"), "Durable semantic connections") + `</div>`
}

func knowledgeNeuralGraph(graph cloud.KnowledgeGraph) string {
	return ui.NeuralGraph(ui.NeuralGraphOptions{
		Mode: "knowledge", Data: graph, SearchPlaceholder: "Search projects, repositories, memories and skills…",
		InspectorLabel: "Knowledge inspector", EmptyTitle: "No knowledge yet",
		EmptyCopy: "Use CodeLocal on a project and let verified project memory accumulate. Durable relationships will appear here.",
		Help:      "Living Project Brain · drag nodes · pan · zoom · select for evidence",
		Filters: []ui.NeuralGraphFilter{
			{Value: "all", Label: "All knowledge"}, {Value: "project", Label: "Projects"},
			{Value: "repository", Label: "Repositories"}, {Value: "workspace", Label: "Workspaces"},
			{Value: "memory", Label: "Memories"}, {Value: "skill", Label: "Learned skills"}, {Value: "device", Label: "Devices"},
		},
		Legend: []ui.NeuralGraphLegend{
			{Label: "Project", Color: "#9b7cff"}, {Label: "Repository", Color: "#58a6ff"},
			{Label: "Memory", Color: "#50d9a6"}, {Label: "Skill", Color: "#f4a45f"},
		},
	})
}

func (s *Server) knowledgeDashboard(w http.ResponseWriter, r *http.Request, identity *webauth.Identity) {
	graph, err := s.Store.KnowledgeGraph(r.Context(), identity.User.ID, 320)
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(ui.DashboardPage(ui.DashboardOptions{
			Title: "Knowledge Graph", Active: "knowledge", Email: identity.User.Email, CSRF: identity.CSRF,
			Subtitle: "A living map of durable project knowledge.",
			Body:     `<div class="alert">Knowledge Graph is temporarily unavailable. Project Brain data was not deleted; retry after the graph store is reachable.</div>`,
			IsAdmin:  cloud.IsAdminEmail(identity.User.Email),
		})))
		return
	}

	durableLearning, _ := s.Store.DurableOutboxHealth(r.Context(), identity.User.ID)
	_, graphFreshness, _ := s.Store.CanonicalGraphFreshness(r.Context(), identity.User.ID)
	_, embeddingFreshness, _ := s.Store.CanonicalEmbeddingFreshness(r.Context(), identity.User.ID)
	semanticCanary, _ := s.Store.CanonicalSemanticCanaryMetrics(r.Context(), identity.User.ID)
	preference, _ := s.Store.CollectivePreference(r.Context(), identity.User.ID)
	recommendations, _ := s.Store.CollectiveRecommendations(r.Context(), identity.User.ID, 6)

	body := knowledgeStats(graph) + knowledgeNeuralGraph(graph)
	body += `<div class="card" style="margin-top:14px"><div class="section-kicker">Brain health</div><div class="title">Diagnostics & rollout controls</div><div class="label">The neural view is presentation only. Durable Knowledge Graph truth remains provenance-aware and independent from the rebuildable local Code Graph.</div></div>`
	body += durableLearningHealthCard(durableLearning) + canonicalGraphFreshnessCard(graphFreshness) + canonicalEmbeddingFreshnessCard(embeddingFreshness) + canonicalSemanticCanaryCard(semanticCanary) + collectivePreferencesCard(identity, preference) + collectiveRecommendationsCard(preference, recommendations)

	writeHTML(w, ui.DashboardPage(ui.DashboardOptions{
		Title: "Knowledge Graph", Active: "knowledge", Email: identity.User.Email, CSRF: identity.CSRF,
		Subtitle: "A living neural map of projects, decisions, memories, skills and durable relationships CodeLocal has verified for your account.",
		Body:     body, IsAdmin: cloud.IsAdminEmail(identity.User.Email),
	}))
}
