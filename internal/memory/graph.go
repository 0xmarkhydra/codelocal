package memory

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

type graphExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

type GraphNode struct {
	ID             string
	UserID         string
	WorkspaceID    string
	Scope          Scope
	Kind           string
	CanonicalName  string
	Summary        string
	Confidence     float64
	Importance     float64
	ValidFrom      int64
	ValidTo        int64
	FirstSeenAt    int64
	LastSeenAt     int64
	SourceType     string
	SourceSession  string
	SourceMemoryID string
}

type GraphEdge struct {
	ID             string
	UserID         string
	WorkspaceID    string
	Scope          Scope
	FromNodeID     string
	ToNodeID       string
	Relation       string
	Confidence     float64
	Importance     float64
	ValidFrom      int64
	ValidTo        int64
	FirstSeenAt    int64
	LastSeenAt     int64
	SourceSession  string
	SourceMemoryID string
}

type GraphContext struct {
	SeedMemoryIDs []string
	Nodes         []GraphNode
	Edges         []GraphEdge
}

func normalizeScope(scope Scope, workspaceID string) (Scope, string, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if scope == "" {
		scope = ScopeWorkspace
	}
	switch scope {
	case ScopeGlobal:
		return ScopeGlobal, "", nil
	case ScopeWorkspace:
		if workspaceID == "" {
			return "", "", errors.New("workspace memory requires workspace id")
		}
		return ScopeWorkspace, workspaceID, nil
	default:
		return "", "", errors.New("invalid memory scope")
	}
}

func graphID(parts ...string) string {
	return IdempotencyKey(parts...)[:32]
}

func graphKind(level Level) string {
	switch level {
	case LevelScenario:
		return "scenario"
	case LevelWorkspace:
		return "knowledge"
	default:
		return "event"
	}
}

func graphMemoryNode(record Record) GraphNode {
	kind := strings.ToLower(strings.TrimSpace(record.Kind))
	if kind == "" {
		kind = graphKind(record.Level)
	}
	sourceType := strings.ToLower(strings.TrimSpace(record.SourceType))
	if sourceType == "" {
		sourceType = "task"
	}
	canonical := "memory:" + record.ID
	id := graphID(record.UserID, string(record.Scope), record.WorkspaceID, "memory", record.ID)
	if sourceType == "conversation" && record.Kind != "" {
		identity := strings.ToLower(strings.Join(strings.Fields(SanitizeText(record.Summary, 500)), " "))
		for _, symbol := range record.Symbols {
			if strings.HasPrefix(symbol, "memory-key:") && strings.TrimSpace(strings.TrimPrefix(symbol, "memory-key:")) != "" {
				identity = symbol
				break
			}
		}
		// Unkeyed repeated facts still consolidate by normalized wording. Mutable
		// keyed facts consolidate by their stable key, so a changed value updates
		// one graph node instead of leaving contradictory active nodes.
		canonical = kind + ":" + graphID(identity)
		id = graphID(record.UserID, string(record.Scope), record.WorkspaceID, "conversation", canonical)
	}
	freshAt := memoryFreshnessAt(record)
	return GraphNode{
		ID:             id,
		UserID:         record.UserID,
		WorkspaceID:    record.WorkspaceID,
		Scope:          record.Scope,
		Kind:           kind,
		CanonicalName:  canonical,
		Summary:        SanitizeText(record.Summary, 1200),
		Confidence:     normalizeScore(record.Confidence, .7),
		Importance:     normalizeScore(record.Importance, .5),
		ValidFrom:      freshAt,
		FirstSeenAt:    record.CreatedAt,
		LastSeenAt:     max(record.LastUsedAt, freshAt),
		SourceType:     sourceType,
		SourceSession:  record.TaskID,
		SourceMemoryID: record.ID,
	}
}

func graphUserAnchorNode(record Record) GraphNode {
	freshAt := memoryFreshnessAt(record)
	return GraphNode{
		ID:            graphID(record.UserID, "global", "user-anchor"),
		UserID:        record.UserID,
		Scope:         ScopeGlobal,
		Kind:          "user",
		CanonicalName: "user",
		Summary:       "User-global memory anchor",
		Confidence:    1,
		Importance:    1,
		ValidFrom:     freshAt,
		FirstSeenAt:   record.CreatedAt,
		LastSeenAt:    max(record.LastUsedAt, freshAt),
		SourceType:    "system",
	}
}

