"use client";

import { useMemo, useState } from "react";
import type { CSSProperties } from "react";
import type { KnowledgeGraphNode, KnowledgeGraphResource } from "@/lib/contracts/knowledge";
import { useTranslations } from "@/lib/i18n/provider";
import type { AppIconName } from "../app-icon";
import { AppIcon } from "../app-icon";
import { formatDashboardTime } from "../dashboard-format";
import { NeuralGraphStage, NeuralStageNode } from "../neural-graph-stage";
import viewStyles from "../graph-view.module.css";

type GraphGroup = "center" | "project" | "structure" | "skill" | "knowledge" | "experience" | "memory";

const colors: Record<GraphGroup, string> = {
  center: "#dce8f7",
  project: "#8da3e8",
  structure: "#72a4d2",
  skill: "#d0a06c",
  knowledge: "#69b2ac",
  experience: "#cf8f7c",
  memory: "#a792c5",
};

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

function nodeMatches(node: KnowledgeGraphNode, query: string) {
  if (!query) return true;
  return `${node.name} ${node.kind} ${node.scope ?? ""} ${node.summary ?? ""}`.toLowerCase().includes(query);
}

function inspectorIconName(kind: string): AppIconName {
  switch (kind) {
    case "project":
    case "repository":
    case "workspace": return "folder";
    case "device": return "device";
    case "skill": return "skill";
    case "knowledge_source":
    case "knowledge_revision":
    case "canonical_knowledge":
    case "conflict": return "database";
    case "experience": return "code";
    case "user": return "brain";
    default: return "memory";
  }
}

export function KnowledgeGraphView({ graph }: { graph: KnowledgeGraphResource }) {
  const { locale, t } = useTranslations();
  const percent = new Intl.NumberFormat(locale, { style: "percent", maximumFractionDigits: 0 });
  const [query, setQuery] = useState("");
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const normalizedQuery = query.trim().toLowerCase();
  const selected = graph.nodes.find((node) => node.id === selectedID) ?? null;

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

  const primaryID = graph.nodes.find((node) => node.kind === "user")?.id
    ?? graph.nodes.find((node) => node.kind === "project")?.id
    ?? graph.nodes[0]?.id;

  const nodes = useMemo<NeuralStageNode[]>(() => graph.nodes.map((node) => {
    const group = graphGroup(node.kind);
    return {
      id: node.id,
      kind: node.kind,
      label: node.name,
      group,
      color: colors[group],
      weight: node.importance,
      primary: node.id === primaryID,
      alwaysLabel: node.kind === "project" || node.kind === "skill",
      matches: nodeMatches(node, normalizedQuery),
    };
  }), [graph.nodes, normalizedQuery, primaryID]);

  const edges = useMemo(() => graph.edges.map((edge) => ({
    id: edge.id,
    from: edge.from,
    to: edge.to,
    relation: edge.relation,
    strength: Math.max(edge.confidence, edge.importance),
    weak: edge.confidence < 0.55,
  })), [graph.edges]);

  return (
    <div className={`${viewStyles.shell} ${viewStyles.viewportShell}`} data-inspector={selected ? "true" : undefined}>
      <div className={viewStyles.main}>
        <div className={viewStyles.toolbar}>
          <AppIcon className={viewStyles.searchIcon} name="search" size={14} />
          <label className="sr-only" htmlFor="knowledge-graph-search">{t("Search Brain")}</label>
          <input
            id="knowledge-graph-search"
            className={viewStyles.search}
            type="search"
            value={query}
            placeholder={t("Search brain…")}
            onChange={(event) => setQuery(event.target.value)}
          />
          <span className={viewStyles.meta}>{t("{count} nodes", { count: graph.nodes.length })} · {t("{count} relationships", { count: graph.edges.length })}</span>
        </div>
        <NeuralGraphStage
          nodes={nodes}
          edges={edges}
          selectedId={selectedID}
          onSelect={setSelectedID}
          ariaLabel={t("Brain graph with {nodes} nodes and {edges} relationships", { nodes: graph.nodes.length, edges: graph.edges.length })}
          emptyLabel={t("No knowledge yet")}
          legend={[
            { label: t("Project"), color: colors.project },
            { label: t("Knowledge"), color: colors.knowledge },
            { label: t("Skill"), color: colors.skill },
            { label: t("Memory"), color: colors.memory },
          ]}
        />
      </div>

      {selected && (
        <aside className={viewStyles.inspector} aria-label={t("Knowledge node details")}>
          <div className={viewStyles.inspectorHead}>
            <div className={viewStyles.identity}>
              <span className={viewStyles.avatar} style={{ "--node-color": colors[graphGroup(selected.kind)] } as CSSProperties}><AppIcon name={inspectorIconName(selected.kind)} size={17} /></span>
              <div><small>{selected.kind.replaceAll("_", " ")}</small><h3>{selected.name}</h3></div>
            </div>
            <button className={viewStyles.close} type="button" aria-label={t("Close inspector")} onClick={() => setSelectedID(null)}><AppIcon name="close" size={15} /></button>
          </div>
          <div className={viewStyles.chips}>
            {selected.scope && <span>{selected.scope}</span>}
            <span>{t("{count} links", { count: Math.max(0, connected.size - 1) })}</span>
          </div>
          {selected.summary && <p className={viewStyles.summary}>{selected.summary}</p>}
          <dl className={viewStyles.facts}>
            <div><dt>{t("Confidence")}</dt><dd>{percent.format(selected.confidence)}</dd></div>
            <div><dt>{t("Importance")}</dt><dd>{percent.format(selected.importance)}</dd></div>
            <div><dt>{t("Seen")}</dt><dd>{formatDashboardTime(selected.lastSeenAt ?? 0, locale)}</dd></div>
          </dl>
        </aside>
      )}
    </div>
  );
}
