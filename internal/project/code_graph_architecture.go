package project

import (
	"sort"
	"strings"
)

type architectureCluster struct {
	Node  CodeGraphNode
	Files map[string]struct{}
}

func codeGraphViewMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "files":
		return "files"
	default:
		return "architecture"
	}
}

func repositoryRelativeGraphPath(path string, snapshot *CodeGraphSnapshot) string {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if snapshot == nil || snapshot.RepositoryPath == "" || snapshot.RepositoryPath == "." {
		return path
	}
	prefix := strings.Trim(snapshot.RepositoryPath, "/")
	if path == prefix {
		return ""
	}
	return strings.TrimPrefix(path, prefix+"/")
}

func architectureClusterName(path string, snapshot *CodeGraphSnapshot) string {
	rel := repositoryRelativeGraphPath(path, snapshot)
	if rel == "" || !strings.Contains(rel, "/") {
		return "(root)"
	}
	parts := strings.Split(rel, "/")
	if len(parts) == 1 {
		return parts[0]
	}
	switch parts[0] {
	case "internal", "cmd", "pkg", "packages", "apps", "services", "modules":
		return strings.Join(parts[:min(2, len(parts))], "/")
	default:
		return parts[0]
	}
}

func externalArchitectureName(specifier string) string {
	specifier = strings.TrimSpace(specifier)
	if specifier == "" {
		return "external"
	}
	if strings.HasPrefix(specifier, "node:") {
		return specifier
	}
	parts := strings.Split(specifier, "/")
	if strings.HasPrefix(specifier, "github.com/") && len(parts) >= 3 {
		return strings.Join(parts[:3], "/")
	}
	if strings.HasPrefix(specifier, "golang.org/") && len(parts) >= 3 {
		return strings.Join(parts[:3], "/")
	}
	if strings.HasPrefix(specifier, "@") && len(parts) >= 2 {
		return strings.Join(parts[:2], "/")
	}
	return parts[0]
}

func architectureNode(repositoryID, repositoryPath, name, kind string) CodeGraphNode {
	return CodeGraphNode{
		ID: "arch_" + codeGraphHash(repositoryID, kind, name), Kind: kind, Name: name,
		RepositoryID: repositoryID, RepositoryPath: repositoryPath, Provider: "structural-index",
		ResolutionMode: "structural", Confidence: .8,
	}
}

func architectureClusterKey(repositoryID, name, kind string) string {
	return repositoryID + "\x00" + kind + "\x00" + name
}

func architectureClusterFor(clusters map[string]*architectureCluster, repositoryID, repositoryPath, name, kind string) *architectureCluster {
	key := architectureClusterKey(repositoryID, name, kind)
	if item := clusters[key]; item != nil {
		return item
	}
	item := &architectureCluster{Node: architectureNode(repositoryID, repositoryPath, name, kind), Files: map[string]struct{}{}}
	clusters[key] = item
	return item
}

func architectureEdgeKey(from, to CodeGraphNode) string {
	return from.ID + "\x00" + to.ID
}

func architectureGraph(revision string, rawEdges []map[string]any, snapshot *CodeGraphSnapshot, maxNodes int) ([]CodeGraphNode, []CodeGraphEdge, bool) {
	clusters := map[string]*architectureCluster{}
	edges := map[string]CodeGraphEdge{}
	truncated := false
	for _, raw := range rawEdges {
		fromPath := graphValue(raw, "from")
		fromRepo := graphValue(raw, "fromRepositoryId")
		fromRepoPath := graphValue(raw, "fromRepositoryPath")
		if snapshot != nil && fromRepo != snapshot.RepositoryID && fromRepoPath != snapshot.RepositoryPath {
			continue
		}
		fromName := architectureClusterName(fromPath, snapshot)
		toRepo := graphValue(raw, "toRepositoryId")
		toRepoPath := graphValue(raw, "toRepositoryPath")
		toPath := graphValue(raw, "to")
		toName, toKind := externalArchitectureName(graphValue(raw, "specifier")), "external"
		if toRepo != "" {
			toName, toKind = architectureClusterName(toPath, snapshot), "module"
		}
		fromKey := architectureClusterKey(fromRepo, fromName, "module")
		toKey := architectureClusterKey(toRepo, toName, toKind)
		missing := 0
		if clusters[fromKey] == nil {
			missing++
		}
		if clusters[toKey] == nil && toKey != fromKey {
			missing++
		}
		if len(clusters)+missing > maxNodes {
			truncated = true
			break
		}
		from := architectureClusterFor(clusters, fromRepo, fromRepoPath, fromName, "module")
		from.Files[fromPath] = struct{}{}
		to := architectureClusterFor(clusters, toRepo, toRepoPath, toName, toKind)
		if toKind == "module" {
			to.Files[toPath] = struct{}{}
		}
		if from.Node.ID == to.Node.ID {
			continue
		}
		key := architectureEdgeKey(from.Node, to.Node)
		edge := edges[key]
		if edge.ID == "" {
			evidence := map[string]any{"provider": "structural-index", "resolutionMode": "structural", "confidence": .8}
			edge = codeGraphEdge(revision, from.Node, to.Node, "IMPORTS", evidence)
		}
		edge.Count++
		edges[key] = edge
		if len(clusters) >= maxNodes {
			truncated = true
			break
		}
	}

	nodes := make([]CodeGraphNode, 0, len(clusters))
	for _, cluster := range clusters {
		node := cluster.Node
		if node.Kind == "module" {
			node.Summary = intString(len(cluster.Files)) + " files · structural module cluster"
		} else {
			node.Summary = "external dependency cluster"
		}
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Kind != nodes[j].Kind {
			return nodes[i].Kind < nodes[j].Kind
		}
		return nodes[i].Name < nodes[j].Name
	})
	outEdges := make([]CodeGraphEdge, 0, len(edges))
	for _, edge := range edges {
		outEdges = append(outEdges, edge)
	}
	sort.Slice(outEdges, func(i, j int) bool {
		if outEdges[i].Count != outEdges[j].Count {
			return outEdges[i].Count > outEdges[j].Count
		}
		return outEdges[i].ID < outEdges[j].ID
	})
	return nodes, outEdges, truncated
}

func intString(value int) string {
	if value == 0 {
		return "0"
	}
	buf := [32]byte{}
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = byte('0' + value%10)
		value /= 10
	}
	return string(buf[i:])
}

func (e *Engine) codeGraphArchitecture(repository string, depth, maxNodes int, snapshots []CodeGraphSnapshot) (CodeGraphView, error) {
	snapshot := snapshotForRepository(snapshots, repository)
	if snapshot == nil && strings.TrimSpace(repository) != "" {
		return CodeGraphView{Status: "empty", View: "architecture", Depth: depth, MaxNodes: maxNodes, Snapshots: snapshots, Nodes: []CodeGraphNode{}, Edges: []CodeGraphEdge{}, GeneratedBy: "local-runtime"}, nil
	}
	revision := ""
	if snapshot != nil {
		revision = snapshot.Revision
	}
	rawEdges, err := e.ImportGraph(maxNodes * 12)
	if err != nil {
		return CodeGraphView{}, err
	}
	nodes, edges, truncated := architectureGraph(revision, rawEdges, snapshot, maxNodes)
	status := "current"
	if len(nodes) == 0 {
		status = "empty"
	}
	return CodeGraphView{
		Status: status, View: "architecture", Depth: depth, MaxNodes: maxNodes, Snapshots: snapshots,
		Snapshot: snapshot, Nodes: nodes, Edges: edges, Truncated: truncated, GeneratedBy: "local-runtime",
	}, nil
}