func graphAnchorNode(record Record) GraphNode {
	if record.Scope == ScopeGlobal {
		return graphUserAnchorNode(record)
	}
	freshAt := memoryFreshnessAt(record)
	return GraphNode{
		ID:            graphID(record.UserID, "workspace", record.WorkspaceID, "workspace-anchor"),
		UserID:        record.UserID,
		WorkspaceID:   record.WorkspaceID,
		Scope:         ScopeWorkspace,
		Kind:          "workspace",
		CanonicalName: record.WorkspaceID,
		Summary:       "Workspace " + record.WorkspaceID,
		Confidence:    1,
		Importance:    1,
		ValidFrom:     freshAt,
		FirstSeenAt:   record.CreatedAt,
		LastSeenAt:    max(record.LastUsedAt, freshAt),
		SourceType:    "system",
	}
}

func graphArtifactNode(record Record, kind, value string) GraphNode {
	value = SanitizeText(value, 300)
	canonical := kind + ":" + strings.ToLower(value)
	return GraphNode{
		ID:             graphID(record.UserID, string(record.Scope), record.WorkspaceID, kind, canonical),
		UserID:         record.UserID,
		WorkspaceID:    record.WorkspaceID,
		Scope:          record.Scope,
		Kind:           "artifact",
		CanonicalName:  canonical,
		Summary:        value,
		Confidence:     .9,
		Importance:     .55,
		ValidFrom:      record.CreatedAt,
		FirstSeenAt:    record.CreatedAt,
		LastSeenAt:     max(record.LastUsedAt, record.CreatedAt),
		SourceType:     "memory",
		SourceSession:  record.TaskID,
		SourceMemoryID: record.ID,
	}
}

func graphRelation(record Record, from, to GraphNode, relation string, importance float64) GraphEdge {
	return GraphEdge{
		ID:             graphID(record.UserID, string(record.Scope), record.WorkspaceID, from.ID, relation, to.ID),
		UserID:         record.UserID,
		WorkspaceID:    record.WorkspaceID,
		Scope:          record.Scope,
		FromNodeID:     from.ID,
		ToNodeID:       to.ID,
		Relation:       relation,
		Confidence:     normalizeScore(record.Confidence, .7),
		Importance:     normalizeScore(importance, .5),
		ValidFrom:      record.CreatedAt,
		FirstSeenAt:    record.CreatedAt,
		LastSeenAt:     max(record.LastUsedAt, record.CreatedAt),
		SourceSession:  record.TaskID,
		SourceMemoryID: record.ID,
	}
}

func graphAnchorEdge(record Record, anchor, memoryNode GraphNode) GraphEdge {
	kind := strings.ToLower(strings.TrimSpace(record.Kind))
	switch kind {
	case "goal":
		return graphRelation(record, anchor, memoryNode, "HAS_GOAL", .9)
	case "preference":
		return graphRelation(record, anchor, memoryNode, "PREFERS", .85)
	case "decision":
		return graphRelation(record, anchor, memoryNode, "HAS_DECISION", .9)
	case "constraint":
		return graphRelation(record, anchor, memoryNode, "HAS_CONSTRAINT", .85)
	case "milestone":
		return graphRelation(record, anchor, memoryNode, "HAS_MILESTONE", .8)
	case "problem":
		return graphRelation(record, anchor, memoryNode, "HAS_PROBLEM", .8)
	case "idea":
		return graphRelation(record, anchor, memoryNode, "HAS_IDEA", .7)
	case "person", "company", "user_fact", "project_fact":
		return graphRelation(record, anchor, memoryNode, "RELATED_TO", .7)
	default:
		return graphRelation(record, memoryNode, anchor, "PART_OF", .8)
	}
}

func (s *Store) SetGraphEnabled(enabled bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.graphEnabled = enabled
	s.mu.Unlock()
}

func (s *Store) GraphEnabled() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.graphEnabled
}

func (s *Store) upsertGraphNode(ctx context.Context, node GraphNode) error {
	return s.upsertGraphNodeWith(ctx, s.db, node)
}

