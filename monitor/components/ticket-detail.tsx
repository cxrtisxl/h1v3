"use client";

import { useEffect, useState, useCallback } from "react";
import Link from "next/link";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { ScrollArea } from "@/components/ui/scroll-area";
import { MessageThread } from "@/components/message-thread";
import { LogTable } from "@/components/log-table";
import { fetchTicket, fetchTickets, fetchLogs } from "@/lib/api";
import type { Ticket, LogEntry, PromptMessage } from "@/lib/api";
import { POLL_INTERVAL } from "@/lib/config";

function logsForTicket(logs: LogEntry[], ticketId: string): LogEntry[] {
  return logs.filter((e) => {
    if (!e.attrs) return false;
    return Object.values(e.attrs).some(
      (v) => typeof v === "string" && v === ticketId
    );
  });
}

function buildPromptContextMap(
  logs: LogEntry[]
): Record<string, PromptMessage[]> {
  const map: Record<string, PromptMessage[]> = {};
  for (const entry of logs) {
    if (entry.message !== "prompt_context") continue;
    const msgId = entry.attrs?.msg_id as string | undefined;
    const raw = entry.attrs?.context as string | undefined;
    if (!msgId || !raw) continue;
    try {
      const parsed = JSON.parse(raw) as PromptMessage[];
      if (Array.isArray(parsed)) {
        map[msgId] = parsed;
      }
    } catch {
      // skip malformed entries
    }
  }
  return map;
}

