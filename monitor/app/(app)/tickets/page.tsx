"use client";

import { useEffect, useState, useCallback } from "react";
import { TicketTable } from "@/components/ticket-table";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { fetchTickets } from "@/lib/api";
import type { Ticket } from "@/lib/api";

const STATUS_OPTIONS = ["all", "open", "closed"] as const;

export default function TicketsPage() {
  const [tickets, setTickets] = useState<Ticket[]>([]);
  const [loading, setLoading] = useState(true);
  const [status, setStatus] = useState<string>("all");

  const load = useCallback(async () => {
    try {
      const data = await fetchTickets({ status, limit: 100 });
      setTickets(data);
    } catch {
      // auth redirect handled by api client
    } finally {
      setLoading(false);
    }
  }, [status]);

  useEffect(() => {
    setLoading(true);
    load();
  }, [load]);

  return (
    <div className="flex flex-col gap-4">
      <h2 className="text-2xl font-semibold tracking-tight">Tickets</h2>

      <div className="flex gap-2">
        {STATUS_OPTIONS.map((s) => (
          <Button
            key={s}
            variant={status === s ? "default" : "outline"}
            size="sm"
            onClick={() => setStatus(s)}
          >
            {s.charAt(0).toUpperCase() + s.slice(1)}
          </Button>
        ))}
      </div>

      {loading ? (
        <Skeleton className="h-64" />
      ) : (
        <TicketTable tickets={tickets} />
      )}
    </div>
  );
}
