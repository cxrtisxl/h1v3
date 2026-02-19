import { getApiUrl, getApiKey, clearAuth } from "./auth";

export interface Agent {
  id: string;
  role: string;
}

export interface Message {
  id: string;
  from: string;
  to: string[];
  content: string;
  ticket_id: string;
  timestamp: string;
}

export interface PromptMessage {
  role: string;
  content: string;
}

export interface Ticket {
  id: string;
  title: string;
  goal?: string;
  parent_ticket_id?: string;
  status: "open" | "closed";
  created_by: string;
  waiting_on: string[];
  tags: string[];
  messages: Message[];
  created_at: string;
  closed_at?: string;
  summary?: string;
}

export interface LogEntry {
  time: string;
  level: string;
  message: string;
  attrs?: Record<string, unknown>;
}

class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const baseUrl = getApiUrl();
  if (!baseUrl) throw new ApiError(0, "Not authenticated");

  const apiKey = getApiKey();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(init?.headers as Record<string, string>),
  };
  if (apiKey) {
    headers["Authorization"] = `Bearer ${apiKey}`;
  }

  const res = await fetch(`${baseUrl}${path}`, { ...init, headers });

  if (res.status === 401) {
    clearAuth();
    if (typeof window !== "undefined") {
      window.location.href = "/login";
    }
    throw new ApiError(401, "Unauthorized");
  }

  if (!res.ok) {
    const body = await res.text();
    throw new ApiError(res.status, body);
  }

  return res.json();
}

export async function fetchAgents(): Promise<Agent[]> {
  return (await apiFetch<Agent[] | null>("/api/agents")) ?? [];
}

export async function fetchTickets(params?: {
  status?: string;
  agent?: string;
  parent_id?: string;
  limit?: number;
}): Promise<Ticket[]> {
  const qs = new URLSearchParams();
  if (params?.status && params.status !== "all") qs.set("status", params.status);
  if (params?.agent) qs.set("agent", params.agent);
  if (params?.parent_id) qs.set("parent_id", params.parent_id);
  if (params?.limit) qs.set("limit", String(params.limit));
  const query = qs.toString();
  return (await apiFetch<Ticket[] | null>(`/api/tickets${query ? `?${query}` : ""}`)) ?? [];
}

export function fetchTicket(id: string): Promise<Ticket> {
  return apiFetch(`/api/tickets/${id}`);
}

export async function fetchLogs(params?: {
  limit?: number;
  level?: string;
  since?: number;
}): Promise<LogEntry[]> {
  const qs = new URLSearchParams();
  if (params?.limit) qs.set("limit", String(params.limit));
  if (params?.level && params.level !== "all") qs.set("level", params.level);
  if (params?.since) qs.set("since", String(params.since));
  const query = qs.toString();
  return (await apiFetch<LogEntry[] | null>(`/api/logs${query ? `?${query}` : ""}`)) ?? [];
}

export function postMessage(body: {
  from?: string;
  ticket_id?: string;
  content: string;
}): Promise<{ status: string; ticket_id: string }> {
  return apiFetch("/api/messages", {
    method: "POST",
    body: JSON.stringify(body),
  });
}
