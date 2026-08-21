"use client";

import { useMemo, useState } from "react";
import type { KnowledgeGraphNode, KnowledgeGraphResource } from "@/lib/contracts/knowledge";
import { formatDashboardTime } from "../dashboard-format";
import styles from "../dashboard.module.css";

type GraphGroup = "center" | "project" | "structure" | "skill" | "knowledge" | "experience" | "memory";
type PositionedNode = KnowledgeGraphNode & { x: number; y: number; group: GraphGroup };

const goldenAngle = Math.PI * (3 - Math.sqrt(5));

function graphGroup(kind: string): GraphGroup {
  switch (kind) {
    case "user": return "center";
    case "project": return "project";
    case "repository":
    case "workspace":
    case "device": return "structure";
    case "skill": return "skill";
    case "knowledge_source":
    case "knowledge_revision":
    case "canonical_knowledge":
    case "conflict": return "knowledge";
    case "experience": return "experience";
    default: return "memory";
  }
}

function groupBand(group: GraphGroup) {
  switch (group) {
    case "center": return [0, 0] as const;
    case "project": return [55, 110] as const;
    case "structure": return [120, 185] as const;
    case "skill": return [185, 245] as const;
    case "knowledge": return [195, 270] as const;
    case "experience": return [220, 280] as const;
    case "memory": return [210, 285] as const;
  }
}

function groupOffset(group: GraphGroup) {
  switch (group) {
    case "center": return 0;
    case "project": return 0.1;
    case "structure": return 0.7;
    case "skill": return 1.35;
    case "knowledge": return 2.1;
    case "experience": return 2.8;
    case "memory": return 3.45;
  }
}

function nodeClass(group: GraphGroup) {
  switch (group) {
    case "center": return styles.graphNodeCenter;
    case "project": return styles.graphNodeProject;
    case "structure": return styles.graphNodeStructure;
    case "skill": return styles.graphNodeSkill;
    case "knowledge": return styles.graphNodeKnowledge;
    case "experience": return styles.graphNodeExperience;
    case "memory": return styles.graphNodeMemory;
  }
}

function layoutNodes(nodes: KnowledgeGraphNode[]): PositionedNode[] {
  const groupCounts = new Map<GraphGroup, number>();
  return nodes.map((node) => {
    const group = graphGroup(node.kind);
    const index = groupCounts.get(group) ?? 0;
    groupCounts.set(group, index + 1);
    if (group === "center") return { ...node, group, x: 500, y: 310 };
    const [minRadius, maxRadius] = groupBand(group);
    const variation = ((index * 37) % 101) / 100;
    const radius = minRadius + (maxRadius - minRadius) * variation;
    const angle = index * goldenAngle + groupOffset(group);
    return {
      ...node,
      group,
      x: 500 + Math.cos(angle) * radius * 1.56,
      y: 310 + Math.sin(angle) * radius,
    };
  });
}

function nodeMatches(node: KnowledgeGraphNode, query: string) {
  if (!query) return true;
  const haystack = `${node.name} ${node.kind} ${node.scope ?? ""} ${node.summary ?? ""}`.toLowerCase();
  return haystack.includes(query);
}