func (s *Store) upsertGraphNodeWith(ctx context.Context, db graphExecer, node GraphNode) error {
	if !s.Enabled() || !s.GraphEnabled() {
		return nil
	}
	metadata, _ := json.Marshal(map[string]any{})
	_, err := db.Exec(ctx, `
INSERT INTO codelocal_memory_nodes(
 id,user_id,workspace_id,scope,kind,canonical_name,summary,confidence,importance,valid_from,valid_to,first_seen_at,last_seen_at,source_type,source_session_id,source_memory_id,metadata)
VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,NULLIF($11,0),$12,$13,$14,NULLIF($15,''),NULLIF($16,''),$17::jsonb)
ON CONFLICT(id) DO UPDATE SET
 summary=CASE WHEN EXCLUDED.summary<>'' THEN EXCLUDED.summary ELSE codelocal_memory_nodes.summary END,
 confidence=GREATEST(codelocal_memory_nodes.confidence,EXCLUDED.confidence),
 importance=GREATEST(codelocal_memory_nodes.importance,EXCLUDED.importance),
 valid_to=EXCLUDED.valid_to,
 last_seen_at=GREATEST(codelocal_memory_nodes.last_seen_at,EXCLUDED.last_seen_at),
 source_memory_id=COALESCE(EXCLUDED.source_memory_id,codelocal_memory_nodes.source_memory_id)`,
		node.ID, node.UserID, node.WorkspaceID, node.Scope, node.Kind, node.CanonicalName, node.Summary,
		node.Confidence, node.Importance, node.ValidFrom, node.ValidTo, node.FirstSeenAt, node.LastSeenAt,
		node.SourceType, node.SourceSession, node.SourceMemoryID, string(metadata),
	)
	return err
}

func (s *Store) upsertGraphEdge(ctx context.Context, edge GraphEdge) error {
	return s.upsertGraphEdgeWith(ctx, s.db, edge)
}

func (s *Store) upsertGraphEdgeWith(ctx context.Context, db graphExecer, edge GraphEdge) error {
	if !s.Enabled() || !s.GraphEnabled() {
		return nil
	}
	metadata, _ := json.Marshal(map[string]any{})
	_, err := db.Exec(ctx, `
INSERT INTO codelocal_memory_edges(
 id,user_id,workspace_id,scope,from_node_id,to_node_id,relation,confidence,importance,valid_from,valid_to,first_seen_at,last_seen_at,source_session_id,source_memory_id,metadata)
VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,NULLIF($11,0),$12,$13,NULLIF($14,''),NULLIF($15,''),$16::jsonb)
ON CONFLICT(id) DO UPDATE SET
 confidence=GREATEST(codelocal_memory_edges.confidence,EXCLUDED.confidence),
 importance=GREATEST(codelocal_memory_edges.importance,EXCLUDED.importance),
 valid_to=EXCLUDED.valid_to,
 last_seen_at=GREATEST(codelocal_memory_edges.last_seen_at,EXCLUDED.last_seen_at),
 source_memory_id=COALESCE(EXCLUDED.source_memory_id,codelocal_memory_edges.source_memory_id)`,
		edge.ID, edge.UserID, edge.WorkspaceID, edge.Scope, edge.FromNodeID, edge.ToNodeID, edge.Relation,
		edge.Confidence, edge.Importance, edge.ValidFrom, edge.ValidTo, edge.FirstSeenAt, edge.LastSeenAt,
		edge.SourceSession, edge.SourceMemoryID, string(metadata),
	)
	return err
}

func (s *Store) upsertGraphSource(ctx context.Context, record Record, nodeID string) error {
	return s.upsertGraphSourceWith(ctx, s.db, record, nodeID)
}

func (s *Store) upsertGraphSourceWith(ctx context.Context, db graphExecer, record Record, nodeID string) error {
	if !s.Enabled() || !s.GraphEnabled() || strings.TrimSpace(nodeID) == "" || strings.TrimSpace(record.ID) == "" {
		return nil
	}
	sourceType := strings.ToLower(strings.TrimSpace(record.SourceType))
	if sourceType == "" {
		sourceType = "task"
	}
	metadata, _ := json.Marshal(map[string]any{"level": record.Level, "kind": record.Kind})
	id := graphID(record.UserID, "source", record.ID, nodeID, record.TaskID)
	createdAt := record.LastUsedAt
	if createdAt <= 0 {
		createdAt = record.CreatedAt
	}
	_, err := db.Exec(ctx, `
INSERT INTO codelocal_memory_sources(
 id,user_id,workspace_id,scope,node_id,source_type,source_session_id,source_memory_id,created_at,metadata)
VALUES($1,$2,NULLIF($3,''),$4,$5,$6,NULLIF($7,''),$8,$9,$10::jsonb)
ON CONFLICT(id) DO UPDATE SET
 node_id=EXCLUDED.node_id,source_type=EXCLUDED.source_type,source_session_id=EXCLUDED.source_session_id,metadata=EXCLUDED.metadata`,
		id, record.UserID, record.WorkspaceID, record.Scope, nodeID, sourceType, record.TaskID, record.ID, createdAt, string(metadata),
	)
	return err
}

