"use client";

import Link from "next/link";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { Ticket } from "@/lib/api";

function formatTime(ts: string) {
  try {
    return new Date(ts).toLocaleString();
  } catch {
    return ts;
  }
}

export function TicketTable({ tickets }: { tickets: Ticket[] }) {
  if (tickets.length === 0) {
    return (
      <p className="py-8 text-center text-sm text-muted-foreground">
        No tickets found.
      </p>
    );
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead className="w-[100px]">ID</TableHead>
          <TableHead>Title</TableHead>
          <TableHead className="w-[80px]">Status</TableHead>
          <TableHead>Created By</TableHead>
          <TableHead>Waiting On</TableHead>
          <TableHead className="w-[180px]">Created</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {tickets.map((t) => (
          <TableRow key={t.id} className="cursor-pointer hover:bg-muted/50">
            <TableCell className="font-mono text-xs">
              <Link href={`/tickets/${t.id}`} className="hover:underline">
                {t.id.slice(0, 8)}
              </Link>
            </TableCell>
            <TableCell>
              <Link href={`/tickets/${t.id}`} className="hover:underline">
                {t.title}
              </Link>
              {t.goal && (
                <p className="text-xs text-muted-foreground truncate max-w-xs">
                  {t.goal}
                </p>
              )}
            </TableCell>
            <TableCell>
              <Badge variant={t.status === "open" ? "default" : "secondary"}>
                {t.status}
              </Badge>
            </TableCell>
            <TableCell className="font-mono text-xs">{t.created_by}</TableCell>
            <TableCell className="font-mono text-xs">
              {t.waiting_on?.join(", ") || "-"}
            </TableCell>
            <TableCell className="text-xs text-muted-foreground">
              {formatTime(t.created_at)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
