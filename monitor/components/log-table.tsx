"use client";

import { useState, useCallback } from "react";
import { Code } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { ContextDialog } from "@/components/context-dialog";
import { ToolDetailDialog } from "@/components/tool-detail-dialog";
import type { LogEntry, PromptMessage } from "@/lib/api";

const levelVariant: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
  DEBUG: "outline",
  INFO: "secondary",
  WARN: "default",
  ERROR: "destructive",
};

function formatTime(ts: string) {
  try {
    return new Date(ts).toLocaleTimeString([], {
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    });
  } catch {
    return ts;
  }
}

function formatLogLine(e: LogEntry): string {
  const ts = e.time ? new Date(e.time).toISOString() : "";
  const agent = (e.attrs?.agent as string) || "";
  const attrs = formatAttrs(e.attrs);
  const parts = [ts, e.level, agent, e.message];
  if (attrs) parts.push(attrs);
  return parts.filter(Boolean).join(" ");
}

function formatAttrs(attrs?: Record<string, unknown>): string {
  if (!attrs || Object.keys(attrs).length === 0) return "";
  const parts: string[] = [];
  for (const [k, v] of Object.entries(attrs)) {
    if (k === "agent") continue;
    parts.push(`${k}=${typeof v === "string" ? v : JSON.stringify(v)}`);
  }
  return parts.join(" ");
}

export function LogTable({
  entries,
  compact,
  promptContextMap,
}: {
  entries: LogEntry[];
  compact?: boolean;
  promptContextMap?: Record<string, PromptMessage[]>;
}) {
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [copied, setCopied] = useState(false);
  const [contextMessages, setContextMessages] = useState<PromptMessage[]>([]);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [toolDetail, setToolDetail] = useState<{ title: string; content: string } | null>(null);

  const allSelected = entries.length > 0 && selected.size === entries.length;

  const toggleAll = useCallback(() => {
    if (allSelected) {
      setSelected(new Set());
    } else {
      setSelected(new Set(entries.map((_, i) => i)));
    }
  }, [allSelected, entries]);

  const toggle = useCallback((idx: number) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(idx)) {
        next.delete(idx);
      } else {
        next.add(idx);
      }
      return next;
    });
  }, []);

  const copySelected = useCallback(() => {
    const lines = Array.from(selected)
      .sort((a, b) => a - b)
      .map((i) => formatLogLine(entries[i]));
    navigator.clipboard.writeText(lines.join("\n")).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  }, [selected, entries]);

  if (entries.length === 0) {
    return (
      <p className="py-8 text-center text-sm text-muted-foreground">
        No log entries.
      </p>
    );
  }

  return (
    <div className="flex flex-col gap-2">
      {selected.size > 0 && (
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={copySelected}>
            {copied ? "Copied!" : `Copy ${selected.size} row${selected.size > 1 ? "s" : ""}`}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="text-muted-foreground"
            onClick={() => setSelected(new Set())}
          >
            Clear
          </Button>
        </div>
      )}
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-8 p-0 pl-1">
              <input
                type="checkbox"
                checked={allSelected}
                onChange={toggleAll}
                className="h-3.5 w-3.5 accent-primary"
              />
            </TableHead>
            <TableHead className="w-[80px]">Time</TableHead>
            <TableHead className="w-[80px]">Level</TableHead>
            <TableHead className="w-[100px]">Agent</TableHead>
            <TableHead>Message</TableHead>
            {!compact && <TableHead>Attrs</TableHead>}
          </TableRow>
        </TableHeader>
        <TableBody>
          {entries.map((e, i) => (
            <TableRow
              key={i}
              className={`font-mono text-xs cursor-pointer ${selected.has(i) ? "bg-muted/60" : ""}`}
              onClick={() => toggle(i)}
            >
              <TableCell className="p-0 pl-1" onClick={(ev) => ev.stopPropagation()}>
                <input
                  type="checkbox"
                  checked={selected.has(i)}
                  onChange={() => toggle(i)}
                  className="h-3.5 w-3.5 accent-primary"
                />
              </TableCell>
              <TableCell className="text-muted-foreground">
                {formatTime(e.time)}
              </TableCell>
              <TableCell>
                <Badge variant={levelVariant[e.level] || "outline"}>
                  {e.level}
                </Badge>
              </TableCell>
              <TableCell className="text-muted-foreground">
                {(e.attrs?.agent as string) || "-"}
              </TableCell>
              <TableCell className="max-w-md truncate">
                {(() => {
                  const ctxId = e.attrs?.prompt_context_id as string | undefined;
                  const hasCtx = !!(promptContextMap && ctxId && promptContextMap[ctxId]);
                  const isToolLog = e.message.startsWith("tool call:") || e.message.startsWith("tool result:") || e.message.startsWith("tool error:");
                  return (
                    <span className="inline-flex items-center gap-1.5">
                      {e.message}
                      {isToolLog && (
                        <button
                          onClick={(ev) => {
                            ev.stopPropagation();
                            const entries = Object.entries(e.attrs || {}).filter(([k]) => k !== "agent");
                            const parts: string[] = [];
                            for (const [k, v] of entries) {
                              if (k === "args" && typeof v === "string") {
                                try {
                                  const parsed = JSON.parse(v);
                                  for (const [ak, av] of Object.entries(parsed)) {
                                    parts.push(`${ak}: ${typeof av === "string" ? av : JSON.stringify(av, null, 2)}`);
                                  }
                                } catch {
                                  parts.push(`args: ${v}`);
                                }
                              } else {
                                parts.push(`${k}: ${typeof v === "string" ? v : JSON.stringify(v, null, 2)}`);
                              }
                            }
                            setToolDetail({
                              title: e.message,
                              content: parts.length > 0 ? parts.join("\n\n") : "(no details available)",
                            });
                          }}
                          className="text-muted-foreground hover:text-foreground transition-colors"
                          title="View tool details"
                        >
                          <Code className="h-3.5 w-3.5" />
                        </button>
                      )}
                      {hasCtx && (
                        <button
                          onClick={(ev) => {
                            ev.stopPropagation();
                            setContextMessages(promptContextMap![ctxId!]!);
                            setDialogOpen(true);
                          }}
                          className="text-muted-foreground hover:text-foreground transition-colors"
                          title="View prompt context"
                        >
                          <Code className="h-3.5 w-3.5" />
                        </button>
                      )}
                    </span>
                  );
                })()}
              </TableCell>
              {!compact && (
                <TableCell className="max-w-xs truncate text-muted-foreground">
                  {formatAttrs(e.attrs)}
                </TableCell>
              )}
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <ContextDialog
        messages={contextMessages}
        open={dialogOpen}
        onOpenChange={setDialogOpen}
      />
      <ToolDetailDialog
        title={toolDetail?.title ?? ""}
        content={toolDetail?.content ?? ""}
        open={!!toolDetail}
        onOpenChange={(open) => { if (!open) setToolDetail(null); }}
      />
    </div>
  );
}
