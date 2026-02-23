"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import { Skeleton } from "@/components/ui/skeleton";
import { fetchAgents, fetchTickets } from "@/lib/api";
import type { Agent, Ticket } from "@/lib/api";

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

interface GraphNode {
  id: string;
  label: string;
  kind: "agent" | "ticket" | "external";
  status?: "open" | "closed";
  x: number;
  y: number;
  vx: number;
  vy: number;
  radius: number;
}

interface GraphEdge {
  source: string;
  target: string;
}

interface Camera {
  x: number;
  y: number;
  zoom: number;
}

/* ------------------------------------------------------------------ */
/*  Graph building                                                     */
/* ------------------------------------------------------------------ */

function buildGraph(agents: Agent[], tickets: Ticket[]) {
  const nodes = new Map<string, GraphNode>();
  const edges: GraphEdge[] = [];
  const edgeSet = new Set<string>();

  const addEdge = (a: string, b: string) => {
    const key = [a, b].sort().join("|||");
    if (edgeSet.has(key)) return;
    edgeSet.add(key);
    edges.push({ source: a, target: b });
  };

  // Agent nodes
  const agentIds = new Set(agents.map((a) => a.id));
  for (const a of agents) {
    nodes.set(a.id, {
      id: a.id,
      label: a.id,
      kind: "agent",
      x: 0,
      y: 0,
      vx: 0,
      vy: 0,
      radius: 32,
    });
  }

  // Ensure _external node exists
  const ensureActor = (id: string) => {
    if (nodes.has(id)) return;
    if (id === "_external") {
      nodes.set(id, {
        id,
        label: "_external",
        kind: "external",
        x: 0,
        y: 0,
        vx: 0,
        vy: 0,
        radius: 28,
      });
    } else if (!agentIds.has(id)) {
      // Unknown actor – treat like an external entity
      nodes.set(id, {
        id,
        label: id,
        kind: "external",
        x: 0,
        y: 0,
        vx: 0,
        vy: 0,
        radius: 28,
      });
    }
  };

  // Ticket nodes + edges
  for (const t of tickets) {
    const tid = `ticket:${t.id}`;
    nodes.set(tid, {
      id: tid,
      label: t.title || t.id,
      kind: "ticket",
      status: t.status,
      x: 0,
      y: 0,
      vx: 0,
      vy: 0,
      radius: 14,
    });

    // created_by
    if (t.created_by) {
      ensureActor(t.created_by);
      addEdge(tid, t.created_by);
    }

    // waiting_on
    for (const w of t.waiting_on ?? []) {
      ensureActor(w);
      addEdge(tid, w);
    }

    // message participants
    for (const m of t.messages ?? []) {
      if (m.from) {
        ensureActor(m.from);
        addEdge(tid, m.from);
      }
      for (const r of m.to ?? []) {
        ensureActor(r);
        addEdge(tid, r);
      }
    }
  }

  return { nodes: Array.from(nodes.values()), edges };
}

/* ------------------------------------------------------------------ */
/*  Force simulation helpers                                           */
/* ------------------------------------------------------------------ */

function initPositions(nodes: GraphNode[], w: number, h: number) {
  const cx = w / 2;
  const cy = h / 2;
  const r = Math.min(w, h) * 0.3;
  nodes.forEach((n, i) => {
    const angle = (2 * Math.PI * i) / nodes.length;
    n.x = cx + r * Math.cos(angle) + (Math.random() - 0.5) * 20;
    n.y = cy + r * Math.sin(angle) + (Math.random() - 0.5) * 20;
    n.vx = 0;
    n.vy = 0;
  });
}

