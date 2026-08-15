package cloud

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	longmemory "github.com/0xmarkhydra/codelocal/internal/memory"
	"github.com/jackc/pgx/v5"
)

const canonicalKnowledgeGraphMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_knowledge_graph_nodes (
 user_id TEXT NOT NULL,
 project_id TEXT NOT NULL,
 node_id TEXT NOT NULL,
 knowledge_id TEXT NOT NULL,
 revision_id TEXT,
 repository_id TEXT,
 node_kind TEXT NOT NULL CHECK (node_kind IN ('knowledge','revision')),
 label TEXT NOT NULL,
 summary TEXT NOT NULL DEFAULT '',
 status TEXT NOT NULL,
 confidence DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
 importance DOUBLE PRECISION NOT NULL CHECK (importance >= 0 AND importance <= 1),
 updated_at BIGINT NOT NULL,
 PRIMARY KEY(user_id,project_id,node_id),
 FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE,
 FOREIGN KEY(user_id,knowledge_id) REFERENCES codelocal_knowledge_objects(user_id,knowledge_id) ON DELETE CASCADE,
 FOREIGN KEY(user_id,revision_id) REFERENCES codelocal_knowledge_revisions(user_id,revision_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_knowledge_graph_nodes_project
 ON codelocal_knowledge_graph_nodes(user_id,project_id,updated_at DESC,node_id);

CREATE TABLE IF NOT EXISTS codelocal_knowledge_graph_edges (
 user_id TEXT NOT NULL,
 project_id TEXT NOT NULL,
 edge_id TEXT NOT NULL,
 from_node_id TEXT NOT NULL,
 to_node_id TEXT NOT NULL,
 relation TEXT NOT NULL CHECK (relation IN ('HAS_CANONICAL_KNOWLEDGE','SCOPES_KNOWLEDGE','HAS_REVISION','SUPERSEDED_BY')),
 confidence DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
 importance DOUBLE PRECISION NOT NULL CHECK (importance >= 0 AND importance <= 1),
 updated_at BIGINT NOT NULL,
 PRIMARY KEY(user_id,project_id,edge_id),
 FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_knowledge_graph_edges_project
 ON codelocal_knowledge_graph_edges(user_id,project_id,updated_at DESC,edge_id);
`

const canonicalKnowledgeGraphStateMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_knowledge_graph_projection_state (
 user_id TEXT NOT NULL,
 project_id TEXT NOT NULL,
 source_object_count INTEGER NOT NULL DEFAULT 0 CHECK (source_object_count >= 0),
 source_revision_count INTEGER NOT NULL DEFAULT 0 CHECK (source_revision_count >= 0),
 source_updated_at BIGINT NOT NULL DEFAULT 0,
 projected_at BIGINT NOT NULL,
 node_count INTEGER NOT NULL DEFAULT 0 CHECK (node_count >= 0),
 edge_count INTEGER NOT NULL DEFAULT 0 CHECK (edge_count >= 0),
 PRIMARY KEY(user_id,project_id),
 FOREIGN KEY(user_id,project_id) REFERENCES codelocal_projects(user_id,project_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_codelocal_knowledge_graph_projection_state_projected
 ON codelocal_knowledge_graph_projection_state(projected_at DESC);
`

const canonicalGraphProjectionStateUpsertSQL = `
INSERT INTO codelocal_knowledge_graph_projection_state(
 user_id,project_id,source_object_count,source_revision_count,source_updated_at,projected_at,node_count,edge_count)
VALUES($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT(user_id,project_id) DO UPDATE SET
 source_object_count=EXCLUDED.source_object_count,source_revision_count=EXCLUDED.source_revision_count,
 source_updated_at=EXCLUDED.source_updated_at,projected_at=EXCLUDED.projected_at,
 node_count=EXCLUDED.node_count,edge_count=EXCLUDED.edge_count`

const canonicalGraphFreshnessSQL = `
WITH source AS (
 SELECT p.user_id,p.project_id,
  COUNT(DISTINCT o.knowledge_id)::int AS object_count,
  COUNT(r.revision_id)::int AS revision_count,
  GREATEST(COALESCE(MAX(o.updated_at),0),COALESCE(MAX(r.created_at),0))::bigint AS source_updated_at
 FROM codelocal_projects p
 LEFT JOIN codelocal_knowledge_objects o
  ON o.user_id=p.user_id AND o.project_id=p.project_id
  AND o.privacy_classification='private_project'
  AND o.status IN ('active','stale','conflicted')
 LEFT JOIN codelocal_knowledge_revisions r
  ON r.user_id=o.user_id AND r.knowledge_id=o.knowledge_id
 WHERE ($1='' OR p.user_id=$1)
 GROUP BY p.user_id,p.project_id
)
SELECT source.user_id,source.project_id,source.object_count,source.revision_count,source.source_updated_at,
 COALESCE(state.source_object_count,-1),COALESCE(state.source_revision_count,-1),COALESCE(state.source_updated_at,-1),
 COALESCE(state.projected_at,0),COALESCE(state.node_count,0),COALESCE(state.edge_count,0)
FROM source
LEFT JOIN codelocal_knowledge_graph_projection_state state
 ON state.user_id=source.user_id AND state.project_id=source.project_id
ORDER BY source.project_id`

const canonicalGraphProjectionSelectSQL = `
SELECT
 o.knowledge_id,COALESCE(o.repository_id,''),COALESCE(o.branch,''),o.knowledge_type,o.stable_key,o.status,
 o.confidence,o.importance,o.updated_at,COALESCE(o.active_revision_id,''),
 r.revision_id,r.revision_number,r.summary,r.confidence,r.importance,r.created_at,COALESCE(r.valid_until,0)
FROM codelocal_knowledge_objects o
JOIN codelocal_knowledge_revisions r
 ON r.user_id=o.user_id AND r.knowledge_id=o.knowledge_id
WHERE o.user_id=$1 AND o.project_id=$2
 AND o.privacy_classification='private_project'
 AND o.status IN ('active','stale','conflicted')
ORDER BY o.updated_at DESC,o.knowledge_id ASC,r.revision_number ASC
LIMIT $3`

const canonicalGraphNodeUpsertSQL = `
INSERT INTO codelocal_knowledge_graph_nodes(
 user_id,project_id,node_id,knowledge_id,revision_id,repository_id,node_kind,label,summary,status,confidence,importance,updated_at)
VALUES($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),$7,$8,$9,$10,$11,$12,$13)
ON CONFLICT(user_id,project_id,node_id) DO UPDATE SET
 knowledge_id=EXCLUDED.knowledge_id,revision_id=EXCLUDED.revision_id,repository_id=EXCLUDED.repository_id,
 node_kind=EXCLUDED.node_kind,label=EXCLUDED.label,summary=EXCLUDED.summary,status=EXCLUDED.status,
 confidence=EXCLUDED.confidence,importance=EXCLUDED.importance,updated_at=EXCLUDED.updated_at`

const canonicalGraphEdgeUpsertSQL = `
INSERT INTO codelocal_knowledge_graph_edges(
 user_id,project_id,edge_id,from_node_id,to_node_id,relation,confidence,importance,updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT(user_id,project_id,edge_id) DO UPDATE SET
 from_node_id=EXCLUDED.from_node_id,to_node_id=EXCLUDED.to_node_id,relation=EXCLUDED.relation,
 confidence=EXCLUDED.confidence,importance=EXCLUDED.importance,updated_at=EXCLUDED.updated_at`

type canonicalGraphProjectionRow struct {
	KnowledgeID     string
	RepositoryID    string
	Branch          string
	KnowledgeType   string
	StableKey       string
	Status          string
	Confidence      float64
	Importance      float64
	ObjectUpdatedAt int64
	ActiveRevision  string
	RevisionID      string
	RevisionNumber  int
	Summary         string
	RevConfidence   float64
	RevImportance   float64
	RevisionCreated int64
	RevisionValidTo int64
}

type canonicalGraphNode struct {
	NodeID       string
	KnowledgeID  string
	RevisionID   string
	RepositoryID string
	Kind         string
	Label        string
	Summary      string
	Status       string
	Confidence   float64
	Importance   float64
	UpdatedAt    int64
}

type canonicalGraphEdge struct {
	EdgeID     string
	From       string
	To         string
	Relation   string
	Confidence float64
	Importance float64
	UpdatedAt  int64
}

type CanonicalGraphProjectionStats struct {
	KnowledgeCount  int   `json:"knowledgeCount"`
	RevisionCount   int   `json:"revisionCount"`
	NodeCount       int   `json:"nodeCount"`
	EdgeCount       int   `json:"edgeCount"`
	SourceUpdatedAt int64 `json:"sourceUpdatedAt"`
	ProjectedAt     int64 `json:"projectedAt"`
}

type CanonicalGraphFreshness struct {
	UserID                   string `json:"-"`
	ProjectID                string `json:"projectId"`
	Status                   string `json:"status"`
	SourceObjectCount        int    `json:"sourceObjectCount"`
	SourceRevisionCount      int    `json:"sourceRevisionCount"`
	ProjectedObjectCount     int    `json:"projectedObjectCount"`
	ProjectedRevisionCount   int    `json:"projectedRevisionCount"`
	SourceUpdatedAt          int64  `json:"sourceUpdatedAt"`
	ProjectedSourceUpdatedAt int64  `json:"projectedSourceUpdatedAt"`
	ProjectedAt              int64  `json:"projectedAt"`
	NodeCount                int    `json:"nodeCount"`
	EdgeCount                int    `json:"edgeCount"`
	LagMS                    int64  `json:"lagMs"`
}

type CanonicalGraphFreshnessSummary struct {
	Status          string `json:"status"`
	ProjectCount    int    `json:"projectCount"`
	CurrentProjects int    `json:"currentProjects"`
	StaleProjects   int    `json:"staleProjects"`
	MissingProjects int    `json:"missingProjects"`
	EmptyProjects   int    `json:"emptyProjects"`
	MaxLagMS        int64  `json:"maxLagMs"`
}

func canonicalGraphProjectionLimit() int {
	limit := envInt("CODELOCAL_KNOWLEDGE_GRAPH_MAX_REVISIONS_PER_PROJECT", 2000)
	if limit < 100 {
		limit = 100
	}
	if limit > 10000 {
		limit = 10000
	}
	return limit
}

func canonicalGraphSafeLabel(kind, stableKey string) string {
	kind = strings.TrimSpace(kind)
	raw := strings.TrimSpace(stableKey)
	if raw == "" || healthSecretLikeKey(raw) || healthStringContainsSecret(raw) {
		return kind
	}
	stableKey = longmemory.SanitizeText(raw, 160)
	if stableKey == "" {
		return kind
	}
	return kind + ": " + stableKey
}

func canonicalGraphSafeSummary(summary string) string {
	raw := strings.TrimSpace(summary)
	if raw == "" {
		return ""
	}
	if healthStringContainsSecret(raw) {
		return "[redacted by knowledge health policy]"
	}
	return longmemory.SanitizeText(raw, 800)
}

func buildCanonicalGraphProjection(projectID string, rows []canonicalGraphProjectionRow) ([]canonicalGraphNode, []canonicalGraphEdge, CanonicalGraphProjectionStats) {
	nodes := map[string]canonicalGraphNode{}
	edges := map[string]canonicalGraphEdge{}
	knowledgeSeen := map[string]struct{}{}
	revisionSeen := map[string]struct{}{}
	lastRevisionNode := map[string]string{}
	sourceUpdatedAt := int64(0)
	for _, row := range rows {
		if row.ObjectUpdatedAt > sourceUpdatedAt {
			sourceUpdatedAt = row.ObjectUpdatedAt
		}
		if row.RevisionCreated > sourceUpdatedAt {
			sourceUpdatedAt = row.RevisionCreated
		}
		if strings.TrimSpace(row.KnowledgeID) == "" || strings.TrimSpace(row.RevisionID) == "" {
			continue
		}
		knowledgeNodeID := "canonical-knowledge:" + row.KnowledgeID
		if _, exists := nodes[knowledgeNodeID]; !exists {
			nodes[knowledgeNodeID] = canonicalGraphNode{
				NodeID: knowledgeNodeID, KnowledgeID: row.KnowledgeID, RepositoryID: row.RepositoryID,
				Kind: "knowledge", Label: canonicalGraphSafeLabel(row.KnowledgeType, row.StableKey),
				Status: row.Status, Confidence: row.Confidence, Importance: row.Importance, UpdatedAt: row.ObjectUpdatedAt,
			}
			knowledgeSeen[row.KnowledgeID] = struct{}{}
			edges["project-canonical:"+row.KnowledgeID] = canonicalGraphEdge{
				EdgeID: "project-canonical:" + row.KnowledgeID, From: "project:" + projectID, To: knowledgeNodeID,
				Relation: "HAS_CANONICAL_KNOWLEDGE", Confidence: row.Confidence, Importance: row.Importance, UpdatedAt: row.ObjectUpdatedAt,
			}
			if row.RepositoryID != "" {
				edges["repo-canonical:"+row.RepositoryID+":"+row.KnowledgeID] = canonicalGraphEdge{
					EdgeID: "repo-canonical:" + row.RepositoryID + ":" + row.KnowledgeID, From: "repo:" + row.RepositoryID, To: knowledgeNodeID,
					Relation: "SCOPES_KNOWLEDGE", Confidence: row.Confidence, Importance: row.Importance, UpdatedAt: row.ObjectUpdatedAt,
				}
			}
		}
		revisionNodeID := "canonical-revision:" + row.RevisionID
		revisionStatus := "historical"
		if row.RevisionID == row.ActiveRevision {
			revisionStatus = "active"
		}
		nodes[revisionNodeID] = canonicalGraphNode{
			NodeID: revisionNodeID, KnowledgeID: row.KnowledgeID, RevisionID: row.RevisionID, RepositoryID: row.RepositoryID,
			Kind: "revision", Label: fmt.Sprintf("revision %d", row.RevisionNumber), Summary: canonicalGraphSafeSummary(row.Summary),
			Status: revisionStatus, Confidence: row.RevConfidence, Importance: row.RevImportance, UpdatedAt: row.RevisionCreated,
		}
		revisionSeen[row.RevisionID] = struct{}{}
		edges["knowledge-revision:"+row.RevisionID] = canonicalGraphEdge{
			EdgeID: "knowledge-revision:" + row.RevisionID, From: knowledgeNodeID, To: revisionNodeID,
			Relation: "HAS_REVISION", Confidence: row.RevConfidence, Importance: row.RevImportance, UpdatedAt: row.RevisionCreated,
		}
		if previous := lastRevisionNode[row.KnowledgeID]; previous != "" {
			edges["revision-superseded:"+row.RevisionID] = canonicalGraphEdge{
				EdgeID: "revision-superseded:" + row.RevisionID, From: previous, To: revisionNodeID,
				Relation: "SUPERSEDED_BY", Confidence: 1, Importance: row.RevImportance, UpdatedAt: row.RevisionCreated,
			}
		}
		lastRevisionNode[row.KnowledgeID] = revisionNodeID
	}
	nodeList := make([]canonicalGraphNode, 0, len(nodes))
	for _, node := range nodes {
		nodeList = append(nodeList, node)
	}
	sort.Slice(nodeList, func(i, j int) bool { return nodeList[i].NodeID < nodeList[j].NodeID })
	edgeList := make([]canonicalGraphEdge, 0, len(edges))
	for _, edge := range edges {
		edgeList = append(edgeList, edge)
	}
	sort.Slice(edgeList, func(i, j int) bool { return edgeList[i].EdgeID < edgeList[j].EdgeID })
	return nodeList, edgeList, CanonicalGraphProjectionStats{
		KnowledgeCount: len(knowledgeSeen), RevisionCount: len(revisionSeen), NodeCount: len(nodeList), EdgeCount: len(edgeList),
		SourceUpdatedAt: sourceUpdatedAt,
	}
}

func (s *Store) RebuildCanonicalKnowledgeGraphProject(ctx context.Context, userID, projectID string) (CanonicalGraphProjectionStats, error) {
	if s == nil || s.DB == nil || strings.TrimSpace(userID) == "" || strings.TrimSpace(projectID) == "" {
		return CanonicalGraphProjectionStats{}, errors.New("canonical graph projection requires user and project")
	}
	limit := canonicalGraphProjectionLimit()
	rows, err := s.DB.Query(ctx, canonicalGraphProjectionSelectSQL, userID, projectID, limit+1)
	if err != nil {
		return CanonicalGraphProjectionStats{}, err
	}
	values := make([]canonicalGraphProjectionRow, 0, min(limit, 256))
	for rows.Next() {
		var row canonicalGraphProjectionRow
		if err := rows.Scan(
			&row.KnowledgeID, &row.RepositoryID, &row.Branch, &row.KnowledgeType, &row.StableKey, &row.Status,
			&row.Confidence, &row.Importance, &row.ObjectUpdatedAt, &row.ActiveRevision,
			&row.RevisionID, &row.RevisionNumber, &row.Summary, &row.RevConfidence, &row.RevImportance, &row.RevisionCreated, &row.RevisionValidTo,
		); err != nil {
			rows.Close()
			return CanonicalGraphProjectionStats{}, err
		}
		values = append(values, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return CanonicalGraphProjectionStats{}, err
	}
	rows.Close()
	if len(values) > limit {
		return CanonicalGraphProjectionStats{}, fmt.Errorf("canonical graph projection exceeds project revision cap: observed>%d", limit)
	}
	nodes, edges, stats := buildCanonicalGraphProjection(projectID, values)
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return CanonicalGraphProjectionStats{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM codelocal_knowledge_graph_edges WHERE user_id=$1 AND project_id=$2`, userID, projectID); err != nil {
		return CanonicalGraphProjectionStats{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM codelocal_knowledge_graph_nodes WHERE user_id=$1 AND project_id=$2`, userID, projectID); err != nil {
		return CanonicalGraphProjectionStats{}, err
	}
	batch := &pgx.Batch{}
	for _, node := range nodes {
		batch.Queue(canonicalGraphNodeUpsertSQL,
			userID, projectID, node.NodeID, node.KnowledgeID, node.RevisionID, node.RepositoryID, node.Kind,
			node.Label, node.Summary, node.Status, node.Confidence, node.Importance, node.UpdatedAt,
		)
	}
	for _, edge := range edges {
		batch.Queue(canonicalGraphEdgeUpsertSQL,
			userID, projectID, edge.EdgeID, edge.From, edge.To, edge.Relation, edge.Confidence, edge.Importance, edge.UpdatedAt,
		)
	}
	results := tx.SendBatch(ctx, batch)
	if err := results.Close(); err != nil {
		return CanonicalGraphProjectionStats{}, err
	}
	projectedAt := time.Now().UnixMilli()
	if _, err := tx.Exec(ctx, canonicalGraphProjectionStateUpsertSQL,
		userID, projectID, stats.KnowledgeCount, stats.RevisionCount, stats.SourceUpdatedAt,
		projectedAt, stats.NodeCount, stats.EdgeCount,
	); err != nil {
		return CanonicalGraphProjectionStats{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CanonicalGraphProjectionStats{}, err
	}
	stats.ProjectedAt = projectedAt
	return stats, nil
}

func canonicalGraphProjectionFreshness(updatedAt int64, now time.Time) time.Duration {
	if updatedAt <= 0 {
		return 0
	}
	age := now.Sub(time.UnixMilli(updatedAt))
	if age < 0 {
		return 0
	}
	return age
}

func classifyCanonicalGraphFreshness(value CanonicalGraphFreshness, now int64) CanonicalGraphFreshness {
	if value.SourceObjectCount == 0 && value.SourceRevisionCount == 0 {
		if value.ProjectedAt > 0 && (value.ProjectedObjectCount > 0 || value.ProjectedRevisionCount > 0 || value.NodeCount > 0 || value.EdgeCount > 0) {
			value.Status = "stale"
			if now > value.ProjectedAt {
				value.LagMS = now - value.ProjectedAt
			}
			return value
		}
		value.Status = "empty"
		value.LagMS = 0
		return value
	}
	if value.ProjectedAt <= 0 || value.ProjectedObjectCount < 0 || value.ProjectedRevisionCount < 0 {
		value.Status = "missing"
		if value.SourceUpdatedAt > 0 && now > value.SourceUpdatedAt {
			value.LagMS = now - value.SourceUpdatedAt
		}
		return value
	}
	if value.ProjectedObjectCount != value.SourceObjectCount ||
		value.ProjectedRevisionCount != value.SourceRevisionCount ||
		value.ProjectedSourceUpdatedAt != value.SourceUpdatedAt {
		value.Status = "stale"
		if value.SourceUpdatedAt > value.ProjectedSourceUpdatedAt {
			value.LagMS = value.SourceUpdatedAt - value.ProjectedSourceUpdatedAt
		}
		return value
	}
	value.Status = "current"
	value.LagMS = 0
	return value
}

func summarizeCanonicalGraphFreshness(values []CanonicalGraphFreshness) CanonicalGraphFreshnessSummary {
	summary := CanonicalGraphFreshnessSummary{Status: "current", ProjectCount: len(values)}
	for _, value := range values {
		switch value.Status {
		case "current":
			summary.CurrentProjects++
		case "empty":
			summary.EmptyProjects++
		case "missing":
			summary.MissingProjects++
			summary.Status = "degraded"
		case "stale":
			summary.StaleProjects++
			summary.Status = "degraded"
		default:
			summary.Status = "degraded"
		}
		if value.LagMS > summary.MaxLagMS {
			summary.MaxLagMS = value.LagMS
		}
	}
	return summary
}

func (s *Store) CanonicalGraphFreshness(ctx context.Context, userID string) ([]CanonicalGraphFreshness, CanonicalGraphFreshnessSummary, error) {
	if s == nil || s.DB == nil {
		return nil, CanonicalGraphFreshnessSummary{Status: "unavailable"}, errors.New("canonical graph freshness unavailable")
	}
	rows, err := s.DB.Query(ctx, canonicalGraphFreshnessSQL, strings.TrimSpace(userID))
	if err != nil {
		return nil, CanonicalGraphFreshnessSummary{Status: "unavailable"}, err
	}
	defer rows.Close()
	now := time.Now().UnixMilli()
	values := []CanonicalGraphFreshness{}
	for rows.Next() {
		var value CanonicalGraphFreshness
		if err := rows.Scan(
			&value.UserID, &value.ProjectID, &value.SourceObjectCount, &value.SourceRevisionCount, &value.SourceUpdatedAt,
			&value.ProjectedObjectCount, &value.ProjectedRevisionCount, &value.ProjectedSourceUpdatedAt,
			&value.ProjectedAt, &value.NodeCount, &value.EdgeCount,
		); err != nil {
			return nil, CanonicalGraphFreshnessSummary{Status: "unavailable"}, err
		}
		values = append(values, classifyCanonicalGraphFreshness(value, now))
	}
	if err := rows.Err(); err != nil {
		return nil, CanonicalGraphFreshnessSummary{Status: "unavailable"}, err
	}
	return values, summarizeCanonicalGraphFreshness(values), nil
}
