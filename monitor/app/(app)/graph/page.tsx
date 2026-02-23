"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog";
import { TicketDetail } from "@/components/ticket-detail";
import { fetchAgents, fetchTickets, fetchLogs } from "@/lib/api";
import type { Agent, Ticket } from "@/lib/api";
import { POLL_INTERVAL } from "@/lib/config";

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

interface GraphNode {
  id: string;
  label: string;
  kind: "agent" | "ticket" | "external";
  status?: "open" | "awaiting_close" | "closed";
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

  // Always show _external node
  nodes.set("_external", {
    id: "_external",
    label: "External",
    kind: "external",
    x: 0,
    y: 0,
    vx: 0,
    vy: 0,
    radius: 28,
  });

  const ensureActor = (id: string) => {
    if (nodes.has(id)) return;
    if (!agentIds.has(id)) {
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

  // Parent–child links between tickets (visual only, not simulated)
  const parentLinks: GraphEdge[] = [];
  for (const t of tickets) {
    if (t.parent_ticket_id) {
      const child = `ticket:${t.id}`;
      const parent = `ticket:${t.parent_ticket_id}`;
      if (nodes.has(child) && nodes.has(parent)) {
        parentLinks.push({ source: child, target: parent });
      }
    }
  }

  return { nodes: Array.from(nodes.values()), edges, parentLinks };
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
  strength: number,
  pinAgents: boolean,
) {
  const lookup = new Map<string, GraphNode>();
  for (const n of nodes) lookup.set(n.id, n);

  // Build adjacency: direct neighbors per node
  const neighbors = new Map<string, Set<string>>();
  for (const n of nodes) neighbors.set(n.id, new Set());
  for (const e of edges) {
    neighbors.get(e.source)?.add(e.target);
    neighbors.get(e.target)?.add(e.source);
  }

  const REPULSION = 3000 * strength;
  const SIBLING_REPULSION = 1500 * strength;
  const ATTRACTION = 0.008;
  const IDEAL_LEN = 120 * strength;
  const DAMPING = 0.85;

  // Repulsion along edges (between directly connected nodes)
  for (const e of edges) {
    const a = lookup.get(e.source);
    const b = lookup.get(e.target);
    if (!a || !b) continue;
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

  // Sibling repulsion: nodes that share a common neighbor push apart
  const siblingPairs = new Set<string>();
  for (const n of nodes) {
    const nbs = neighbors.get(n.id);
    if (!nbs || nbs.size < 2) continue;
    const arr = Array.from(nbs);
    for (let i = 0; i < arr.length; i++) {
      for (let j = i + 1; j < arr.length; j++) {
        const key = arr[i] < arr[j] ? `${arr[i]}|||${arr[j]}` : `${arr[j]}|||${arr[i]}`;
        if (siblingPairs.has(key)) continue;
        siblingPairs.add(key);
        const a = lookup.get(arr[i]);
        const b = lookup.get(arr[j]);
        if (!a || !b) continue;
        const dx = a.x - b.x;
        const dy = a.y - b.y;
        const dist = Math.sqrt(dx * dx + dy * dy) || 1;
        const force = SIBLING_REPULSION / (dist * dist);
        const fx = (dx / dist) * force;
        const fy = (dy / dist) * force;
        a.vx += fx;
        a.vy += fy;
        b.vx -= fx;
        b.vy -= fy;
      }
    }
  }

  // Attraction along edges (spring toward ideal length)
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

  // Apply velocities
  for (const n of nodes) {
    if (pinAgents && n.kind !== "ticket") {
      n.vx = 0;
      n.vy = 0;
      continue;
    }
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
  agentBg: "#0a0a10",
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

/** Pulse map: node/edge id → { time, isError } */
interface Pulse {
  time: number;
  isError: boolean;
}
type PulseMap = Map<string, Pulse>;

const PULSE_DURATION = 800; // ms

function draw(
  ctx: CanvasRenderingContext2D,
  nodes: GraphNode[],
  edges: GraphEdge[],
  w: number,
  h: number,
  cam: Camera,
  hoveredId: string | null,
  dragId: string | null,
  logo: HTMLImageElement | null,
  parentLinks: GraphEdge[],
  showParentLinks: boolean,
  pulses: PulseMap,
  now: number,
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

    // Edge pulse: check if this edge has an active pulse
    const edgeKey = [a.id, b.id].sort().join("|||");
    const edgePulse = pulses.get(edgeKey);
    let edgeGlow = false;
    let edgeGlowAlpha = 0;
    let edgeGlowError = false;
    if (edgePulse !== undefined) {
      const elapsed = now - edgePulse.time;
      if (elapsed < PULSE_DURATION) {
        edgeGlow = true;
        edgeGlowAlpha = 0.8 * (1 - elapsed / PULSE_DURATION);
        edgeGlowError = edgePulse.isError;
      } else {
        pulses.delete(edgeKey);
      }
    }

    if (edgeGlow) {
      // Draw glow line behind
      ctx.beginPath();
      ctx.moveTo(a.x, a.y);
      ctx.lineTo(b.x, b.y);
      const rgb = edgeGlowError ? "239,68,68" : "106,236,1";
      ctx.strokeStyle = `rgba(${rgb},${edgeGlowAlpha})`;
      ctx.lineWidth = 4 / cam.zoom;
      ctx.stroke();
    }

    ctx.beginPath();
    ctx.moveTo(a.x, a.y);
    ctx.lineTo(b.x, b.y);
    ctx.strokeStyle = isHighlighted ? "rgba(148,163,184,0.7)" : COLORS.edge;
    ctx.lineWidth = (isHighlighted ? 2 : 1) / cam.zoom;
    ctx.stroke();
  }

  // Parent–child links (dotted)
  if (showParentLinks) {
    ctx.setLineDash([4, 4]);
    for (const e of parentLinks) {
      const a = lookup.get(e.source);
      const b = lookup.get(e.target);
      if (!a || !b) continue;
      const isHighlighted = hoveredId && (a.id === hoveredId || b.id === hoveredId);
      ctx.beginPath();
      ctx.moveTo(a.x, a.y);
      ctx.lineTo(b.x, b.y);
      ctx.strokeStyle = isHighlighted ? "rgba(148,163,184,0.6)" : "rgba(148,163,184,0.25)";
      ctx.lineWidth = (isHighlighted ? 2 : 1) / cam.zoom;
      ctx.stroke();
    }
    ctx.setLineDash([]);
  }

  // Nodes
  for (const n of nodes) {
    const isActive = n.id === hoveredId || n.id === dragId;
    const r = n.radius + (isActive ? 3 : 0);

    // Pulse glow — expanding ring behind the node
    const pulse = pulses.get(n.id);
    if (pulse !== undefined) {
      const elapsed = now - pulse.time;
      if (elapsed < PULSE_DURATION) {
        const t = elapsed / PULSE_DURATION;
        const alpha = 0.7 * (1 - t);
        const pulseR = r + 6 + 24 * t;
        const rgb = pulse.isError ? "239,68,68"
          : n.kind === "agent" ? "106,236,1"
          : n.kind === "external" ? "255,255,255"
          : "99,102,241";
        const grad = ctx.createRadialGradient(n.x, n.y, r, n.x, n.y, pulseR);
        grad.addColorStop(0, `rgba(${rgb},${alpha})`);
        grad.addColorStop(1, `rgba(${rgb},0)`);
        ctx.beginPath();
        ctx.arc(n.x, n.y, pulseR, 0, Math.PI * 2);
        ctx.fillStyle = grad;
        ctx.fill();
      } else {
        pulses.delete(n.id);
      }
    }

    if (n.kind === "agent") {
      // Filled background + green border + logo inside
      ctx.shadowColor = "rgba(106,236,1,0.3)";
      ctx.shadowBlur = 12;

      ctx.beginPath();
      ctx.arc(n.x, n.y, r, 0, Math.PI * 2);
      ctx.fillStyle = COLORS.agentBg;
      ctx.fill();
      ctx.strokeStyle = COLORS.agent;
      ctx.lineWidth = (isActive ? 3.5 : 2.5) / cam.zoom;
      ctx.stroke();

      ctx.shadowColor = "transparent";
      ctx.shadowBlur = 0;

      // Draw logo centered inside
      if (logo) {
        const logoSize = r * 1.2;
        ctx.drawImage(logo, n.x - logoSize / 2, n.y - logoSize / 2, logoSize, logoSize);
      }
    } else if (n.kind === "external") {
      // Filled background + white border + person icon
      ctx.beginPath();
      ctx.arc(n.x, n.y, r, 0, Math.PI * 2);
      ctx.fillStyle = COLORS.agentBg;
      ctx.fill();
      ctx.strokeStyle = COLORS.external;
      ctx.lineWidth = (isActive ? 3.5 : 2.5) / cam.zoom;
      ctx.stroke();

      // Person icon (head + shoulders)
      ctx.fillStyle = COLORS.external;
      ctx.beginPath();
      ctx.arc(n.x, n.y - r * 0.15, r * 0.22, 0, Math.PI * 2);
      ctx.fill();
      ctx.beginPath();
      ctx.ellipse(n.x, n.y + r * 0.35, r * 0.35, r * 0.22, 0, Math.PI, 0, true);
      ctx.fill();
    } else {
      // Ticket nodes
      let fill: string;
      let stroke: string;
      if (n.status === "awaiting_close") {
        // Interpolate between open and closed colors using sine wave (~2s cycle)
        const t = (Math.sin(now * Math.PI / 1000) + 1) / 2; // 0..1 over ~2s
        const lerp = (a: string, b: string) => {
          const ai = parseInt(a.slice(1), 16);
          const bi = parseInt(b.slice(1), 16);
          const r = Math.round(((ai >> 16) & 0xff) * (1 - t) + ((bi >> 16) & 0xff) * t);
          const g = Math.round(((ai >> 8) & 0xff) * (1 - t) + ((bi >> 8) & 0xff) * t);
          const bv = Math.round((ai & 0xff) * (1 - t) + (bi & 0xff) * t);
          return `rgb(${r},${g},${bv})`;
        };
        fill = lerp(COLORS.ticketOpen, COLORS.ticketClosed);
        stroke = lerp(COLORS.ticketOpenStroke, COLORS.ticketClosedStroke);
      } else {
        fill = n.status === "open" ? COLORS.ticketOpen : COLORS.ticketClosed;
        stroke = n.status === "open" ? COLORS.ticketOpenStroke : COLORS.ticketClosedStroke;
      }

      ctx.beginPath();
      ctx.arc(n.x, n.y, r, 0, Math.PI * 2);
      ctx.fillStyle = fill;
      ctx.fill();
      ctx.strokeStyle = stroke;
      ctx.lineWidth = (isActive ? 3 : 2) / cam.zoom;
      ctx.stroke();
    }

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
      if (n.status === "awaiting_close") {
        const t = (Math.sin(now * Math.PI / 1000) + 1) / 2;
        const r1 = parseInt(COLORS.text.slice(1), 16);
        const r2 = parseInt(COLORS.textMuted.slice(1), 16);
        const r = Math.round(((r1 >> 16) & 0xff) * (1 - t) + ((r2 >> 16) & 0xff) * t);
        const g = Math.round(((r1 >> 8) & 0xff) * (1 - t) + ((r2 >> 8) & 0xff) * t);
        const b = Math.round((r1 & 0xff) * (1 - t) + (r2 & 0xff) * t);
        ctx.fillStyle = `rgb(${r},${g},${b})`;
      } else {
        ctx.fillStyle = n.status === "closed" ? COLORS.textMuted : COLORS.text;
      }
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
/*  Persistent state (survives SPA navigation)                         */
/* ------------------------------------------------------------------ */

interface PersistedState {
  nodes: GraphNode[];
  edges: GraphEdge[];
  parentLinks: GraphEdge[];
  camera: Camera;
  strength: number;
  showClosed: boolean;
  pinAgents: boolean;
  showParentLinks: boolean;
}

let persisted: PersistedState | null = null;

/* ------------------------------------------------------------------ */
/*  Page component                                                     */
/* ------------------------------------------------------------------ */

export default function GraphPage() {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const nodesRef = useRef<GraphNode[]>(persisted?.nodes ?? []);
  const edgesRef = useRef<GraphEdge[]>(persisted?.edges ?? []);
  const parentLinksRef = useRef<GraphEdge[]>(persisted?.parentLinks ?? []);
  const animRef = useRef<number>(0);
  const hoveredRef = useRef<string | null>(null);
  const dragRef = useRef<{ id: string; offsetX: number; offsetY: number } | null>(null);
  const panRef = useRef<{ startX: number; startY: number; camX: number; camY: number } | null>(null);
  const camRef = useRef<Camera>(persisted?.camera ?? { x: 0, y: 0, zoom: 1 });
  const strengthRef = useRef(persisted?.strength ?? 1);
  const logoRef = useRef<HTMLImageElement | null>(null);
  const pulsesRef = useRef<PulseMap>(new Map());
  const logSinceRef = useRef<number>(0);
  const sizeRef = useRef({ w: 800, h: 600 });
  const mouseDownPosRef = useRef<{ x: number; y: number } | null>(null);
  const [selectedTicketId, setSelectedTicketId] = useState<string | null>(null);

  const hadPersisted = useRef(!!persisted);
  const [loading, setLoading] = useState(!persisted);
  const [strength, setStrength] = useState(persisted?.strength ?? 1);
  const [showClosed, setShowClosed] = useState(persisted?.showClosed ?? true);
  const showClosedRef = useRef(persisted?.showClosed ?? true);
  const [pinAgents, setPinAgents] = useState(persisted?.pinAgents ?? false);
  const pinAgentsRef = useRef(persisted?.pinAgents ?? false);
  const [showParentLinks, setShowParentLinks] = useState(persisted?.showParentLinks ?? true);
  const showParentLinksRef = useRef(persisted?.showParentLinks ?? true);
  const [, setTick] = useState(0); // force re-render for legend

  // Preload logo
  useEffect(() => {
    const img = new Image();
    img.src = "/h1v3-logo.svg";
    img.onload = () => { logoRef.current = img; };
  }, []);

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

  // Load data + poll every 5s
  const loadGraph = useCallback(async (initial: boolean) => {
    try {
      const [agents, tickets] = await Promise.all([
        fetchAgents(),
        fetchTickets({ limit: 200 }),
      ]);
      const { nodes: freshNodes, edges, parentLinks } = buildGraph(agents, tickets);
      parentLinksRef.current = parentLinks;

      if (initial && !hadPersisted.current) {
        nodesRef.current = freshNodes;
        edgesRef.current = edges;
        requestAnimationFrame(() => {
          resize();
          initPositions(nodesRef.current, sizeRef.current.w, sizeRef.current.h);
          setTick((t) => t + 1);
        });
      } else {
        // Merge: preserve positions of existing nodes, init new ones
        const existing = new Map(nodesRef.current.map((n) => [n.id, n]));
        const { w, h } = sizeRef.current;
        for (const n of freshNodes) {
          const prev = existing.get(n.id);
          if (prev) {
            // Keep position/velocity, update mutable data
            prev.label = n.label;
            prev.status = n.status;
          } else {
            // New node — place near center with jitter
            n.x = w / 2 + (Math.random() - 0.5) * 100;
            n.y = h / 2 + (Math.random() - 0.5) * 100;
            n.vx = 0;
            n.vy = 0;
          }
        }
        const freshIds = new Set(freshNodes.map((n) => n.id));
        nodesRef.current = freshNodes.map((n) => existing.get(n.id) ?? n);
        // Remove nodes that no longer exist
        nodesRef.current = nodesRef.current.filter((n) => freshIds.has(n.id));
        edgesRef.current = edges;
      }
    } catch {
      // auth redirect handled by api client
    }
  }, [resize]);

  useEffect(() => {
    loadGraph(true).finally(() => {
      hadPersisted.current = false;
      setLoading(false);
    });
    const interval = setInterval(() => loadGraph(false), POLL_INTERVAL);
    return () => clearInterval(interval);
  }, [loadGraph]);

  // Save state on unmount
  useEffect(() => {
    return () => {
      persisted = {
        nodes: nodesRef.current,
        edges: edgesRef.current,
        parentLinks: parentLinksRef.current,
        camera: { ...camRef.current },
        strength: strengthRef.current,
        showClosed: showClosedRef.current,
        pinAgents: pinAgentsRef.current,
        showParentLinks: showParentLinksRef.current,
      };
    };
  }, []);

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

      const hideClosed = !showClosedRef.current;
      const visibleNodes = hideClosed
        ? nodesRef.current.filter((n) => !(n.kind === "ticket" && n.status === "closed"))
        : nodesRef.current;
      const visibleIds = hideClosed ? new Set(visibleNodes.map((n) => n.id)) : null;
      const visibleEdges = visibleIds
        ? edgesRef.current.filter((e) => visibleIds.has(e.source) && visibleIds.has(e.target))
        : edgesRef.current;

      simulate(visibleNodes, visibleEdges, w, h, strengthRef.current, pinAgentsRef.current);
      // Filter parent links for visible nodes
      const visibleParentLinks = visibleIds
        ? parentLinksRef.current.filter((e) => visibleIds.has(e.source) && visibleIds.has(e.target))
        : parentLinksRef.current;

      draw(ctx, visibleNodes, visibleEdges, w, h, camRef.current, hoveredRef.current, dragRef.current?.id ?? null, logoRef.current, visibleParentLinks, showParentLinksRef.current, pulsesRef.current, performance.now());
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

  // Log polling for pulse effects
  useEffect(() => {
    if (loading) return;
    const poll = async () => {
      try {
        const since = logSinceRef.current > 0 ? logSinceRef.current + 1 : undefined;
        const logs = await fetchLogs({ limit: 50, since });
        if (logs.length === 0) return;
        const last = new Date(logs[logs.length - 1].time).getTime();
        if (last > logSinceRef.current) logSinceRef.current = last;
        const now = performance.now();
        for (const log of logs) {
          const agentId = log.attrs?.agent as string | undefined;
          const ticketId = log.attrs?.ticket as string | undefined;
          const isError = log.level === "ERROR";
          if (agentId) pulsesRef.current.set(agentId, { time: now, isError });
          if (ticketId) pulsesRef.current.set(`ticket:${ticketId}`, { time: now, isError });
          // Edge pulse when log references both agent and ticket
          if (agentId && ticketId) {
            const edgeKey = [agentId, `ticket:${ticketId}`].sort().join("|||");
            pulsesRef.current.set(edgeKey, { time: now, isError });
          }
        }
      } catch {
        // ignore
      }
    };
    // Initial fetch to set baseline (no pulse on first load)
    fetchLogs({ limit: 1 }).then((logs) => {
      if (logs.length > 0) {
        logSinceRef.current = new Date(logs[logs.length - 1].time).getTime();
      }
    }).catch(() => {});
    const interval = setInterval(poll, POLL_INTERVAL);
    return () => clearInterval(interval);
  }, [loading]);

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
      canvas.style.cursor = n ? (n.kind === "ticket" ? "pointer" : "grab") : "default";
    },
    [findNode],
  );

  const onMouseDown = useCallback(
    (e: React.MouseEvent) => {
      mouseDownPosRef.current = { x: e.clientX, y: e.clientY };
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

  const onMouseUp = useCallback((e: React.MouseEvent) => {
    const downPos = mouseDownPosRef.current;
    const wasDrag = dragRef.current;
    dragRef.current = null;
    panRef.current = null;
    mouseDownPosRef.current = null;
    const canvas = canvasRef.current;
    if (canvas) canvas.style.cursor = "default";

    // Detect click (< 5px movement) on a ticket node
    if (downPos && wasDrag) {
      const dx = e.clientX - downPos.x;
      const dy = e.clientY - downPos.y;
      if (dx * dx + dy * dy < 25) {
        const n = findNode(e.clientX, e.clientY);
        if (n && n.kind === "ticket") {
          const ticketId = n.id.replace(/^ticket:/, "");
          setSelectedTicketId(ticketId);
        }
      }
    }
  }, [findNode]);

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
            <span className="inline-block h-3 w-3 rounded-full border-2" style={{ borderColor: COLORS.agent }} />
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
        <div className="flex items-center gap-4 text-xs text-muted-foreground">
          <label className="flex items-center gap-2 cursor-pointer select-none">
            <span>Closed</span>
            <button
              role="switch"
              aria-checked={showClosed}
              onClick={() => {
                setShowClosed((v) => {
                  showClosedRef.current = !v;
                  return !v;
                });
              }}
              className={`relative inline-flex h-4 w-7 items-center rounded-full transition-colors ${showClosed ? "bg-[#6AEC01]" : "bg-muted"}`}
            >
              <span className={`inline-block h-3 w-3 rounded-full bg-white transition-transform ${showClosed ? "translate-x-3.5" : "translate-x-0.5"}`} />
            </button>
          </label>
          <label className="flex items-center gap-2 cursor-pointer select-none">
            <span>Pin</span>
            <button
              role="switch"
              aria-checked={pinAgents}
              onClick={() => {
                setPinAgents((v) => {
                  pinAgentsRef.current = !v;
                  return !v;
                });
              }}
              className={`relative inline-flex h-4 w-7 items-center rounded-full transition-colors ${pinAgents ? "bg-[#6AEC01]" : "bg-muted"}`}
            >
              <span className={`inline-block h-3 w-3 rounded-full bg-white transition-transform ${pinAgents ? "translate-x-3.5" : "translate-x-0.5"}`} />
            </button>
          </label>
          <label className="flex items-center gap-2 cursor-pointer select-none">
            <span>Links</span>
            <button
              role="switch"
              aria-checked={showParentLinks}
              onClick={() => {
                setShowParentLinks((v) => {
                  showParentLinksRef.current = !v;
                  return !v;
                });
              }}
              className={`relative inline-flex h-4 w-7 items-center rounded-full transition-colors ${showParentLinks ? "bg-[#6AEC01]" : "bg-muted"}`}
            >
              <span className={`inline-block h-3 w-3 rounded-full bg-white transition-transform ${showParentLinks ? "translate-x-3.5" : "translate-x-0.5"}`} />
            </button>
          </label>
          <div className="flex items-center gap-2">
            <span>Force</span>
            <input
              type="range"
              min={0.1}
              max={3}
              step={0.1}
              value={strength}
              onChange={(e) => {
                const v = parseFloat(e.target.value);
                setStrength(v);
                strengthRef.current = v;
              }}
              className="h-1 w-24 cursor-pointer accent-[#6AEC01]"
            />
          </div>
        </div>
      </div>
      <div ref={containerRef} className="relative flex-1 min-h-0 rounded-lg border bg-card overflow-hidden">
        <canvas
          ref={canvasRef}
          onMouseMove={onMouseMove}
          onMouseDown={onMouseDown}
          onMouseUp={onMouseUp}
          onMouseLeave={() => {
            dragRef.current = null;
            panRef.current = null;
            mouseDownPosRef.current = null;
            const canvas = canvasRef.current;
            if (canvas) canvas.style.cursor = "default";
          }}
          onWheel={onWheel}
        />
      </div>

      <Dialog
        open={selectedTicketId !== null}
        onOpenChange={(open) => { if (!open) setSelectedTicketId(null); }}
      >
        <DialogContent className="max-w-5xl max-h-[85vh] overflow-y-auto">
          <DialogTitle className="sr-only">Ticket Details</DialogTitle>
          {selectedTicketId && <TicketDetail id={selectedTicketId} onNavigate={setSelectedTicketId} />}
        </DialogContent>
      </Dialog>
    </div>
  );
}
