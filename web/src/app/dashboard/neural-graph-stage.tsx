"use client";

import { useMemo, useRef, useState } from "react";
import type { CSSProperties, PointerEvent as ReactPointerEvent, WheelEvent } from "react";
import styles from "./neural-graph-stage.module.css";

export type NeuralStageNode = {
  id: string;
  kind: string;
  label: string;
  group: string;
  color: string;
  weight?: number;
  primary?: boolean;
  alwaysLabel?: boolean;
  matches?: boolean;
};

export type NeuralStageEdge = {
  id: string;
  from: string;
  to: string;
  relation?: string;
  strength?: number;
  weak?: boolean;
};

export type NeuralStageLegend = {
  label: string;
  color: string;
};

type Point = { x: number; y: number };
type DragState =
  | { type: "pan"; x: number; y: number; moved: boolean }
  | { type: "node"; id: string; x: number; y: number; moved: boolean }
  | null;

type Props = {
  nodes: NeuralStageNode[];
  edges: NeuralStageEdge[];
  selectedId?: string | null;
  onSelect?: (id: string | null) => void;
  legend?: NeuralStageLegend[];
  compact?: boolean;
  controls?: boolean;
  emptyLabel?: string;
  ariaLabel: string;
};

const WIDTH = 1000;
const HEIGHT = 620;
const CX = WIDTH / 2;
const CY = HEIGHT / 2;
const GOLDEN_ANGLE = Math.PI * (3 - Math.sqrt(5));

