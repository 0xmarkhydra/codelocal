"use client";

import { useMemo, useState } from "react";
import type { CSSProperties } from "react";
import type { CodeGraphResource } from "@/lib/contracts/code-graph";
import type { AppIconName } from "../app-icon";
import { AppIcon } from "../app-icon";
import { NeuralGraphStage, NeuralStageNode } from "../neural-graph-stage";
import viewStyles from "../graph-view.module.css";

type CodeGroup = "module" | "file" | "symbol" | "external";

const colors: Record<CodeGroup, string> = {
  module: "#72a4d2",
  file: "#69b2ac",
  symbol: "#8da3e8",
  external: "#a792c5",
};

function codeGroup(kind: string): CodeGroup {
  switch (kind) {
    case "module":
    case "package": return "module";
    case "file": return "file";
    case "external": return "external";
    default: return "symbol";
  }
}

function relationCount(nodeID: string, graph: CodeGraphResource) {
  return graph.edges.reduce((count, edge) => count + (edge.from === nodeID || edge.to === nodeID ? 1 : 0), 0);
}

function inspectorIconName(kind: string): AppIconName {
  const group = codeGroup(kind);
  if (group === "file") return "file";
  if (group === "module") return "module";
  if (group === "external") return "connection";
  return "code";
}

export function CodeGraphView({ graph }: { graph: CodeGraphResource }) {
  const [selectedID, setSelectedID] = useState<string | null>(graph.selectedId ?? null);
  const selected = graph.nodes.find((node) => node.id === selectedID) ?? null;
  const primaryID = graph.selectedId
    ?? graph.nodes.find((node) => codeGroup(node.kind) === "module")?.id
    ?? graph.nodes[0]?.id;

  const nodes = useMemo<NeuralStageNode[]>(() => graph.nodes.map((node) => {
    const group = codeGroup(node.kind);
    return {
      id: node.id,
      kind: node.kind,
      label: node.name,
      group,
      color: colors[group],
      weight: node.confidence,
      primary: node.id === primaryID,
      alwaysLabel: node.selected || group === "module",
    };
  }), [graph.nodes, primaryID]);

  const edges = useMemo(() => graph.edges.map((edge) => ({
    id: edge.id,
    from: edge.from,
    to: edge.to,
    relation: edge.relation,
    strength: edge.confidence,
    weak: edge.resolutionMode === "text" || edge.confidence < 0.7,
  })), [graph.edges]);

  return (
    <div className={viewStyles.shell} data-inspector={selected ? "true" : undefined}>
      <div className={viewStyles.main}>
        <div className={viewStyles.toolbar}>
          <span className={viewStyles.meta}>{graph.view === "files" ? "Files" : "Architecture"} · {graph.nodes.length}/{graph.edges.length}</span>
        </div>
        <NeuralGraphStage
          nodes={nodes}
          edges={edges}
          selectedId={selectedID}
          onSelect={setSelectedID}
          ariaLabel={`Code Graph with ${graph.nodes.length} nodes and ${graph.edges.length} relationships`}
          emptyLabel="No code relationships"
          legend={[
            { label: "Module", color: colors.module },
            { label: "File", color: colors.file },
            { label: "Symbol", color: colors.symbol },
            { label: "External", color: colors.external },
          ]}
        />
      </div>

      {selected && (
        <aside className={viewStyles.inspector} aria-label="Code node details">
          <div className={viewStyles.inspectorHead}>
            <div className={viewStyles.identity}>
              <span className={viewStyles.avatar} style={{ "--node-color": colors[codeGroup(selected.kind)] } as CSSProperties}><AppIcon name={inspectorIconName(selected.kind)} size={17} /></span>
              <div><small>{selected.kind}</small><h3>{selected.qualifiedName || selected.name}</h3></div>
            </div>
            <button className={viewStyles.close} type="button" aria-label="Close inspector" onClick={() => setSelectedID(null)}><AppIcon name="close" size={15} /></button>
          </div>
          <div className={viewStyles.chips}>
            {selected.resolutionMode && <span>{selected.resolutionMode}</span>}
            {selected.canonical && <span>canonical</span>}
            <span>{relationCount(selected.id, graph)} links</span>
          </div>
          {selected.summary && <p className={viewStyles.summary}>{selected.summary}</p>}
          <dl className={viewStyles.facts}>
            {selected.path && <div><dt>Path</dt><dd>{selected.path}</dd></div>}
            {selected.line ? <div><dt>Position</dt><dd>{selected.line}:{selected.column || 1}</dd></div> : null}
            <div><dt>Repository</dt><dd>{selected.repositoryPath || "."}</dd></div>
            <div><dt>Provider</dt><dd>{selected.provider || "structural"}</dd></div>
            <div><dt>Confidence</dt><dd>{Math.round(selected.confidence * 100)}%</dd></div>
          </dl>
        </aside>
      )}
    </div>
  );
}
