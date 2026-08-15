package cloud

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
)

func knowledgeRepoName(remote, id string) string {
	remote = strings.Trim(strings.TrimSpace(remote), "/")
	if remote != "" {
		if name := path.Base(remote); name != "." && name != "/" && name != "" {
			return name
		}
	}
	if len(id) > 12 {
		return "repo-" + id[:12]
	}
	return "repository"
}

func knowledgeMemoryKind(kind string) string {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return "memory"
	}
	return kind
}

func knowledgeMemoryName(kind, summary string) string {
	kind = knowledgeMemoryKind(kind)
	summary = strings.Join(strings.Fields(strings.TrimSpace(summary)), " ")
	runes := []rune(summary)
	if len(runes) > 72 {
		summary = string(runes[:72]) + "…"
	}
	if summary == "" {
		return kind
	}
	return kind + ": " + summary
}

func (s *Store) KnowledgeGraph(ctx context.Context, userID string, limit int) (KnowledgeGraph, error) {
	out := KnowledgeGraph{Stats: map[string]int{}}
	if s == nil || s.DB == nil || strings.TrimSpace(userID) == "" {
		return out, nil
	}
	if limit < 50 {
		limit = 250
	}
	if limit > 1000 {
		limit = 1000
	}
	nodes := map[string]KnowledgeNode{}
	edges := map[string]KnowledgeEdge{}
	addNode := func(node KnowledgeNode) {
		if node.ID == "" || len(nodes) >= limit {
			return
		}
		if existing, ok := nodes[node.ID]; ok {
			if existing.LastSeenAt >= node.LastSeenAt {
				return
			}
		}
		nodes[node.ID] = node
	}
	addEdge := func(edge KnowledgeEdge) {
		if edge.ID == "" || edge.From == "" || edge.To == "" {
			return
		}
		edges[edge.ID] = edge
	}

	userNode := "user:" + userID
	addNode(KnowledgeNode{ID: userNode, Kind: "user", Name: "You", Scope: "global", Confidence: 1, Importance: 1})

	projectRows, err := s.DB.Query(ctx, `SELECT project_id,name,last_seen_at FROM codelocal_projects WHERE user_id=$1 ORDER BY last_seen_at DESC LIMIT 100`, userID)
	if err != nil {
		return out, err
	}
	projectIDs := []string{}
	for projectRows.Next() {
		var projectID, name string
		var lastSeen int64
		if err := projectRows.Scan(&projectID, &name, &lastSeen); err != nil {
			projectRows.Close()
			return out, err
		}
		id := "project:" + projectID
		addNode(KnowledgeNode{ID: id, Kind: "project", Name: name, Scope: "project", Confidence: 1, Importance: .95, LastSeenAt: lastSeen})
		addEdge(KnowledgeEdge{ID: "works-on:" + projectID, From: userNode, To: id, Relation: "WORKS_ON", Confidence: 1, Importance: .95})
		projectIDs = append(projectIDs, projectID)
	}
	if err := projectRows.Err(); err != nil {
		projectRows.Close()
		return out, err
	}
	projectRows.Close()
	out.Stats["projects"] = len(projectIDs)

	repoRows, err := s.DB.Query(ctx, `
SELECT pr.project_id,r.repository_id,COALESCE(r.remote,''),r.identity_source,r.last_seen_at
FROM codelocal_project_repositories pr
JOIN codelocal_repositories r ON r.user_id=pr.user_id AND r.repository_id=pr.repository_id
WHERE pr.user_id=$1
ORDER BY pr.last_seen_at DESC
LIMIT 300`, userID)
	if err != nil {
		return out, err
	}
	repoSeen := map[string]struct{}{}
	for repoRows.Next() {
		var projectID, repoID, remote, source string
		var lastSeen int64
		if err := repoRows.Scan(&projectID, &repoID, &remote, &source, &lastSeen); err != nil {
			repoRows.Close()
			return out, err
		}
		nodeID := "repo:" + repoID
		addNode(KnowledgeNode{ID: nodeID, Kind: "repository", Name: knowledgeRepoName(remote, repoID), Summary: remote, Scope: "repository", Confidence: 1, Importance: .75, LastSeenAt: lastSeen})
		addEdge(KnowledgeEdge{ID: "project-repo:" + projectID + ":" + repoID, From: "project:" + projectID, To: nodeID, Relation: "CONTAINS_REPO", Confidence: 1, Importance: .8})
		repoSeen[repoID] = struct{}{}
		_ = source
	}
	if err := repoRows.Err(); err != nil {
		repoRows.Close()
		return out, err
	}
	repoRows.Close()
	out.Stats["repositories"] = len(repoSeen)

	workspaceRows, err := s.DB.Query(ctx, `
SELECT w.device_id,w.workspace_id,w.workspace_name,w.last_seen_at,wp.project_id,wp.confidence
FROM codelocal_workspace_projects wp
JOIN codelocal_workspaces w ON w.user_id=wp.user_id AND w.device_id=wp.device_id AND w.workspace_id=wp.workspace_id
WHERE wp.user_id=$1
ORDER BY w.last_seen_at DESC
LIMIT 200`, userID)
	if err != nil {
		return out, err
	}
	workspaceCount := 0
	deviceSeen := map[string]struct{}{}
	workspaceNodes := map[string]string{}
	workspaceProjects := map[string]string{}
	workspaceIDProjects := map[string]map[string]struct{}{}
	for workspaceRows.Next() {
		var deviceID, workspaceID, workspaceName, projectID string
		var lastSeen int64
		var confidence float64
		if err := workspaceRows.Scan(&deviceID, &workspaceID, &workspaceName, &lastSeen, &projectID, &confidence); err != nil {
			workspaceRows.Close()
			return out, err
		}
		deviceNode := "device:" + deviceID
		workspaceNode := "workspace:" + deviceID + ":" + workspaceID
		workspaceKey := deviceID + "\x00" + workspaceID
		workspaceNodes[workspaceKey] = workspaceNode
		workspaceProjects[workspaceKey] = projectID
		if workspaceIDProjects[workspaceID] == nil {
			workspaceIDProjects[workspaceID] = map[string]struct{}{}
		}
		workspaceIDProjects[workspaceID][projectID] = struct{}{}
		addNode(KnowledgeNode{ID: deviceNode, Kind: "device", Name: deviceID, Scope: "device", Confidence: 1, Importance: .55, LastSeenAt: lastSeen})
		addNode(KnowledgeNode{ID: workspaceNode, Kind: "workspace", Name: workspaceName, Scope: "workspace", Confidence: confidence, Importance: .65, LastSeenAt: lastSeen})
		addEdge(KnowledgeEdge{ID: "project-workspace:" + projectID + ":" + deviceID + ":" + workspaceID, From: "project:" + projectID, To: workspaceNode, Relation: "HAS_CHECKOUT", Confidence: confidence, Importance: .7})
		addEdge(KnowledgeEdge{ID: "device-workspace:" + deviceID + ":" + workspaceID, From: deviceNode, To: workspaceNode, Relation: "HOSTS", Confidence: 1, Importance: .55})
		workspaceCount++
		deviceSeen[deviceID] = struct{}{}
	}
	if err := workspaceRows.Err(); err != nil {
		workspaceRows.Close()
		return out, err
	}
	workspaceRows.Close()
	out.Stats["workspaces"] = workspaceCount
	out.Stats["devices"] = len(deviceSeen)

	skillRows, err := s.DB.Query(ctx, `
SELECT device_id,workspace_id,skill_id,intent,status,confidence,success_count,failure_count,step_count,updated_at,last_used_at
FROM codelocal_learned_skill_metadata
WHERE user_id=$1
ORDER BY GREATEST(last_used_at,updated_at) DESC
LIMIT 300`, userID)
	if err != nil {
		return out, err
	}
	skillCount := 0
	for skillRows.Next() {
		var deviceID, workspaceID, skillID, intent, status string
		var confidence float64
		var successCount, failureCount, stepCount int
		var updatedAt, lastUsedAt int64
		if err := skillRows.Scan(&deviceID, &workspaceID, &skillID, &intent, &status, &confidence, &successCount, &failureCount, &stepCount, &updatedAt, &lastUsedAt); err != nil {
			skillRows.Close()
			return out, err
		}
		workspaceKey := deviceID + "\x00" + workspaceID
		workspaceNode := workspaceNodes[workspaceKey]
		if workspaceNode == "" {
			continue
		}
		nodeID := "skill:" + deviceID + ":" + workspaceID + ":" + skillID
		lastSeen := max(updatedAt, lastUsedAt)
		summary := fmt.Sprintf("%s · %d successful · %d failed · %d steps · local recipe remains private", status, successCount, failureCount, stepCount)
		addNode(KnowledgeNode{ID: nodeID, Kind: "skill", Name: intent, Summary: summary, Scope: "skill", Confidence: confidence, Importance: .72, LastSeenAt: lastSeen})
		addEdge(KnowledgeEdge{ID: "workspace-skill:" + nodeID, From: workspaceNode, To: nodeID, Relation: "HAS_SKILL", Confidence: confidence, Importance: .72})
		if projectID := workspaceProjects[workspaceKey]; projectID != "" {
			addEdge(KnowledgeEdge{ID: "project-skill:" + projectID + ":" + nodeID, From: "project:" + projectID, To: nodeID, Relation: "USES_SKILL", Confidence: confidence, Importance: .68})
		}
		skillCount++
	}
	if err := skillRows.Err(); err != nil {
		skillRows.Close()
		return out, err
	}
	skillRows.Close()
	out.Stats["skills"] = skillCount

	knowledgeRows, err := s.DB.Query(ctx, `
SELECT s.project_id,COALESCE(s.repository_id,''),s.source_id,s.provider,s.source_type,s.canonical_path,s.classification,s.status,s.last_seen_at,
       COALESCE(s.active_revision_id,''),COALESCE(r.content_hash,''),
       COALESCE((SELECT COUNT(*) FROM codelocal_knowledge_conflicts c WHERE c.user_id=s.user_id AND c.source_id=s.source_id AND c.status='open'),0)::int
FROM codelocal_knowledge_sources s
LEFT JOIN codelocal_knowledge_source_revisions r ON r.user_id=s.user_id AND r.revision_id=s.active_revision_id
WHERE s.user_id=$1
ORDER BY CASE WHEN s.status='conflicted' THEN 0 ELSE 1 END,s.last_seen_at DESC
LIMIT 400`, userID)
	if err != nil {
		return out, err
	}
	knowledgeCount := 0
	conflictedSources := 0
	for knowledgeRows.Next() {
		var projectID, repositoryID, sourceID, provider, sourceType, canonicalPath, classification, status, revisionID, contentHash string
		var lastSeen int64
		var openConflicts int
		if err := knowledgeRows.Scan(&projectID, &repositoryID, &sourceID, &provider, &sourceType, &canonicalPath, &classification, &status, &lastSeen, &revisionID, &contentHash, &openConflicts); err != nil {
			knowledgeRows.Close()
			return out, err
		}
		nodeID := "knowledge-source:" + sourceID
		revisionSummary := revisionID
		if len(revisionSummary) > 18 {
			revisionSummary = revisionSummary[:18] + "…"
		}
		hashSummary := contentHash
		if len(hashSummary) > 12 {
			hashSummary = hashSummary[:12]
		}
		summary := fmt.Sprintf("%s · %s · %s · revision %s · content %s", provider, sourceType, status, revisionSummary, hashSummary)
		importance := .7
		if status == KnowledgeStatusConflicted {
			importance = .93
			conflictedSources++
		}
		addNode(KnowledgeNode{ID: nodeID, Kind: "knowledge_source", Name: canonicalPath, Summary: summary, Scope: classification, Confidence: 1, Importance: importance, LastSeenAt: lastSeen})
		if projectID != "" {
			addEdge(KnowledgeEdge{ID: "project-knowledge:" + projectID + ":" + sourceID, From: "project:" + projectID, To: nodeID, Relation: "HAS_KNOWLEDGE_SOURCE", Confidence: 1, Importance: importance})
		}
		if repositoryID != "" {
			addEdge(KnowledgeEdge{ID: "repo-knowledge:" + repositoryID + ":" + sourceID, From: "repo:" + repositoryID, To: nodeID, Relation: "HAS_KNOWLEDGE_SOURCE", Confidence: 1, Importance: importance})
		}
		if openConflicts > 0 {
			conflictNode := "knowledge-conflicts:" + sourceID
			addNode(KnowledgeNode{ID: conflictNode, Kind: "conflict", Name: fmt.Sprintf("%d unresolved knowledge conflict(s)", openConflicts), Summary: "Concurrent source revisions require explicit resolution; no silent last-write-wins.", Scope: "project", Confidence: 1, Importance: 1, LastSeenAt: lastSeen})
			addEdge(KnowledgeEdge{ID: "knowledge-conflict:" + sourceID, From: nodeID, To: conflictNode, Relation: "HAS_CONFLICT", Confidence: 1, Importance: 1})
		}
		knowledgeCount++
	}
	if err := knowledgeRows.Err(); err != nil {
		knowledgeRows.Close()
		return out, err
	}
	knowledgeRows.Close()
	out.Stats["knowledgeSources"] = knowledgeCount
	out.Stats["conflictedKnowledgeSources"] = conflictedSources

	experienceRows, err := s.DB.Query(ctx, `
SELECT experience_id,COALESCE(project_id,''),COALESCE(repository_id,''),COALESCE(task_kind,''),objective,outcome,verification_summary,created_at
FROM codelocal_experiences
WHERE user_id=$1
ORDER BY created_at DESC
LIMIT 250`, userID)
	if err != nil {
		return out, err
	}
	experienceCount := 0
	for experienceRows.Next() {
		var experienceID, projectID, repositoryID, taskKind, objective, outcome, verificationSummary string
		var createdAt int64
		if err := experienceRows.Scan(&experienceID, &projectID, &repositoryID, &taskKind, &objective, &outcome, &verificationSummary, &createdAt); err != nil {
			experienceRows.Close()
			return out, err
		}
		nodeID := "experience:" + experienceID
		name := objective
		runes := []rune(name)
		if len(runes) > 80 {
			name = string(runes[:80]) + "…"
		}
		summary := strings.TrimSpace(taskKind + " · " + outcome + " · " + verificationSummary)
		addNode(KnowledgeNode{ID: nodeID, Kind: "experience", Name: name, Summary: summary, Scope: "project", Confidence: 1, Importance: .82, LastSeenAt: createdAt})
		if projectID != "" {
			addEdge(KnowledgeEdge{ID: "project-experience:" + projectID + ":" + experienceID, From: "project:" + projectID, To: nodeID, Relation: "HAS_VERIFIED_EXPERIENCE", Confidence: 1, Importance: .82})
		}
		if repositoryID != "" {
			addEdge(KnowledgeEdge{ID: "repo-experience:" + repositoryID + ":" + experienceID, From: "repo:" + repositoryID, To: nodeID, Relation: "HAS_VERIFIED_EXPERIENCE", Confidence: 1, Importance: .78})
		}
		experienceCount++
	}
	if err := experienceRows.Err(); err != nil {
		experienceRows.Close()
		return out, err
	}
	experienceRows.Close()
	out.Stats["experiences"] = experienceCount

	// Project/repository-scoped memories live directly in the base memory table
	// while the legacy graph schema is migrated gradually. Surface them in the
	// dashboard immediately and anchor them to the logical Project/Repository so
	// users can inspect the same knowledge that token-aware recall uses.
	nativeRemaining := max(0, limit-len(nodes))
	if nativeRemaining > 0 {
		nativeRows, err := s.DB.Query(ctx, `
SELECT id,COALESCE(project_id,''),COALESCE(repository_id,''),scope,COALESCE(kind,''),summary,lifecycle_status,confidence,importance,GREATEST(created_at,updated_at,last_used_at)
FROM codelocal_memories
WHERE user_id=$1 AND scope IN ('project','repository') AND lifecycle_status NOT IN ('invalidated','superseded')
ORDER BY importance DESC,confidence DESC,GREATEST(created_at,updated_at,last_used_at) DESC
LIMIT $2`, userID, nativeRemaining)
		if err != nil {
			return out, err
		}
		nativeCount := 0
		for nativeRows.Next() {
			var memoryID, projectID, repositoryID, scope, kind, summary, lifecycle string
			var confidence, importance float64
			var lastSeen int64
			if err := nativeRows.Scan(&memoryID, &projectID, &repositoryID, &scope, &kind, &summary, &lifecycle, &confidence, &importance, &lastSeen); err != nil {
				nativeRows.Close()
				return out, err
			}
			nodeID := "native-memory:" + memoryID
			addNode(KnowledgeNode{ID: nodeID, Kind: knowledgeMemoryKind(kind), Name: knowledgeMemoryName(kind, summary), Summary: lifecycle + " · " + summary, Scope: scope, Confidence: confidence, Importance: importance, LastSeenAt: lastSeen, SourceMemoryID: memoryID})
			switch scope {
			case "repository":
				if repositoryID != "" {
					addEdge(KnowledgeEdge{ID: "repo-memory:" + repositoryID + ":" + memoryID, From: "repo:" + repositoryID, To: nodeID, Relation: "HAS_MEMORY", Confidence: confidence, Importance: importance})
				}
			case "project":
				if projectID != "" {
					addEdge(KnowledgeEdge{ID: "project-memory:" + projectID + ":" + memoryID, From: "project:" + projectID, To: nodeID, Relation: "HAS_MEMORY", Confidence: confidence, Importance: importance})
				}
			}
			nativeCount++
		}
		if err := nativeRows.Err(); err != nil {
			nativeRows.Close()
			return out, err
		}
		nativeRows.Close()
		out.Stats["nativeMemories"] = nativeCount
	}

	remaining := max(0, limit-len(nodes))
	if remaining > 0 {
		memoryRows, err := s.DB.Query(ctx, `
SELECT id,COALESCE(workspace_id,''),scope,kind,canonical_name,summary,confidence,importance,last_seen_at,COALESCE(source_memory_id,'')
FROM codelocal_memory_nodes
WHERE user_id=$1 AND valid_to IS NULL
ORDER BY importance DESC,confidence DESC,last_seen_at DESC
LIMIT $2`, userID, remaining)
		if err != nil {
			return out, err
		}
		memoryIDs := map[string]string{}
		workspaceAnchors := map[string][]string{}
		for memoryRows.Next() {
			var rawID, workspaceID, scope, kind, name, summary, sourceMemoryID string
			var confidence, importance float64
			var lastSeen int64
			if err := memoryRows.Scan(&rawID, &workspaceID, &scope, &kind, &name, &summary, &confidence, &importance, &lastSeen, &sourceMemoryID); err != nil {
				memoryRows.Close()
				return out, err
			}
			id := "memory:" + rawID
			memoryIDs[rawID] = id
			addNode(KnowledgeNode{ID: id, Kind: kind, Name: name, Summary: summary, Scope: scope, Confidence: confidence, Importance: importance, LastSeenAt: lastSeen, SourceMemoryID: sourceMemoryID})
			if kind == "workspace" && workspaceID != "" {
				workspaceAnchors[workspaceID] = append(workspaceAnchors[workspaceID], id)
			}
		}
		if err := memoryRows.Err(); err != nil {
			memoryRows.Close()
			return out, err
		}
		memoryRows.Close()
		out.Stats["memories"] = len(memoryIDs)

		if len(memoryIDs) > 0 {
			edgeRows, err := s.DB.Query(ctx, `
SELECT id,from_node_id,to_node_id,relation,confidence,importance
FROM codelocal_memory_edges
WHERE user_id=$1 AND valid_to IS NULL
ORDER BY importance DESC,confidence DESC,last_seen_at DESC
LIMIT 1200`, userID)
			if err != nil {
				return out, err
			}
			for edgeRows.Next() {
				var id, fromRaw, toRaw, relation string
				var confidence, importance float64
				if err := edgeRows.Scan(&id, &fromRaw, &toRaw, &relation, &confidence, &importance); err != nil {
					edgeRows.Close()
					return out, err
				}
				from, fromOK := memoryIDs[fromRaw]
				to, toOK := memoryIDs[toRaw]
				if fromOK && toOK {
					addEdge(KnowledgeEdge{ID: "memory-edge:" + id, From: from, To: to, Relation: relation, Confidence: confidence, Importance: importance})
				}
			}
			if err := edgeRows.Err(); err != nil {
				edgeRows.Close()
				return out, err
			}
			edgeRows.Close()
		}

		// Legacy memory rows only carry workspace_id, not device_id. Link them to
		// concrete checkouts only when that workspace id resolves to one logical
		// project for this user. If two devices reuse the same workspace id for
		// different projects, skipping the edge is safer than cross-project graph
		// contamination.
		for workspaceID, anchors := range workspaceAnchors {
			if len(workspaceIDProjects[workspaceID]) != 1 {
				continue
			}
			for nodeID, node := range nodes {
				if node.Kind != "workspace" || !strings.HasSuffix(nodeID, ":"+workspaceID) {
					continue
				}
				for _, anchor := range anchors {
					id := fmt.Sprintf("workspace-memory:%s:%s", nodeID, anchor)
					addEdge(KnowledgeEdge{ID: id, From: nodeID, To: anchor, Relation: "HAS_MEMORY", Confidence: .9, Importance: .65})
				}
			}
		}
	}

	out.Nodes = make([]KnowledgeNode, 0, len(nodes))
	for _, node := range nodes {
		out.Nodes = append(out.Nodes, node)
	}
	sort.Slice(out.Nodes, func(i, j int) bool {
		if out.Nodes[i].Importance != out.Nodes[j].Importance {
			return out.Nodes[i].Importance > out.Nodes[j].Importance
		}
		if out.Nodes[i].LastSeenAt != out.Nodes[j].LastSeenAt {
			return out.Nodes[i].LastSeenAt > out.Nodes[j].LastSeenAt
		}
		return out.Nodes[i].ID < out.Nodes[j].ID
	})
	kept := map[string]struct{}{}
	for _, node := range out.Nodes {
		kept[node.ID] = struct{}{}
	}
	out.Edges = make([]KnowledgeEdge, 0, len(edges))
	for _, edge := range edges {
		if _, ok := kept[edge.From]; !ok {
			continue
		}
		if _, ok := kept[edge.To]; !ok {
			continue
		}
		out.Edges = append(out.Edges, edge)
	}
	sort.Slice(out.Edges, func(i, j int) bool {
		if out.Edges[i].Importance != out.Edges[j].Importance {
			return out.Edges[i].Importance > out.Edges[j].Importance
		}
		return out.Edges[i].ID < out.Edges[j].ID
	})
	out.Stats["nodes"] = len(out.Nodes)
	out.Stats["edges"] = len(out.Edges)
	return out, nil
}