export function KnowledgeGraphView({ graph }: { graph: KnowledgeGraphResource }) {
  const [query, setQuery] = useState("");
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const normalizedQuery = query.trim().toLowerCase();
  const nodes = useMemo(() => layoutNodes(graph.nodes), [graph.nodes]);
  const byID = useMemo(() => new Map(nodes.map((node) => [node.id, node])), [nodes]);
  const selected = selectedID ? byID.get(selectedID) ?? null : null;
  const connected = useMemo(() => {
    const ids = new Set<string>();
    if (!selectedID) return ids;
    ids.add(selectedID);
    for (const edge of graph.edges) {
      if (edge.from === selectedID) ids.add(edge.to);
      if (edge.to === selectedID) ids.add(edge.from);
    }
    return ids;
  }, [graph.edges, selectedID]);

  return (
    <div className={styles.knowledgeGraphLayout}>
      <div className={styles.knowledgeGraphPanel}>
        <div className={styles.graphToolbar}>
          <label>
            <span className="sr-only">Search Knowledge Graph</span>
            <input
              className={styles.graphSearch}
              type="search"
              value={query}
              placeholder="Search projects, memories, skills…"
              onChange={(event) => setQuery(event.target.value)}
            />
          </label>
          <div className={styles.graphLegend} aria-label="Knowledge Graph legend">
            <span><i className={styles.graphNodeProject} />Project</span>
            <span><i className={styles.graphNodeStructure} />Structure</span>
            <span><i className={styles.graphNodeKnowledge} />Knowledge</span>
            <span><i className={styles.graphNodeSkill} />Skill</span>
            <span><i className={styles.graphNodeMemory} />Memory</span>
          </div>
        </div>

        <div className={styles.graphViewport}>
          <svg viewBox="0 0 1000 620" role="img" aria-label={`Knowledge Graph with ${graph.nodes.length} nodes and ${graph.edges.length} relationships`}>
            <g className={styles.graphEdges}>
              {graph.edges.map((edge) => {
                const from = byID.get(edge.from);
                const to = byID.get(edge.to);
                if (!from || !to) return null;
                const selectedEdge = selectedID !== null && (edge.from === selectedID || edge.to === selectedID);
                const queryEdge = !normalizedQuery || (nodeMatches(from, normalizedQuery) && nodeMatches(to, normalizedQuery));
                const opacity = selectedID ? (selectedEdge ? 0.78 : 0.035) : queryEdge ? 0.1 + edge.importance * 0.16 : 0.025;
                return <line key={edge.id} x1={from.x} y1={from.y} x2={to.x} y2={to.y} style={{ opacity }} />;
              })}
            </g>
            <g>
              {nodes.map((node) => {
                const matches = nodeMatches(node, normalizedQuery);
                const related = selectedID ? connected.has(node.id) : true;
                const active = node.id === selectedID;
                const opacity = matches && related ? 1 : selectedID && related ? 0.72 : 0.13;
                const radius = 4.2 + node.importance * 5.5 + (active ? 3 : 0);
                return (
                  <g
                    className={styles.graphNode}
                    data-active={active ? "true" : undefined}
                    key={node.id}
                    role="button"
                    tabIndex={0}
                    aria-label={`${node.kind}: ${node.name}`}
                    style={{ opacity }}
                    onClick={() => setSelectedID(active ? null : node.id)}
                    onKeyDown={(event) => {
                      if (event.key === "Enter" || event.key === " ") {
                        event.preventDefault();
                        setSelectedID(active ? null : node.id);
                      }
                    }}
                  >
                    <circle className={nodeClass(node.group)} cx={node.x} cy={node.y} r={radius} />
                    {(active || node.kind === "project") && (
                      <text x={node.x + radius + 7} y={node.y + 4}>{node.name}</text>
                    )}
                    <title>{node.name}</title>
                  </g>
                );
              })}
            </g>
          </svg>
          {graph.nodes.length === 0 && <div className={styles.graphEmpty}>No durable knowledge nodes are available yet.</div>}
        </div>
        <div className={styles.graphFooter}>
          <span>{graph.nodes.length} nodes · {graph.edges.length} relationships</span>
          <span>Static by default · select/search to highlight real relationships</span>
        </div>
      </div>

      <aside className={styles.graphInspector}>
        <span className={styles.eyebrow}>Knowledge inspector</span>
        {selected ? (
          <>
            <h3>{selected.name}</h3>
            <div className={styles.inspectorMeta}>
              <span>{selected.kind}</span>
              {selected.scope && <span>{selected.scope}</span>}
            </div>
            {selected.summary && <p>{selected.summary}</p>}
            <dl>
              <div><dt>Confidence</dt><dd>{Math.round(selected.confidence * 100)}%</dd></div>
              <div><dt>Importance</dt><dd>{Math.round(selected.importance * 100)}%</dd></div>
              <div><dt>Last seen</dt><dd>{formatDashboardTime(selected.lastSeenAt ?? 0)}</dd></div>
              <div><dt>Connections</dt><dd>{Math.max(0, connected.size - 1)}</dd></div>
            </dl>
          </>
        ) : (
          <p className={styles.inspectorEmpty}>Select a node</p>
        )}
      </aside>
    </div>
  );
}