function simulate(
  nodes: GraphNode[],
  edges: GraphEdge[],
  w: number,
  h: number,
) {
  const lookup = new Map<string, GraphNode>();
  for (const n of nodes) lookup.set(n.id, n);

  const REPULSION = 4000;
  const ATTRACTION = 0.005;
  const IDEAL_LEN = 140;
  const CENTER_PULL = 0.01;
  const DAMPING = 0.85;

  const cx = w / 2;
  const cy = h / 2;

  // Repulsion between all node pairs
  for (let i = 0; i < nodes.length; i++) {
    for (let j = i + 1; j < nodes.length; j++) {
      const a = nodes[i];
      const b = nodes[j];
      const dx = a.x - b.x;
      const dy = a.y - b.y;
      const dist = Math.sqrt(dx * dx + dy * dy) || 1;
      const force = REPULSION / (dist * dist);
      const fx = (dx / dist) * force;
      const fy = (dy / dist) * force;
      a.vx += fx;
      a.vy += fy;
      b.vx -= fx;
      b.vy -= fy;
    }
  }

  // Attraction along edges
  for (const e of edges) {
    const a = lookup.get(e.source);
    const b = lookup.get(e.target);
    if (!a || !b) continue;
    const dx = b.x - a.x;
    const dy = b.y - a.y;
    const dist = Math.sqrt(dx * dx + dy * dy) || 1;
    const displacement = dist - IDEAL_LEN;
    const force = ATTRACTION * displacement;
    const fx = (dx / dist) * force;
    const fy = (dy / dist) * force;
    a.vx += fx;
    a.vy += fy;
    b.vx -= fx;
    b.vy -= fy;
  }

  // Center gravity
  for (const n of nodes) {
    n.vx += (cx - n.x) * CENTER_PULL;
    n.vy += (cy - n.y) * CENTER_PULL;
  }

  // Apply velocities
  for (const n of nodes) {
    n.vx *= DAMPING;
    n.vy *= DAMPING;
    n.x += n.vx;
    n.y += n.vy;
  }
}

/* ------------------------------------------------------------------ */
/*  Drawing                                                            */
/* ------------------------------------------------------------------ */

const COLORS = {
  agent: "#6AEC01",
  agentStroke: "#55bd01",
  external: "#FFFFFF",
  externalStroke: "#cccccc",
  ticketOpen: "#211F2D",
  ticketOpenStroke: "#3a3750",
  ticketClosed: "#15131f",
  ticketClosedStroke: "#2a2839",
  edge: "rgba(148,163,184,0.35)",
  text: "#e2e8f0",
  textMuted: "#64748b",
  textDark: "#0f172a",
};

function draw(
  ctx: CanvasRenderingContext2D,
  nodes: GraphNode[],
  edges: GraphEdge[],
  w: number,
  h: number,
  cam: Camera,
  hoveredId: string | null,
  dragId: string | null,
) {
  const lookup = new Map<string, GraphNode>();
  for (const n of nodes) lookup.set(n.id, n);
  const dpr = window.devicePixelRatio || 1;

  ctx.clearRect(0, 0, w * dpr, h * dpr);
  ctx.save();
  ctx.scale(dpr, dpr);

  // Apply camera transform
  ctx.translate(cam.x, cam.y);
  ctx.scale(cam.zoom, cam.zoom);

  // Edges
  for (const e of edges) {
    const a = lookup.get(e.source);
    const b = lookup.get(e.target);
    if (!a || !b) continue;
    const isHighlighted = hoveredId && (a.id === hoveredId || b.id === hoveredId);
    ctx.beginPath();
    ctx.moveTo(a.x, a.y);
    ctx.lineTo(b.x, b.y);
    ctx.strokeStyle = isHighlighted ? "rgba(148,163,184,0.7)" : COLORS.edge;
    ctx.lineWidth = (isHighlighted ? 2 : 1) / cam.zoom;
    ctx.stroke();
  }

  // Nodes
  for (const n of nodes) {
    const isActive = n.id === hoveredId || n.id === dragId;
    let fill: string;
    let stroke: string;

    switch (n.kind) {
      case "agent":
        fill = COLORS.agent;
        stroke = COLORS.agentStroke;
        break;
      case "external":
        fill = COLORS.external;
        stroke = COLORS.externalStroke;
        break;
      default:
        fill = n.status === "open" ? COLORS.ticketOpen : COLORS.ticketClosed;
        stroke = n.status === "open" ? COLORS.ticketOpenStroke : COLORS.ticketClosedStroke;
        break;
    }

    // Shadow for agents
    if (n.kind === "agent") {
      ctx.shadowColor = "rgba(106,236,1,0.3)";
      ctx.shadowBlur = 12;
    }

    ctx.beginPath();
    ctx.arc(n.x, n.y, n.radius + (isActive ? 3 : 0), 0, Math.PI * 2);
    ctx.fillStyle = fill;
    ctx.fill();
    ctx.strokeStyle = stroke;
    ctx.lineWidth = (isActive ? 3 : 2) / cam.zoom;
    ctx.stroke();

    ctx.shadowColor = "transparent";
    ctx.shadowBlur = 0;

    // Label — always below the node
    const fontSize = n.kind === "ticket" ? 10 : 12;
    ctx.font = `600 ${fontSize}px ui-monospace, monospace`;
    ctx.textAlign = "center";
    ctx.textBaseline = "top";

    const displayLabel =
      n.kind === "ticket"
        ? n.label.length > 16
          ? n.label.slice(0, 15) + "\u2026"
          : n.label
        : n.label;

    if (n.kind === "ticket") {
      ctx.fillStyle = n.status === "closed" ? COLORS.textMuted : COLORS.text;
    } else {
      ctx.fillStyle = COLORS.text;
    }
    ctx.fillText(displayLabel, n.x, n.y + n.radius + 6);
  }

  ctx.restore();
}