func graphProjection(record Record) ([]GraphNode, []GraphEdge, error) {
	scope, workspaceID, err := normalizeScope(record.Scope, record.WorkspaceID)
	if err != nil {
		return nil, nil, err
	}
	record.Scope = scope
	record.WorkspaceID = workspaceID
	memoryNode := graphMemoryNode(record)
	anchor := graphAnchorNode(record)
	nodes := []GraphNode{anchor, memoryNode}
	edges := []GraphEdge{graphAnchorEdge(record, anchor, memoryNode)}
	if record.Scope == ScopeWorkspace {
		userAnchor := graphUserAnchorNode(record)
		nodes = append(nodes, userAnchor)
		edges = append(edges, graphRelation(record, userAnchor, anchor, "WORKS_ON", .9))
	}
	for _, file := range SanitizeList(record.Files, 20) {
		node := graphArtifactNode(record, "file", file)
		nodes = append(nodes, node)
		edges = append(edges, graphRelation(record, memoryNode, node, "RELATED_TO", .65))
	}
	for _, symbol := range SanitizeList(record.Symbols, 20) {
		if strings.HasPrefix(symbol, "memory-key:") {
			continue
		}
		node := graphArtifactNode(record, "symbol", symbol)
		nodes = append(nodes, node)
		edges = append(edges, graphRelation(record, memoryNode, node, "RELATED_TO", .65))
	}
	return nodes, edges, nil
}

