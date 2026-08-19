"use client";

import { useMemo, useState } from "react";
import type { CodeGraphNode, CodeGraphResource } from "@/lib/contracts/code-graph";
import styles from "../dashboard.module.css";

type CodeGroup = "module" | "file" | "symbol" | "external";
type PositionedCodeNode = CodeGraphNode & { x: number; y: number; group: CodeGroup };

const goldenAngle = Math.PI * (3 - Math.sqrt(5));

function codeGroup(kind: string): CodeGroup {
  switch (kind) {
    case "module":
    case "package": return "module";
    case "file": return "file";
    case "external": return "external";
    default: return "symbol";
  }
}

function codeNodeClass(group: CodeGroup) {
  switch (group) {
    case "module": return styles.graphNodeStructure;
    case "file": return styles.graphNodeMemory;
    case "external": return styles.graphNodeExperience;
    case "symbol": return styles.graphNodeProject;
  }
}

function codeBand(group: CodeGroup) {
  switch (group) {
    case "module": return [70, 155] as const;
    case "file": return [150, 235] as const;
    case "symbol": return [175, 285] as const;
    case "external": return [235, 305] as const;
  }
}

function codeOffset(group: CodeGroup) {
  switch (group) {
    case "module": return 0.15;
    case "file": return 1.05;
    case "symbol": return 2.05;
    case "external": return 3.15;
  }
}

function layoutCodeNodes(nodes: CodeGraphNode[]): PositionedCodeNode[] {
  const counts = new Map<CodeGroup, number>();
  return nodes.map((node) => {
    const group = codeGroup(node.kind);
    const index = counts.get(group) ?? 0;
    counts.set(group, index + 1);
    const [minRadius, maxRadius] = codeBand(group);
    const variation = ((index * 43) % 101) / 100;
    const radius = minRadius + (maxRadius - minRadius) * variation;
    const angle = index * goldenAngle + codeOffset(group);
    return {
      ...node,
      group,
      x: 500 + Math.cos(angle) * radius * 1.52,
      y: 310 + Math.sin(angle) * radius,
    };
  });
}

function relationCount(nodeID: string, graph: CodeGraphResource) {
  return graph.edges.reduce((count, edge) => count + (edge.from === nodeID || edge.to === nodeID ? 1 : 0), 0);
}

export function CodeGraphView({ graph }: { graph: CodeGraphResource }) {
  const [selectedID, setSelectedID] = useState<string | null>(graph.selectedId ?? null);
  const nodes = useMemo(() => layoutCodeNodes(graph.nodes), [graph.nodes]);
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
          <div>
            <span className={styles.eyebrow}>{graph.view === "files" ? "File view" : "Architecture view"}</span>
            <strong className={styles.graphToolbarTitle}>{graph.query ? `Query: ${graph.query}` : "Bounded runtime evidence"}</strong>
          </div>
          <div className={styles.graphLegend} aria-label="Code Graph legend">
            <span><i className={styles.graphNodeStructure} />Module</span>
            <span><i className={styles.graphNodeMemory} />File</span>
            <span><i className={styles.graphNodeProject} />Symbol</span>
            <span><i className={styles.graphNodeExperience} />External</span>
          </div>
        </div>

        <div className={styles.graphViewport}>
          <svg viewBox="0 0 1000 620" role="img" aria-label={`Code Graph with ${graph.nodes.length} nodes and ${graph.edges.length} relationships`}>
            <g className={styles.graphEdges}>
              {graph.edges.map((edge) => {
                const from = byID.get(edge.from);
                const to = byID.get(edge.to);
                if (!from || !to) return null;
                const active = selectedID !== null && (edge.from === selectedID || edge.to === selectedID);
                const opacity = selectedID ? (active ? 0.82 : 0.035) : 0.08 + edge.confidence * 0.22;
                const dashed = edge.resolutionMode === "text" || edge.confidence < 0.7;
                return (
                  <line
                    key={edge.id}
                    x1={from.x}
                    y1={from.y}
                    x2={to.x}
                    y2={to.y}
                    style={{ opacity, strokeDasharray: dashed ? "4 6" : undefined }}
                  >
                    <title>{edge.relation} · {Math.round(edge.confidence * 100)}% confidence</title>
                  </line>
                );
              })}
            </g>
            <g>
              {nodes.map((node) => {
                const active = node.id === selectedID;
                const related = selectedID ? connected.has(node.id) : true;
                const opacity = related ? 1 : 0.14;
                const radius = 4.5 + Math.max(0.1, node.confidence) * 4.2 + (active ? 3 : 0);
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
                    <circle className={codeNodeClass(node.group)} cx={node.x} cy={node.y} r={radius} />
                    {(active || node.selected || node.kind === "module") && (
                      <text x={node.x + radius + 7} y={node.y + 4}>{node.name}</text>
                    )}
                    <title>{node.qualifiedName || node.name}</title>
                  </g>
                );
              })}
            </g>
          </svg>
          {graph.nodes.length === 0 && <div className={styles.graphEmpty}>No code relationships are available in this bounded slice.</div>}
        </div>
        <div className={styles.graphFooter}>
          <span>{graph.nodes.length} nodes · {graph.edges.length} edges · depth {graph.depth}</span>
          <span>{graph.truncated ? "Safety bound reached" : "Bounded local-runtime result"} · no full graph persisted in Cloud</span>
        </div>
      </div>

      <aside className={styles.graphInspector}>
        <span className={styles.eyebrow}>Code inspector</span>
        {selected ? (
          <>
            <h3>{selected.qualifiedName || selected.name}</h3>
            <div className={styles.inspectorMeta}>
              <span>{selected.kind}</span>
              {selected.resolutionMode && <span>{selected.resolutionMode}</span>}
              {selected.canonical && <span>canonical</span>}
            </div>
            {selected.summary && <p>{selected.summary}</p>}
            <dl>
              <div><dt>Path</dt><dd>{selected.path || "—"}</dd></div>
              <div><dt>Position</dt><dd>{selected.line ? `${selected.line}:${selected.column || 1}` : "—"}</dd></div>
              <div><dt>Repository</dt><dd>{selected.repositoryPath || "."}</dd></div>
              <div><dt>Provider</dt><dd>{selected.provider || "structural"}</dd></div>
              <div><dt>Confidence</dt><dd>{Math.round(selected.confidence * 100)}%</dd></div>
              <div><dt>Connections</dt><dd>{relationCount(selected.id, graph)}</dd></div>
            </dl>
          </>
        ) : (
          <p className={styles.inspectorEmpty}>Select a node to inspect workspace-relative evidence and direct relationships.</p>
        )}
      </aside>
    </div>
  );
}