/* ------------------------------------------------------------------ */
/*  Coordinate helpers                                                 */
/* ------------------------------------------------------------------ */

/** Convert screen (mouse) coords to world (graph) coords */
function screenToWorld(sx: number, sy: number, cam: Camera): { wx: number; wy: number } {
  return {
    wx: (sx - cam.x) / cam.zoom,
    wy: (sy - cam.y) / cam.zoom,
  };
}

/* ------------------------------------------------------------------ */
/*  Page component                                                     */
/* ------------------------------------------------------------------ */

export default function GraphPage() {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const nodesRef = useRef<GraphNode[]>([]);
  const edgesRef = useRef<GraphEdge[]>([]);
  const animRef = useRef<number>(0);
  const hoveredRef = useRef<string | null>(null);
  const dragRef = useRef<{ id: string; offsetX: number; offsetY: number } | null>(null);
  const panRef = useRef<{ startX: number; startY: number; camX: number; camY: number } | null>(null);
  const camRef = useRef<Camera>({ x: 0, y: 0, zoom: 1 });
  const sizeRef = useRef({ w: 800, h: 600 });

  const [loading, setLoading] = useState(true);
  const [, setTick] = useState(0); // force re-render for legend

  const resize = useCallback(() => {
    const canvas = canvasRef.current;
    const container = containerRef.current;
    if (!canvas || !container) return;
    const dpr = window.devicePixelRatio || 1;
    const w = container.clientWidth;
    const h = container.clientHeight;
    canvas.width = w * dpr;
    canvas.height = h * dpr;
    canvas.style.width = `${w}px`;
    canvas.style.height = `${h}px`;
    sizeRef.current = { w, h };
  }, []);

  // Load data
  useEffect(() => {
    async function load() {
      try {
        const [agents, tickets] = await Promise.all([
          fetchAgents(),
          fetchTickets({ limit: 200 }),
        ]);
        const { nodes, edges } = buildGraph(agents, tickets);
        nodesRef.current = nodes;
        edgesRef.current = edges;
        // Init positions after we know container size
        requestAnimationFrame(() => {
          resize();
          initPositions(nodesRef.current, sizeRef.current.w, sizeRef.current.h);
          setTick((t) => t + 1);
        });
      } catch {
        // auth redirect handled by api client
      } finally {
        setLoading(false);
      }
    }
    load();
  }, [resize]);

  // Animation loop
  useEffect(() => {
    if (loading) return;
    let frame = 0;

    const loop = () => {
      const { w, h } = sizeRef.current;
      const canvas = canvasRef.current;
      if (!canvas) return;
      const ctx = canvas.getContext("2d");
      if (!ctx) return;

      simulate(nodesRef.current, edgesRef.current, w, h);
      draw(ctx, nodesRef.current, edgesRef.current, w, h, camRef.current, hoveredRef.current, dragRef.current?.id ?? null);
      frame = requestAnimationFrame(loop);
    };

    frame = requestAnimationFrame(loop);
    animRef.current = frame;
    return () => cancelAnimationFrame(frame);
  }, [loading]);

  // Resize observer
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const ro = new ResizeObserver(() => resize());
    ro.observe(container);
    return () => ro.disconnect();
  }, [resize]);

  // Hit-test in world coordinates
  const findNode = useCallback((ex: number, ey: number) => {
    const canvas = canvasRef.current;
    if (!canvas) return null;
    const rect = canvas.getBoundingClientRect();
    const { wx, wy } = screenToWorld(ex - rect.left, ey - rect.top, camRef.current);
    for (let i = nodesRef.current.length - 1; i >= 0; i--) {
      const n = nodesRef.current[i];
      const dx = wx - n.x;
      const dy = wy - n.y;
      if (dx * dx + dy * dy <= (n.radius + 4) * (n.radius + 4)) return n;
    }
    return null;
  }, []);

  const onMouseMove = useCallback(
    (e: React.MouseEvent) => {
      const canvas = canvasRef.current;
      if (!canvas) return;
      const rect = canvas.getBoundingClientRect();

      // Node drag
      const d = dragRef.current;
      if (d) {
        const { wx, wy } = screenToWorld(e.clientX - rect.left, e.clientY - rect.top, camRef.current);
        const node = nodesRef.current.find((n) => n.id === d.id);
        if (node) {
          node.x = wx - d.offsetX;
          node.y = wy - d.offsetY;
          node.vx = 0;
          node.vy = 0;
        }
        return;
      }

      // Canvas pan
      const p = panRef.current;
      if (p) {
        camRef.current.x = p.camX + (e.clientX - p.startX);
        camRef.current.y = p.camY + (e.clientY - p.startY);
        return;
      }

      // Hover
      const n = findNode(e.clientX, e.clientY);
      hoveredRef.current = n?.id ?? null;
      canvas.style.cursor = n ? "grab" : "default";
    },
    [findNode],
  );

  const onMouseDown = useCallback(
    (e: React.MouseEvent) => {
      const n = findNode(e.clientX, e.clientY);
      const canvas = canvasRef.current;
      if (!canvas) return;
      const rect = canvas.getBoundingClientRect();

      if (n) {
        // Drag node
        const { wx, wy } = screenToWorld(e.clientX - rect.left, e.clientY - rect.top, camRef.current);
        dragRef.current = {
          id: n.id,
          offsetX: wx - n.x,
          offsetY: wy - n.y,
        };
        canvas.style.cursor = "grabbing";
      } else {
        // Start panning
        panRef.current = {
          startX: e.clientX,
          startY: e.clientY,
          camX: camRef.current.x,
          camY: camRef.current.y,
        };
        canvas.style.cursor = "move";
      }
    },
    [findNode],
  );

  const onMouseUp = useCallback(() => {
    dragRef.current = null;
    panRef.current = null;
    const canvas = canvasRef.current;
    if (canvas) canvas.style.cursor = "default";
  }, []);

  const onWheel = useCallback((e: React.WheelEvent) => {
    e.preventDefault();
    const canvas = canvasRef.current;
    if (!canvas) return;
    const rect = canvas.getBoundingClientRect();
    const mx = e.clientX - rect.left;
    const my = e.clientY - rect.top;

    const cam = camRef.current;
    const factor = e.deltaY < 0 ? 1.1 : 1 / 1.1;
    const newZoom = Math.min(5, Math.max(0.1, cam.zoom * factor));

    // Zoom centered on cursor
    cam.x = mx - (mx - cam.x) * (newZoom / cam.zoom);
    cam.y = my - (my - cam.y) * (newZoom / cam.zoom);
    cam.zoom = newZoom;
  }, []);

  if (loading) {
    return (
      <div className="flex flex-col gap-6">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-[calc(100vh-12rem)]" />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4" style={{ height: "calc(100vh - 3rem)" }}>
      <div className="flex items-center justify-between">
        <h2 className="text-2xl font-semibold tracking-tight">Graph</h2>
        <div className="flex items-center gap-4 text-xs text-muted-foreground">
          <span className="flex items-center gap-1.5">
            <span className="inline-block h-3 w-3 rounded-full" style={{ background: COLORS.agent }} />
            Agent
          </span>
          <span className="flex items-center gap-1.5">
            <span className="inline-block h-3 w-3 rounded-full border border-muted-foreground/40" style={{ background: COLORS.external }} />
            External
          </span>
          <span className="flex items-center gap-1.5">
            <span className="inline-block h-3 w-3 rounded-full" style={{ background: COLORS.ticketOpen }} />
            Open Ticket
          </span>
          <span className="flex items-center gap-1.5">
            <span className="inline-block h-3 w-3 rounded-full" style={{ background: COLORS.ticketClosed }} />
            Closed Ticket
          </span>
        </div>
      </div>
      <div ref={containerRef} className="relative flex-1 min-h-0 rounded-lg border bg-card overflow-hidden">
        <canvas
          ref={canvasRef}
          onMouseMove={onMouseMove}
          onMouseDown={onMouseDown}
          onMouseUp={onMouseUp}
          onMouseLeave={onMouseUp}
          onWheel={onWheel}
        />
      </div>
    </div>
  );
}
