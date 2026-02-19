"use client";

import { useEffect, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { TicketTable } from "@/components/ticket-table";
import { Skeleton } from "@/components/ui/skeleton";
import { fetchAgents, fetchTickets } from "@/lib/api";
import type { Agent, Ticket } from "@/lib/api";

export default function OverviewPage() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [tickets, setTickets] = useState<Ticket[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function load() {
      try {
        const [a, t] = await Promise.all([
          fetchAgents(),
          fetchTickets({ limit: 20 }),
        ]);
        setAgents(a);
        setTickets(t);
      } catch {
        // auth redirect handled by api client
      } finally {
        setLoading(false);
      }
    }
    load();
  }, []);

  const openCount = tickets.filter((t) => t.status === "open").length;

  if (loading) {
    return (
      <div className="flex flex-col gap-6">
        <Skeleton className="h-8 w-48" />
        <div className="grid gap-4 md:grid-cols-3">
          <Skeleton className="h-24" />
          <Skeleton className="h-24" />
          <Skeleton className="h-24" />
        </div>
        <Skeleton className="h-64" />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <h2 className="text-2xl font-semibold tracking-tight">Overview</h2>

      <div className="grid gap-4 md:grid-cols-3">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Agents
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold">{agents.length}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Open Tickets
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold">{openCount}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Total Tickets
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold">{tickets.length}</div>
          </CardContent>
        </Card>
      </div>

      <div>
        <h3 className="mb-3 text-lg font-medium">Agents</h3>
        <div className="grid gap-3 md:grid-cols-4">
          {agents.map((a) => (
            <Card key={a.id}>
              <CardContent className="pt-4">
                <div className="font-mono text-sm font-semibold">{a.id}</div>
                <div className="text-xs text-muted-foreground">{a.role}</div>
              </CardContent>
            </Card>
          ))}
        </div>
      </div>

      <div>
        <h3 className="mb-3 text-lg font-medium">Recent Tickets</h3>
        <TicketTable tickets={tickets} />
      </div>
    </div>
  );
}