export function TicketDetail({ id, onNavigate }: { id: string; onNavigate?: (ticketId: string) => void }) {
  const [ticket, setTicket] = useState<Ticket | null>(null);
  const [parent, setParent] = useState<Ticket | null>(null);
  const [children, setChildren] = useState<Ticket[]>([]);
  const [ticketLogs, setTicketLogs] = useState<LogEntry[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    try {
      const [data, allLogs, childTickets] = await Promise.all([
        fetchTicket(id),
        fetchLogs({ limit: 2000 }),
        fetchTickets({ parent_id: id }),
      ]);
      setTicket(data);
      setTicketLogs(logsForTicket(allLogs, id));
      setChildren(childTickets);

      // Fetch parent if exists
      if (data.parent_ticket_id) {
        try {
          const p = await fetchTicket(data.parent_ticket_id);
          setParent(p);
        } catch {
          setParent(null);
        }
      } else {
        setParent(null);
      }
    } catch {
      // handled by api client
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    setLoading(true);
    load();
  }, [load]);

  // Auto-refresh for open tickets
  useEffect(() => {
    if (!ticket || ticket.status !== "open") return;
    const interval = setInterval(load, POLL_INTERVAL);
    return () => clearInterval(interval);
  }, [ticket, load]);

  const reversedLogs = ticketLogs.slice().reverse();

  if (loading) {
    return (
      <div className="flex flex-col gap-4">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-48" />
        <Skeleton className="h-96" />
      </div>
    );
  }

  if (!ticket) {
    return <p className="text-muted-foreground">Ticket not found.</p>;
  }

  const promptContextMap = buildPromptContextMap(ticketLogs);
  const hasRelated = parent || children.length > 0;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-1">
        <div className="flex items-center gap-3">
          <h2 className="text-2xl font-semibold tracking-tight">
            {ticket.title}
          </h2>
          <Badge variant={ticket.status === "open" ? "default" : "secondary"}>
            {ticket.status}
          </Badge>
        </div>
        {ticket.goal && (
          <p className="text-sm text-muted-foreground">
            Goal: {ticket.goal}
          </p>
        )}
      </div>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm text-muted-foreground">
            Details
          </CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-2 gap-x-6 gap-y-2 text-sm md:grid-cols-4">
            <div>
              <dt className="text-muted-foreground">ID</dt>
              <dd className="font-mono">{ticket.id.slice(0, 8)}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Created By</dt>
              <dd className="font-mono">{ticket.created_by}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Waiting On</dt>
              <dd className="font-mono">
                {ticket.waiting_on?.join(", ") || "-"}
              </dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Tags</dt>
              <dd className="flex gap-1">
                {ticket.tags?.length
                  ? ticket.tags.map((t) => (
                      <Badge key={t} variant="outline" className="text-xs">
                        {t}
                      </Badge>
                    ))
                  : "-"}
              </dd>
            </div>
          </dl>
          {ticket.summary && (
            <div className="mt-3 border-t pt-3">
              <p className="text-sm text-muted-foreground">Summary</p>
              <p className="text-sm">{ticket.summary}</p>
            </div>
          )}
        </CardContent>
      </Card>

      {hasRelated && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm text-muted-foreground">
              Related Tickets
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            {parent && (
              <div>
                <p className="text-xs text-muted-foreground mb-1">Parent</p>
                {onNavigate ? (
                  <button
                    onClick={() => onNavigate(parent.id)}
                    className="flex w-full items-center gap-2 rounded-md border p-2 hover:bg-muted/50 transition-colors text-left"
                  >
                    <Badge variant={parent.status === "open" ? "default" : "secondary"} className="text-xs">
                      {parent.status}
                    </Badge>
                    <span className="font-mono text-xs text-muted-foreground">
                      {parent.id.slice(0, 8)}
                    </span>
                    <span className="text-sm truncate">{parent.title}</span>
                    <span className="text-xs text-muted-foreground ml-auto">
                      {parent.created_by}
                    </span>
                  </button>
                ) : (
                  <Link
                    href={`/tickets/${parent.id}`}
                    className="flex items-center gap-2 rounded-md border p-2 hover:bg-muted/50 transition-colors"
                  >
                    <Badge variant={parent.status === "open" ? "default" : "secondary"} className="text-xs">
                      {parent.status}
                    </Badge>
                    <span className="font-mono text-xs text-muted-foreground">
                      {parent.id.slice(0, 8)}
                    </span>
                    <span className="text-sm truncate">{parent.title}</span>
                    <span className="text-xs text-muted-foreground ml-auto">
                      {parent.created_by}
                    </span>
                  </Link>
                )}
              </div>
            )}
            {children.length > 0 && (
              <div>
                <p className="text-xs text-muted-foreground mb-1">
                  Sub-tickets ({children.length})
                </p>
                <div className="flex flex-col gap-1">
                  {children.map((child) =>
                    onNavigate ? (
                      <button
                        key={child.id}
                        onClick={() => onNavigate(child.id)}
                        className="flex w-full items-center gap-2 rounded-md border p-2 hover:bg-muted/50 transition-colors text-left"
                      >
                        <Badge variant={child.status === "open" ? "default" : "secondary"} className="text-xs">
                          {child.status}
                        </Badge>
                        <span className="font-mono text-xs text-muted-foreground">
                          {child.id.slice(0, 8)}
                        </span>
                        <span className="text-sm truncate">{child.title}</span>
                        {child.goal && (
                          <span className="text-xs text-muted-foreground truncate max-w-[200px]">
                            {child.goal}
                          </span>
                        )}
                        <span className="text-xs text-muted-foreground ml-auto whitespace-nowrap">
                          {child.waiting_on?.join(", ")}
                        </span>
                      </button>
                    ) : (
                      <Link
                        key={child.id}
                        href={`/tickets/${child.id}`}
                        className="flex items-center gap-2 rounded-md border p-2 hover:bg-muted/50 transition-colors"
                      >
                        <Badge variant={child.status === "open" ? "default" : "secondary"} className="text-xs">
                          {child.status}
                        </Badge>
                        <span className="font-mono text-xs text-muted-foreground">
                          {child.id.slice(0, 8)}
                        </span>
                        <span className="text-sm truncate">{child.title}</span>
                        {child.goal && (
                          <span className="text-xs text-muted-foreground truncate max-w-[200px]">
                            {child.goal}
                          </span>
                        )}
                        <span className="text-xs text-muted-foreground ml-auto whitespace-nowrap">
                          {child.waiting_on?.join(", ")}
                        </span>
                      </Link>
                    )
                  )}
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      )}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm text-muted-foreground">
              Messages ({ticket.messages?.length || 0})
            </CardTitle>
          </CardHeader>
          <CardContent>
            <ScrollArea className="h-[500px] pr-4">
              <MessageThread
                messages={ticket.messages || []}
                promptContextMap={promptContextMap}
              />
            </ScrollArea>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm text-muted-foreground">
              Logs ({ticketLogs.length})
            </CardTitle>
          </CardHeader>
          <CardContent>
            <ScrollArea className="h-[500px]">
              <LogTable entries={reversedLogs} compact promptContextMap={promptContextMap} />
            </ScrollArea>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
