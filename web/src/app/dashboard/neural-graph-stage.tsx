"use client";

import { useEffect, useMemo, useRef } from "react";
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

type Point = { x: number; y: number };
type RuntimeNode = NeuralStageNode & { x: number; y: number; vx: number; vy: number };
type DragState =
  | { type: "pan"; x: number; y: number; moved: boolean }
  | { type: "node"; id: string; x: number; y: number; moved: boolean }
  | null;

function hash(value: string) {
  let h = 2166136261;
  for (let index = 0; index < value.length; index += 1) {
    h ^= value.charCodeAt(index);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

function createLayout(nodes: NeuralStageNode[]) {
  const runtime = nodes.map<RuntimeNode>((node, index) => {
    const seed = hash(node.id);
    const angle = ((seed % 10000) / 10000) * Math.PI * 2;
    let radius = 145 + (index % 7) * 28;
    if (node.primary) radius = 0;
    else if (["project", "module", "package"].includes(node.kind)) radius = 150;
    else if (["repository", "workspace", "file"].includes(node.kind)) radius = 235 + (index % 3) * 24;
    else if (node.group === "memory" || node.group === "knowledge") radius = 280 + (index % 4) * 18;
    return {
      ...node,
      x: Math.cos(angle) * radius,
      y: Math.sin(angle) * radius,
      vx: 0,
      vy: 0,
    };
  });
  const primary = runtime.find((node) => node.primary);
  if (primary) {
    primary.x = 0;
    primary.y = 0;
  }
  return runtime;
}

function nodeRadius(node: NeuralStageNode, selected: boolean) {
  const weight = Math.max(0, Math.min(1, node.weight ?? 0.55));
  if (node.primary) return selected ? 13 : 11;
  return 5.5 + weight * 4 + (selected ? 2.5 : 0);
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
  const wrapRef = useRef<HTMLDivElement | null>(null);
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const runtimeRef = useRef<RuntimeNode[]>(createLayout(nodes));
  const panRef = useRef<Point>({ x: 0, y: 0 });
  const scaleRef = useRef(1);
  const dragRef = useRef<DragState>(null);
  const rafRef = useRef<number | null>(null);
  const settleRef = useRef(0);
  const selectedRef = useRef(selectedId);
  const onSelectRef = useRef(onSelect);
  const drawRef = useRef<() => void>(() => {});
  const restartRef = useRef<(withSettling?: boolean) => void>(() => {});
  const edgeMap = useMemo(() => edges, [edges]);

  useEffect(() => {
    runtimeRef.current = createLayout(nodes);
    panRef.current = { x: 0, y: 0 };
    scaleRef.current = 1;
    settleRef.current = 0;
  }, [nodes]);

  useEffect(() => {
    const canvas = canvasRef.current;
    const wrap = wrapRef.current;
    if (!canvas || !wrap) return;
    const context = canvas.getContext("2d");
    if (!context) return;

    const byId = () => new Map(runtimeRef.current.map((node) => [node.id, node]));
    const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    let disposed = false;
    let lastFrame = 0;

    const screen = (node: RuntimeNode, width: number, height: number) => ({
      x: width / 2 + panRef.current.x + node.x * scaleRef.current,
      y: height / 2 + panRef.current.y + node.y * scaleRef.current,
    });

    const world = (x: number, y: number) => {
      const rect = canvas.getBoundingClientRect();
      return {
        x: (x - rect.width / 2 - panRef.current.x) / scaleRef.current,
        y: (y - rect.height / 2 - panRef.current.y) / scaleRef.current,
      };
    };

    const draw = () => {
      const rect = canvas.getBoundingClientRect();
      const width = rect.width;
      const height = rect.height;
      context.clearRect(0, 0, width, height);
      const map = byId();
      const selected = selectedRef.current;
      const related = new Set<string>();
      if (selected) {
        related.add(selected);
        for (const edge of edgeMap) {
          if (edge.from === selected) related.add(edge.to);
          if (edge.to === selected) related.add(edge.from);
        }
      }

      for (const edge of edgeMap) {
        const fromNode = map.get(edge.from);
        const toNode = map.get(edge.to);
        if (!fromNode || !toNode || fromNode.matches === false || toNode.matches === false) continue;
        const from = screen(fromNode, width, height);
        const to = screen(toNode, width, height);
        const active = Boolean(selected && (edge.from === selected || edge.to === selected));
        const muted = Boolean(selected && !active);
        const strength = Math.max(0, Math.min(1, edge.strength ?? 0.5));

        context.save();
        context.beginPath();
        context.moveTo(from.x, from.y);
        context.lineTo(to.x, to.y);
        context.strokeStyle = active
          ? "rgba(103,157,255,.78)"
          : edge.weak
            ? "rgba(112,102,155,.16)"
            : "rgba(76,112,171,.24)";
        context.globalAlpha = muted ? 0.12 : 1;
        context.lineWidth = active ? 1.7 : 0.75 + strength * 0.65;
        if (edge.weak) context.setLineDash([4, 6]);
        context.stroke();
        context.restore();
      }

      for (const node of runtimeRef.current) {
        if (node.matches === false) continue;
        const point = screen(node, width, height);
        const active = node.id === selected;
        const isRelated = !selected || related.has(node.id);
        const radius = nodeRadius(node, active);
        const showLabel = active || node.primary || node.alwaysLabel || (scaleRef.current > 1.25 && isRelated);

        context.save();
        context.globalAlpha = isRelated ? 1 : 0.2;
        context.fillStyle = node.color;
        if (active && !reducedMotion) {
          context.shadowColor = node.color;
          context.shadowBlur = 14;
        }
        context.beginPath();
        context.arc(point.x, point.y, radius, 0, Math.PI * 2);
        context.fill();
        context.shadowBlur = 0;
        context.strokeStyle = active ? "rgba(235,244,255,.92)" : "rgba(190,207,230,.34)";
        context.lineWidth = active ? 1.5 : 0.65;
        context.stroke();

        if (showLabel) {
          context.font = "600 10px -apple-system,BlinkMacSystemFont,Segoe UI,sans-serif";
          context.fillStyle = isRelated ? "rgba(214,226,242,.9)" : "rgba(150,166,188,.35)";
          context.textAlign = "center";
          context.textBaseline = "top";
          const label = node.label.length > 32 ? `${node.label.slice(0, 31)}…` : node.label;
          context.fillText(label, point.x, point.y + radius + 7);
        }
        context.restore();
      }
    };

    const settle = () => {
      if (settleRef.current >= 55 || dragRef.current?.type === "node") return false;
      settleRef.current += 1;
      const map = byId();
      const visible = runtimeRef.current.filter((node) => node.matches !== false);

      for (const edge of edgeMap) {
        const a = map.get(edge.from);
        const b = map.get(edge.to);
        if (!a || !b || a.matches === false || b.matches === false) continue;
        let dx = b.x - a.x;
        let dy = b.y - a.y;
        const distance = Math.hypot(dx, dy) || 1;
        const target = edge.weak ? 132 : 116;
        const force = (distance - target) * 0.00145;
        dx /= distance;
        dy /= distance;
        a.vx += dx * force;
        a.vy += dy * force;
        b.vx -= dx * force;
        b.vy -= dy * force;
      }

      for (let index = 0; index < visible.length; index += 1) {
        const limit = Math.min(visible.length, index + 48);
        for (let otherIndex = index + 1; otherIndex < limit; otherIndex += 1) {
          const a = visible[index];
          const b = visible[otherIndex];
          const dx = b.x - a.x;
          const dy = b.y - a.y;
          const distanceSq = dx * dx + dy * dy + 120;
          if (distanceSq > 70000) continue;
          const force = Math.min(0.1, 24 / distanceSq);
          a.vx -= dx * force;
          a.vy -= dy * force;
          b.vx += dx * force;
          b.vy += dy * force;
        }
      }

      for (const node of visible) {
        if (node.primary) continue;
        node.vx += -node.x * 0.000024;
        node.vy += -node.y * 0.000024;
        node.vx *= 0.87;
        node.vy *= 0.87;
        node.x += node.vx;
        node.y += node.vy;
      }
      return true;
    };

    const frame = (now: number) => {
      if (disposed) return;
      if (now - lastFrame >= 32) {
        const moving = reducedMotion ? false : settle();
        draw();
        lastFrame = now;
        if (!moving) {
          rafRef.current = null;
          return;
        }
      }
      rafRef.current = requestAnimationFrame(frame);
    };

    const requestDraw = (withSettling = false) => {
      if (withSettling) settleRef.current = 0;
      draw();
      if (!reducedMotion && settleRef.current < 55 && rafRef.current === null) {
        rafRef.current = requestAnimationFrame(frame);
      }
    };
    drawRef.current = draw;
    restartRef.current = requestDraw;

    const resize = () => {
      const rect = wrap.getBoundingClientRect();
      const dpr = Math.min(2, window.devicePixelRatio || 1);
      const width = Math.max(1, Math.floor(rect.width));
      const height = Math.max(1, Math.floor(rect.height));
      canvas.width = Math.floor(width * dpr);
      canvas.height = Math.floor(height * dpr);
      canvas.style.width = `${width}px`;
      canvas.style.height = `${height}px`;
      context.setTransform(dpr, 0, 0, dpr, 0, 0);
      requestDraw(true);
    };

    const hit = (x: number, y: number) => {
      const rect = canvas.getBoundingClientRect();
      let best: RuntimeNode | null = null;
      let bestDistance = 21;
      for (const node of runtimeRef.current) {
        if (node.matches === false) continue;
        const point = screen(node, rect.width, rect.height);
        const distance = Math.hypot(point.x - x, point.y - y);
        if (distance < bestDistance) {
          best = node;
          bestDistance = distance;
        }
      }
      return best;
    };

    const pointerPosition = (event: PointerEvent) => {
      const rect = canvas.getBoundingClientRect();
      return { x: event.clientX - rect.left, y: event.clientY - rect.top };
    };

    const pointerDown = (event: PointerEvent) => {
      if (event.button !== 0) return;
      const point = pointerPosition(event);
      const node = hit(point.x, point.y);
      dragRef.current = node
        ? { type: "node", id: node.id, x: point.x, y: point.y, moved: false }
        : { type: "pan", x: point.x, y: point.y, moved: false };
      canvas.setPointerCapture?.(event.pointerId);
      canvas.classList.add(styles.dragging);
    };

    const pointerMove = (event: PointerEvent) => {
      const drag = dragRef.current;
      if (!drag) return;
      const point = pointerPosition(event);
      const dx = point.x - drag.x;
      const dy = point.y - drag.y;
      const moved = drag.moved || Math.abs(dx) + Math.abs(dy) > 2;

      if (drag.type === "pan") {
        panRef.current.x += dx;
        panRef.current.y += dy;
      } else {
        const node = runtimeRef.current.find((item) => item.id === drag.id);
        if (node) {
          const next = world(point.x, point.y);
          node.x = next.x;
          node.y = next.y;
          node.vx = 0;
          node.vy = 0;
        }
      }
      dragRef.current = { ...drag, x: point.x, y: point.y, moved };
      draw();
    };

    const pointerEnd = (event: PointerEvent) => {
      const drag = dragRef.current;
      dragRef.current = null;
      canvas.classList.remove(styles.dragging);
      if (drag?.type === "node" && !drag.moved) {
        onSelectRef.current?.(selectedRef.current === drag.id ? null : drag.id);
      }
      canvas.releasePointerCapture?.(event.pointerId);
    };

    const wheel = (event: WheelEvent) => {
      event.preventDefault();
      const rect = canvas.getBoundingClientRect();
      const x = event.clientX - rect.left;
      const y = event.clientY - rect.top;
      const before = world(x, y);
      const nextScale = Math.max(0.35, Math.min(3, scaleRef.current * Math.exp(-event.deltaY * 0.001)));
      scaleRef.current = nextScale;
      panRef.current.x = x - rect.width / 2 - before.x * nextScale;
      panRef.current.y = y - rect.height / 2 - before.y * nextScale;
      draw();
    };

    const observer = new ResizeObserver(resize);
    observer.observe(wrap);
    canvas.addEventListener("pointerdown", pointerDown);
    canvas.addEventListener("pointermove", pointerMove);
    canvas.addEventListener("pointerup", pointerEnd);
    canvas.addEventListener("pointercancel", pointerEnd);
    canvas.addEventListener("wheel", wheel, { passive: false });
    resize();

    return () => {
      disposed = true;
      observer.disconnect();
      canvas.removeEventListener("pointerdown", pointerDown);
      canvas.removeEventListener("pointermove", pointerMove);
      canvas.removeEventListener("pointerup", pointerEnd);
      canvas.removeEventListener("pointercancel", pointerEnd);
      canvas.removeEventListener("wheel", wheel);
      if (rafRef.current !== null) cancelAnimationFrame(rafRef.current);
      rafRef.current = null;
      drawRef.current = () => {};
      restartRef.current = () => {};
    };
  }, [edgeMap, nodes]);

  useEffect(() => {
    selectedRef.current = selectedId;
    drawRef.current();
  }, [selectedId]);

  useEffect(() => {
    onSelectRef.current = onSelect;
  }, [onSelect]);

  function resetView() {
    runtimeRef.current = createLayout(nodes);
    panRef.current = { x: 0, y: 0 };
    scaleRef.current = 1;
    settleRef.current = 0;
    restartRef.current(true);
  }

  function zoomBy(factor: number) {
    scaleRef.current = Math.max(0.35, Math.min(3, scaleRef.current * factor));
    drawRef.current();
  }

  return (
    <div ref={wrapRef} className={`${styles.stage} ${compact ? styles.compact : ""}`}>
      <canvas ref={canvasRef} className={styles.canvas} role="img" aria-label={ariaLabel} />

      {controls && nodes.length > 0 && (
        <div className={styles.controls} aria-label="Graph controls">
          <button type="button" title="Zoom in" aria-label="Zoom in" onClick={() => zoomBy(1.18)}>+</button>
          <button type="button" title="Zoom out" aria-label="Zoom out" onClick={() => zoomBy(1 / 1.18)}>−</button>
          <button type="button" title="Reset view" aria-label="Reset view" onClick={resetView}>⌾</button>
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
