"use client";

import { useState } from "react";
import { Code } from "lucide-react";
import { cn } from "@/lib/utils";
import type { Message, PromptMessage } from "@/lib/api";
import { ContextDialog } from "@/components/context-dialog";

function formatTime(ts: string) {
  try {
    return new Date(ts).toLocaleTimeString();
  } catch {
    return ts;
  }
}

export function MessageThread({
  messages,
  promptContextMap,
}: {
  messages: Message[];
  promptContextMap?: Record<string, PromptMessage[]>;
}) {
  const [contextMessages, setContextMessages] = useState<PromptMessage[]>([]);
  const [dialogOpen, setDialogOpen] = useState(false);

  if (!messages || messages.length === 0) {
    return (
      <p className="py-8 text-center text-sm text-muted-foreground">
        No messages yet.
      </p>
    );
  }

  return (
    <>
      <div className="flex flex-col gap-3">
        {messages.map((msg, i) => {
          const isExternal =
            msg.from === "api" ||
            msg.from === "user" ||
            msg.from === "_external";
          const hasContext =
            promptContextMap && msg.id && promptContextMap[msg.id];
          return (
            <div
              key={i}
              className={cn(
                "group flex flex-col gap-1 rounded-lg px-4 py-3 max-w-[85%]",
                isExternal
                  ? "self-end bg-primary text-primary-foreground"
                  : "self-start bg-muted"
              )}
            >
              <div className="flex items-center gap-2">
                <span className="text-xs font-semibold">{msg.from}</span>
                <span
                  className={cn(
                    "text-xs",
                    isExternal
                      ? "text-primary-foreground/60"
                      : "text-muted-foreground"
                  )}
                >
                  {formatTime(msg.timestamp)}
                </span>
                {hasContext && (
                  <button
                    onClick={() => {
                      setContextMessages(promptContextMap[msg.id]!);
                      setDialogOpen(true);
                    }}
                    className={cn(
                      "ml-auto",
                      isExternal
                        ? "text-primary-foreground/60 hover:text-primary-foreground"
                        : "text-muted-foreground hover:text-foreground"
                    )}
                    title="View prompt context"
                  >
                    <Code className="h-3.5 w-3.5" />
                  </button>
                )}
              </div>
              <p className="whitespace-pre-wrap text-sm">{msg.content}</p>
            </div>
          );
        })}
      </div>
      <ContextDialog
        messages={contextMessages}
        open={dialogOpen}
        onOpenChange={setDialogOpen}
      />
    </>
  );
}