function hash(value: string) {
  let h = 2166136261;
  for (let index = 0; index < value.length; index += 1) {
    h ^= value.charCodeAt(index);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

function initialLayout(nodes: NeuralStageNode[]) {
  const result = new Map<string, Point>();
  const primary = nodes.find((node) => node.primary);
  if (primary) result.set(primary.id, { x: CX, y: CY });

  const groups = new Map<string, NeuralStageNode[]>();
  for (const node of nodes) {
    if (node.id === primary?.id) continue;
    const items = groups.get(node.group) ?? [];
    items.push(node);
    groups.set(node.group, items);
  }

  const groupEntries = [...groups.entries()];
  groupEntries.forEach(([, items], groupIndex) => {
    const baseRadius = 118 + (groupIndex % 4) * 58 + Math.floor(groupIndex / 4) * 18;
    items.forEach((node, index) => {
      const seed = hash(node.id);
      const radius = baseRadius + (seed % 48) - 24 + (index % 3) * 14;
      const angle = index * GOLDEN_ANGLE + groupIndex * 0.78 + ((seed % 100) / 100) * 0.36;
      result.set(node.id, {
        x: CX + Math.cos(angle) * radius * 1.42,
        y: CY + Math.sin(angle) * radius,
      });
    });
  });

  return result;
}

function curvePath(from: Point, to: Point, id: string) {
  const dx = to.x - from.x;
  const dy = to.y - from.y;
  const length = Math.max(1, Math.hypot(dx, dy));
  const bend = ((hash(id) % 2 === 0 ? 1 : -1) * Math.min(58, length * 0.12));
  const mx = (from.x + to.x) / 2 - (dy / length) * bend;
  const my = (from.y + to.y) / 2 + (dx / length) * bend;
  return `M ${from.x} ${from.y} Q ${mx} ${my} ${to.x} ${to.y}`;
}

function nodeRadius(node: NeuralStageNode, selected: boolean) {
  const weight = Math.max(0, Math.min(1, node.weight ?? 0.55));
  if (node.primary) return selected ? 27 : 24;
  return 14 + weight * 7 + (selected ? 5 : 0);
}

function Icon({ kind }: { kind: string }) {
  const normalized = kind.toLowerCase();
  if (normalized === "brain") {
    return <><circle cx="0" cy="0" r="3" /><path d="M0-3v-6M0 3v6M-3 0h-6M3 0h6M-6-6l4 4M6-6 2-2M-6 6l4-4M6 6 2 2" /></>;
  }
  if (["project", "workspace", "repository"].includes(normalized)) {
    return <path d="M-8-5h6l2 2h8v9H-8z" />;
  }
  if (["file", "knowledge_source", "knowledge_revision", "canonical_knowledge"].includes(normalized)) {
    return <path d="M-6-8h8l5 5v11H-6zM2-8v5h5" />;
  }
  if (["module", "package"].includes(normalized)) {
    return <path d="M0-9 8-5v10L0 9-8 5V-5zM-8-5 0 0l8-5M0 0v9" />;
  }
  if (["skill", "symbol", "function", "method", "class", "callsite"].includes(normalized)) {
    return <path d="m-3-7-6 7 6 7M3-7l6 7-6 7" />;
  }
  if (normalized === "device") {
    return <path d="M-9-7H9V5H-9zM-4 9h8M0 5v4" />;
  }
  if (["user", "memory", "experience", "decision", "event"].includes(normalized)) {
    return <path d="M0-8a4 4 0 1 1 0 8 4 4 0 0 1 0-8ZM-8 8c1-5 4-7 8-7s7 2 8 7" />;
  }
  return <path d="M0-8 8 0 0 8-8 0z" />;
}

export function NeuralGraphStage({
  nodes,
  edges,
  selectedId = null,
  onSelect,
  legend = [],
  compact = false,
  controls = true,
  emptyLabel = "No graph data",
  ariaLabel,
}: Props) {
  const svgRef = useRef<SVGSVGElement | null>(null);
  const dragRef = useRef<DragState>(null);
  const basePositions = useMemo(() => initialLayout(nodes), [nodes]);
  const [positionOverrides, setPositionOverrides] = useState<Map<string, Point>>(() => new Map());
  const [zoom, setZoom] = useState(1);
  const [pan, setPan] = useState<Point>({ x: 0, y: 0 });
  const positions = useMemo(() => {
    const resolved = new Map(basePositions);
    for (const [id, point] of positionOverrides) resolved.set(id, point);
    return resolved;
  }, [basePositions, positionOverrides]);

  const byId = useMemo(() => new Map(nodes.map((node) => [node.id, node])), [nodes]);
  const related = useMemo(() => {
    const ids = new Set<string>();
    if (!selectedId) return ids;
    ids.add(selectedId);
    for (const edge of edges) {
      if (edge.from === selectedId) ids.add(edge.to);
      if (edge.to === selectedId) ids.add(edge.from);
    }
    return ids;
  }, [edges, selectedId]);

  function pointerPoint(event: ReactPointerEvent<SVGSVGElement>): Point {
    const rect = svgRef.current?.getBoundingClientRect();
    if (!rect) return { x: 0, y: 0 };
    return {
      x: ((event.clientX - rect.left) / rect.width) * WIDTH,
      y: ((event.clientY - rect.top) / rect.height) * HEIGHT,
    };
  }

  function worldPoint(point: Point): Point {
    return { x: (point.x - pan.x) / zoom, y: (point.y - pan.y) / zoom };
  }

  function beginPan(event: ReactPointerEvent<SVGSVGElement>) {
    if (event.button !== 0) return;
    const point = pointerPoint(event);
    dragRef.current = { type: "pan", x: point.x, y: point.y, moved: false };
    event.currentTarget.setPointerCapture(event.pointerId);
  }

  function beginNodeDrag(event: ReactPointerEvent<SVGGElement>, id: string) {
    if (event.button !== 0) return;
    event.stopPropagation();
    const rect = svgRef.current?.getBoundingClientRect();
    if (!rect) return;
    const point = {
      x: ((event.clientX - rect.left) / rect.width) * WIDTH,
      y: ((event.clientY - rect.top) / rect.height) * HEIGHT,
    };
    dragRef.current = { type: "node", id, x: point.x, y: point.y, moved: false };
    svgRef.current?.setPointerCapture(event.pointerId);
  }

  function movePointer(event: ReactPointerEvent<SVGSVGElement>) {
    const drag = dragRef.current;
    if (!drag) return;
    const point = pointerPoint(event);
    const dx = point.x - drag.x;
    const dy = point.y - drag.y;
    const moved = drag.moved || Math.abs(dx) + Math.abs(dy) > 2;

    if (drag.type === "pan") {
      setPan((current) => ({ x: current.x + dx, y: current.y + dy }));
      dragRef.current = { ...drag, x: point.x, y: point.y, moved };
      return;
    }

    const world = worldPoint(point);
    setPositionOverrides((current) => {
      const next = new Map(current);
      next.set(drag.id, world);
      return next;
    });
    dragRef.current = { ...drag, x: point.x, y: point.y, moved };
  }

  function endPointer() {
    const drag = dragRef.current;
    dragRef.current = null;
    if (drag?.type === "node" && !drag.moved) {
      onSelect?.(selectedId === drag.id ? null : drag.id);
    }
  }

  function handleWheel(event: WheelEvent<SVGSVGElement>) {
    event.preventDefault();
    const next = Math.max(0.55, Math.min(2.25, zoom * Math.exp(-event.deltaY * 0.001)));
    setZoom(next);
  }

  function resetView() {
    setPositionOverrides(new Map());
    setZoom(1);
    setPan({ x: 0, y: 0 });
  }

  return (
    <div className={`${styles.stage} ${compact ? styles.compact : ""}`}>
      <svg
        ref={svgRef}
        className={styles.canvas}
        viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
        role="img"
        aria-label={ariaLabel}
        onPointerDown={beginPan}
        onPointerMove={movePointer}
        onPointerUp={endPointer}
        onPointerCancel={endPointer}
        onWheel={handleWheel}
      >
        <defs>
          <filter id="neural-node-glow" x="-100%" y="-100%" width="300%" height="300%">
            <feGaussianBlur stdDeviation="5" result="blur" />
            <feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
          </filter>
          <linearGradient id="neural-edge-gradient" x1="0" y1="0" x2="1" y2="1">
            <stop offset="0" stopColor="#42dcff" />
            <stop offset=".52" stopColor="#7f70ff" />
            <stop offset="1" stopColor="#ff62d9" />
          </linearGradient>
        </defs>
        <g transform={`translate(${pan.x} ${pan.y}) scale(${zoom})`}>
          <g className={styles.edges}>
            {edges.map((edge) => {
              const from = positions.get(edge.from);
              const to = positions.get(edge.to);
              const fromNode = byId.get(edge.from);
              const toNode = byId.get(edge.to);
              if (!from || !to || !fromNode || !toNode) return null;
              const active = Boolean(selectedId && (edge.from === selectedId || edge.to === selectedId));
              const muted = Boolean(selectedId && !active);
              const queryMuted = fromNode.matches === false || toNode.matches === false;
              return (
                <path
                  key={edge.id}
                  d={curvePath(from, to, edge.id)}
                  data-active={active ? "true" : undefined}
                  data-muted={muted ? "true" : undefined}
                  data-weak={edge.weak ? "true" : undefined}
                  style={{ opacity: selectedId ? (active ? 0.9 : 0.055) : queryMuted ? 0.025 : 0.2 + Math.min(1, edge.strength ?? 0.5) * 0.34 }}
                >
                  <title>{edge.relation ?? "Relationship"}</title>
                </path>
              );
            })}
          </g>
          <g className={styles.nodes}>
            {nodes.map((node) => {
              const point = positions.get(node.id);
              if (!point) return null;
              const selected = node.id === selectedId;
              const connected = !selectedId || related.has(node.id);
              const matches = node.matches !== false;
              const opacity = connected && matches ? 1 : selectedId && connected ? 0.64 : 0.13;
              const radius = nodeRadius(node, selected);
              const showLabel = selected || node.primary || node.alwaysLabel || (zoom > 1.25 && connected);
              return (
                <g
                  className={styles.node}
                  data-selected={selected ? "true" : undefined}
                  data-primary={node.primary ? "true" : undefined}
                  key={node.id}
                  role="button"
                  tabIndex={0}
                  aria-label={`${node.kind}: ${node.label}`}
                  style={{ opacity, "--node-color": node.color } as CSSProperties}
                  transform={`translate(${point.x} ${point.y})`}
                  onPointerDown={(event) => beginNodeDrag(event, node.id)}
                  onKeyDown={(event) => {
                    if (event.key === "Enter" || event.key === " ") {
                      event.preventDefault();
                      onSelect?.(selected ? null : node.id);
                    }
                  }}
                >
                  <circle className={styles.nodeHalo} r={radius + 9} />
                  <circle className={styles.nodeBody} r={radius} />
                  <g className={styles.nodeIcon} transform={`scale(${Math.max(0.72, Math.min(1.1, radius / 20))})`}>
                    <Icon kind={node.kind} />
                  </g>
                  {showLabel && (
                    <text className={styles.nodeLabel} x={radius + 11} y={4} textAnchor="start">
                      {node.label.slice(0, 34)}
                    </text>
                  )}
                  <title>{node.label}</title>
                </g>
              );
            })}
          </g>
        </g>
      </svg>

      {controls && nodes.length > 0 && (
        <div className={styles.controls} aria-label="Graph controls">
          <button type="button" title="Zoom in" aria-label="Zoom in" onClick={() => setZoom((value) => Math.min(2.25, value * 1.18))}>+</button>
          <button type="button" title="Zoom out" aria-label="Zoom out" onClick={() => setZoom((value) => Math.max(0.55, value / 1.18))}>−</button>
          <button type="button" title="Fit graph" aria-label="Fit graph" onClick={resetView}>⌾</button>
        </div>
      )}

      {legend.length > 0 && (
        <div className={styles.legend} aria-label="Graph legend">
          {legend.map((item) => <span key={item.label}><i style={{ background: item.color }} />{item.label}</span>)}
        </div>
      )}

      {nodes.length === 0 && <div className={styles.empty}>{emptyLabel}</div>}
    </div>
  );
}
