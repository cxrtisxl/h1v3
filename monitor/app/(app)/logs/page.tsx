"use client";

import { useEffect, useState, useRef, useCallback, useMemo } from "react";
import { LogTable } from "@/components/log-table";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { ScrollArea } from "@/components/ui/scroll-area";
import { fetchLogs } from "@/lib/api";
import type { LogEntry } from "@/lib/api";

const LEVEL_OPTIONS = ["all", "debug", "info", "warn", "error"] as const;

export default function LogsPage() {
  const [entries, setEntries] = useState<LogEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [level, setLevel] = useState<string>("all");
  const [polling, setPolling] = useState(true);
  const latestTime = useRef<number>(0);

  // Initial load
  const loadAll = useCallback(async () => {
    try {
      const data = await fetchLogs({ limit: 500, level });
      setEntries(data);
      if (data.length > 0) {
        const last = new Date(data[data.length - 1].time).getTime();
        latestTime.current = last;
      }
    } catch {
      // handled by api client
    } finally {
      setLoading(false);
    }
  }, [level]);

  useEffect(() => {
    setLoading(true);
    latestTime.current = 0;
    loadAll();
  }, [loadAll]);

  // Incremental polling
  useEffect(() => {
    if (!polling) return;

    const interval = setInterval(async () => {
      try {
        const since = latestTime.current > 0 ? latestTime.current + 1 : undefined;
        const newEntries = await fetchLogs({ limit: 200, level, since });
        if (newEntries.length > 0) {
          setEntries((prev) => {
            const combined = [...prev, ...newEntries];
            // Keep last 2000
            return combined.length > 2000
              ? combined.slice(combined.length - 2000)
              : combined;
          });
          const last = new Date(newEntries[newEntries.length - 1].time).getTime();
          latestTime.current = last;
        }
      } catch {
        // ignore polling errors
      }
    }, 2000);

    return () => clearInterval(interval);
  }, [polling, level]);

  const sortedEntries = [...entries].reverse();

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h2 className="text-2xl font-semibold tracking-tight">Logs</h2>
        <Button
          variant="outline"
          size="sm"
          onClick={() => setPolling((p) => !p)}
        >
          {polling ? "Pause" : "Resume"}
        </Button>
      </div>

      <div className="flex gap-2">
        {LEVEL_OPTIONS.map((l) => (
          <Button
            key={l}
            variant="outline"
            size="sm"
            className={level === l ? "bg-[#1E1D27] text-foreground border-[#1E1D27]" : ""}
            onClick={() => setLevel(l)}
          >
            {l.toUpperCase()}
          </Button>
        ))}
      </div>

      {loading ? (
        <Skeleton className="h-96" />
      ) : (
        <ScrollArea className="h-[calc(100vh-200px)]">
          <LogTable entries={sortedEntries} />
        </ScrollArea>
      )}
    </div>
  );
}
