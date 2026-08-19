package cloudserver

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

const knowledgeGraphAPINodeLimit = 320

var knowledgeGraphStatKeys = []string{
	"projects", "repositories", "workspaces", "devices", "skills", "portableSkills",
	"corroboratedPortableSkills", "degradedPortableSkills", "canonicalGraphNodes", "canonicalGraphEdges",
	"knowledgeSources", "conflictedKnowledgeSources", "experiences", "nativeMemories", "memories", "nodes", "edges",
}

type knowledgeGraphMetaDTO struct {
	NodeLimit   int  `json:"nodeLimit"`
	AtNodeLimit bool `json:"atNodeLimit"`
}

type knowledgeGraphNodeDTO struct {
	ID         string  `json:"id"`
	Kind       string  `json:"kind"`
	Name       string  `json:"name"`
	Summary    string  `json:"summary,omitempty"`
	Scope      string  `json:"scope,omitempty"`
	Confidence float64 `json:"confidence"`
	Importance float64 `json:"importance"`
	LastSeenAt int64   `json:"lastSeenAt,omitempty"`
}

type knowledgeGraphEdgeDTO struct {
	ID         string  `json:"id"`
	From       string  `json:"from"`
	To         string  `json:"to"`
	Relation   string  `json:"relation"`
	Confidence float64 `json:"confidence"`
	Importance float64 `json:"importance"`
}

type knowledgeGraphResourceDTO struct {
	Meta  knowledgeGraphMetaDTO   `json:"meta"`
	Stats map[string]int          `json:"stats"`
	Nodes []knowledgeGraphNodeDTO `json:"nodes"`
	Edges []knowledgeGraphEdgeDTO `json:"edges"`
}

func compactGraphText(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if maxRunes <= 0 || value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) > maxRunes {
		value = string(runes[:maxRunes]) + "…"
	}
	return value
}

func graphUnit(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func knowledgeGraphNodeName(node cloud.KnowledgeNode, deviceOrdinal int) string {
	if node.Kind == "device" {
		if deviceOrdinal <= 0 {
			return "Device"
		}
		return "Device " + strconv.Itoa(deviceOrdinal)
	}
	return compactGraphText(node.Name, 120)
}

func buildKnowledgeGraphResourceDTO(graph cloud.KnowledgeGraph, nodeLimit int) knowledgeGraphResourceDTO {
	if nodeLimit <= 0 {
		nodeLimit = knowledgeGraphAPINodeLimit
	}
	out := knowledgeGraphResourceDTO{
		Meta:  knowledgeGraphMetaDTO{NodeLimit: nodeLimit, AtNodeLimit: len(graph.Nodes) >= nodeLimit},
		Stats: make(map[string]int, len(knowledgeGraphStatKeys)),
		Nodes: make([]knowledgeGraphNodeDTO, 0, len(graph.Nodes)),
		Edges: make([]knowledgeGraphEdgeDTO, 0, len(graph.Edges)),
	}
	for _, key := range knowledgeGraphStatKeys {
		if value, ok := graph.Stats[key]; ok {
			out.Stats[key] = value
		}
	}

	responseIDs := make(map[string]string, len(graph.Nodes))
	deviceOrdinal := 0
	for _, node := range graph.Nodes {
		if strings.TrimSpace(node.ID) == "" {
			continue
		}
		if _, exists := responseIDs[node.ID]; exists {
			continue
		}
		responseID := "n" + strconv.Itoa(len(out.Nodes)+1)
		responseIDs[node.ID] = responseID
		if node.Kind == "device" {
			deviceOrdinal++
		}
		summary := compactGraphText(node.Summary, 280)
		if node.Kind == "repository" {
			// Store.KnowledgeGraph uses repository remote as this summary. The
			// browser graph only needs the display repository name; keep the full
			// remote/server path out of the client-neutral contract.
			summary = ""
		}
		out.Nodes = append(out.Nodes, knowledgeGraphNodeDTO{
			ID: responseID, Kind: compactGraphText(node.Kind, 64), Name: knowledgeGraphNodeName(node, deviceOrdinal),
			Summary: summary, Scope: compactGraphText(node.Scope, 64), Confidence: graphUnit(node.Confidence),
			Importance: graphUnit(node.Importance), LastSeenAt: node.LastSeenAt,
		})
	}

	for _, edge := range graph.Edges {
		from := responseIDs[edge.From]
		to := responseIDs[edge.To]
		if from == "" || to == "" || from == to {
			continue
		}
		out.Edges = append(out.Edges, knowledgeGraphEdgeDTO{
			ID: "e" + strconv.Itoa(len(out.Edges)+1), From: from, To: to,
			Relation: compactGraphText(edge.Relation, 80), Confidence: graphUnit(edge.Confidence), Importance: graphUnit(edge.Importance),
		})
	}
	out.Stats["nodes"] = len(out.Nodes)
	out.Stats["edges"] = len(out.Edges)
	return out
}

func (s *Server) knowledgeGraphResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	graph, err := s.Store.KnowledgeGraph(r.Context(), identity.User.ID, knowledgeGraphAPINodeLimit)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "knowledge_graph_unavailable"})
		return
	}
	webutil.JSON(w, http.StatusOK, buildKnowledgeGraphResourceDTO(graph, knowledgeGraphAPINodeLimit))
}