func (s *Store) projectRecordToGraph(ctx context.Context, record Record) error {
	if !s.Enabled() || !s.GraphEnabled() {
		return nil
	}
	nodes, edges, err := graphProjection(record)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, node := range nodes {
		if err := s.upsertGraphNodeWith(ctx, tx, node); err != nil {
			return err
		}
	}
	for _, edge := range edges {
		if err := s.upsertGraphEdgeWith(ctx, tx, edge); err != nil {
			return err
		}
	}
	// The source row is the completion marker used by BackfillGraph. Write it
	// last, inside the same transaction, so a partial projection can never be
	// mistaken for a completed backfill.
	if len(nodes) > 1 {
		if err := s.upsertGraphSourceWith(ctx, tx, record, nodes[1].ID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) BackfillGraph(ctx context.Context, limit int) (int, error) {
	if !s.Enabled() || !s.GraphEnabled() {
		return 0, nil
	}
	if limit <= 0 {
		limit = 500
	}
	if limit > 2000 {
		limit = 2000
	}
	rows, err := s.db.Query(ctx, `
SELECT m.id,m.user_id,COALESCE(m.workspace_id,''),m.scope,COALESCE(m.task_id,''),m.level,COALESCE(m.kind,''),m.source_type,m.summary,COALESCE(m.branch,''),m.files,m.symbols,m.confidence,m.importance,m.created_at,m.updated_at,m.last_used_at
FROM codelocal_memories m
WHERE NOT EXISTS (
 SELECT 1 FROM codelocal_memory_sources s
 WHERE s.user_id=m.user_id AND s.source_memory_id=m.id
)
ORDER BY m.created_at ASC
LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	records := []Record{}
	for rows.Next() {
		var record Record
		var filesRaw, symbolsRaw []byte
		if err := rows.Scan(&record.ID, &record.UserID, &record.WorkspaceID, &record.Scope, &record.TaskID, &record.Level, &record.Kind, &record.SourceType, &record.Summary, &record.Branch, &filesRaw, &symbolsRaw, &record.Confidence, &record.Importance, &record.CreatedAt, &record.UpdatedAt, &record.LastUsedAt); err != nil {
			return 0, err
		}
		record.Files = decodeList(filesRaw)
		record.Symbols = decodeList(symbolsRaw)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	count := 0
	for _, record := range records {
		if err := s.projectRecordToGraph(ctx, record); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (s *Store) RecallGraphContext(ctx context.Context, input RecallInput, seeds []Record) (GraphContext, error) {
	out := GraphContext{}
	if !s.Enabled() || !s.GraphEnabled() || len(seeds) == 0 {
		return out, nil
	}
	input.UserID = strings.TrimSpace(input.UserID)
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	if input.UserID == "" {
		return out, nil
	}
	seedMemoryIDs := make([]string, 0, len(seeds))
	for _, seed := range seeds {
		if seed.ID != "" {
			seedMemoryIDs = append(seedMemoryIDs, seed.ID)
		}
	}
	if len(seedMemoryIDs) == 0 {
		return out, nil
	}
	out.SeedMemoryIDs = append([]string(nil), seedMemoryIDs...)
	nodeLimit := max(12, min(40, len(seedMemoryIDs)*4))
	rows, err := s.db.Query(ctx, `
SELECT id,user_id,COALESCE(workspace_id,''),scope,kind,canonical_name,summary,confidence,importance,valid_from,COALESCE(valid_to,0),first_seen_at,last_seen_at,source_type,COALESCE(source_session_id,''),COALESCE(source_memory_id,'')
FROM codelocal_memory_nodes n
WHERE n.user_id=$1 AND n.valid_to IS NULL
 AND (n.scope='global' OR ($3<>'' AND n.scope='workspace' AND n.workspace_id=$3))
 AND (
  n.source_memory_id=ANY($2)
  OR EXISTS (
   SELECT 1 FROM codelocal_memory_sources s
   WHERE s.user_id=$1 AND s.node_id=n.id AND s.source_memory_id=ANY($2)
  )
 )
ORDER BY n.importance DESC,n.confidence DESC,n.last_seen_at DESC
LIMIT $4`, input.UserID, seedMemoryIDs, input.WorkspaceID, nodeLimit)
	if err != nil {
		return out, err
	}
	nodeMap := map[string]GraphNode{}
	seedNodeIDs := []string{}
	for rows.Next() {
		var node GraphNode
		if err := rows.Scan(&node.ID, &node.UserID, &node.WorkspaceID, &node.Scope, &node.Kind, &node.CanonicalName, &node.Summary, &node.Confidence, &node.Importance, &node.ValidFrom, &node.ValidTo, &node.FirstSeenAt, &node.LastSeenAt, &node.SourceType, &node.SourceSession, &node.SourceMemoryID); err != nil {
			rows.Close()
			return out, err
		}
		nodeMap[node.ID] = node
		seedNodeIDs = append(seedNodeIDs, node.ID)
	}
	rows.Close()
	if len(seedNodeIDs) == 0 {
		return out, nil
	}
	edgeLimit := max(16, min(64, len(seedNodeIDs)*5))
	edges, err := s.db.Query(ctx, `
SELECT id,user_id,COALESCE(workspace_id,''),scope,from_node_id,to_node_id,relation,confidence,importance,valid_from,COALESCE(valid_to,0),first_seen_at,last_seen_at,COALESCE(source_session_id,''),COALESCE(source_memory_id,'')
FROM codelocal_memory_edges
WHERE user_id=$1 AND valid_to IS NULL
 AND (scope='global' OR ($3<>'' AND scope='workspace' AND workspace_id=$3))
 AND (from_node_id=ANY($2) OR to_node_id=ANY($2))
ORDER BY importance DESC,confidence DESC,last_seen_at DESC
LIMIT $4`, input.UserID, seedNodeIDs, input.WorkspaceID, edgeLimit)
	if err != nil {
		return out, err
	}
	connectedIDs := append([]string(nil), seedNodeIDs...)
	seenConnected := map[string]struct{}{}
	seenEdges := map[string]struct{}{}
	for _, id := range seedNodeIDs {
		seenConnected[id] = struct{}{}
	}
	appendEdge := func(edge GraphEdge) {
		if _, exists := seenEdges[edge.ID]; exists {
			return
		}
		seenEdges[edge.ID] = struct{}{}
		out.Edges = append(out.Edges, edge)
		for _, id := range []string{edge.FromNodeID, edge.ToNodeID} {
			if _, ok := seenConnected[id]; !ok {
				seenConnected[id] = struct{}{}
				connectedIDs = append(connectedIDs, id)
			}
		}
	}
	for edges.Next() {
		var edge GraphEdge
		if err := edges.Scan(&edge.ID, &edge.UserID, &edge.WorkspaceID, &edge.Scope, &edge.FromNodeID, &edge.ToNodeID, &edge.Relation, &edge.Confidence, &edge.Importance, &edge.ValidFrom, &edge.ValidTo, &edge.FirstSeenAt, &edge.LastSeenAt, &edge.SourceSession, &edge.SourceMemoryID); err != nil {
			edges.Close()
			return out, err
		}
		appendEdge(edge)
	}
	if err := edges.Err(); err != nil {
		edges.Close()
		return out, err
	}
	edges.Close()

	// Expand one additional bounded hop. This lets a seed memory reach its
	// user/workspace anchor and then related goals/preferences/decisions without
	// turning recall into an unbounded graph walk.
	if len(connectedIDs) > 0 && len(connectedIDs) < nodeLimit {
		hopSeeds := append([]string(nil), connectedIDs...)
		secondEdgeLimit := max(16, min(64, (nodeLimit-len(connectedIDs)+len(hopSeeds))*4))
		secondEdges, secondErr := s.db.Query(ctx, `
SELECT id,user_id,COALESCE(workspace_id,''),scope,from_node_id,to_node_id,relation,confidence,importance,valid_from,COALESCE(valid_to,0),first_seen_at,last_seen_at,COALESCE(source_session_id,''),COALESCE(source_memory_id,'')
FROM codelocal_memory_edges
WHERE user_id=$1 AND valid_to IS NULL
 AND (scope='global' OR ($3<>'' AND scope='workspace' AND workspace_id=$3))
 AND (from_node_id=ANY($2) OR to_node_id=ANY($2))
ORDER BY importance DESC,confidence DESC,last_seen_at DESC
LIMIT $4`, input.UserID, hopSeeds, input.WorkspaceID, secondEdgeLimit)
		if secondErr != nil {
			return out, secondErr
		}
		for secondEdges.Next() {
			var edge GraphEdge
			if err := secondEdges.Scan(&edge.ID, &edge.UserID, &edge.WorkspaceID, &edge.Scope, &edge.FromNodeID, &edge.ToNodeID, &edge.Relation, &edge.Confidence, &edge.Importance, &edge.ValidFrom, &edge.ValidTo, &edge.FirstSeenAt, &edge.LastSeenAt, &edge.SourceSession, &edge.SourceMemoryID); err != nil {
				secondEdges.Close()
				return out, err
			}
			appendEdge(edge)
			if len(connectedIDs) >= nodeLimit {
				break
			}
		}
		if err := secondEdges.Err(); err != nil {
			secondEdges.Close()
			return out, err
		}
		secondEdges.Close()
	}
	if len(connectedIDs) > nodeLimit {
		connectedIDs = connectedIDs[:nodeLimit]
	}
	keptNodeIDs := make(map[string]struct{}, len(connectedIDs))
	for _, id := range connectedIDs {
		keptNodeIDs[id] = struct{}{}
	}
	filteredEdges := out.Edges[:0]
	for _, edge := range out.Edges {
		_, fromKept := keptNodeIDs[edge.FromNodeID]
		_, toKept := keptNodeIDs[edge.ToNodeID]
		if fromKept && toKept {
			filteredEdges = append(filteredEdges, edge)
		}
	}
	out.Edges = filteredEdges
	connectedRows, err := s.db.Query(ctx, `
SELECT id,user_id,COALESCE(workspace_id,''),scope,kind,canonical_name,summary,confidence,importance,valid_from,COALESCE(valid_to,0),first_seen_at,last_seen_at,source_type,COALESCE(source_session_id,''),COALESCE(source_memory_id,'')
FROM codelocal_memory_nodes
WHERE user_id=$1 AND id=ANY($2) AND valid_to IS NULL
 AND (scope='global' OR ($3<>'' AND scope='workspace' AND workspace_id=$3))
ORDER BY importance DESC,confidence DESC,last_seen_at DESC
LIMIT $4`, input.UserID, connectedIDs, input.WorkspaceID, nodeLimit)
	if err != nil {
		return out, err
	}
	for connectedRows.Next() {
		var node GraphNode
		if err := connectedRows.Scan(&node.ID, &node.UserID, &node.WorkspaceID, &node.Scope, &node.Kind, &node.CanonicalName, &node.Summary, &node.Confidence, &node.Importance, &node.ValidFrom, &node.ValidTo, &node.FirstSeenAt, &node.LastSeenAt, &node.SourceType, &node.SourceSession, &node.SourceMemoryID); err != nil {
			connectedRows.Close()
			return out, err
		}
		nodeMap[node.ID] = node
	}
	if err := connectedRows.Err(); err != nil {
		connectedRows.Close()
		return out, err
	}
	connectedRows.Close()
	out.Nodes = make([]GraphNode, 0, len(nodeMap))
	for _, id := range connectedIDs {
		if node, ok := nodeMap[id]; ok {
			out.Nodes = append(out.Nodes, node)
		}
	}
	return out, nil
}
