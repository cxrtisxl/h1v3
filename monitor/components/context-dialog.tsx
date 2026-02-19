"use client";

import { useState } from "react";
import { Copy, Check } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Badge } from "@/components/ui/badge";
import type { PromptMessage } from "@/lib/api";

export function ContextDialog({
  messages,
  open,
  onOpenChange,
}: {
  messages: PromptMessage[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [copied, setCopied] = useState(false);

  function handleCopy() {
    const text = messages
      .map((m) => `[${m.role}]\n${m.content}`)
      .join("\n\n---\n\n");
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl max-h-[80vh]">
        <DialogHeader className="flex flex-row items-center justify-between gap-2 pr-8">
          <DialogTitle>Prompt Context ({messages.length} messages)</DialogTitle>
          <Button variant="outline" size="sm" onClick={handleCopy}>
            {copied ? (
              <Check className="h-3.5 w-3.5 mr-1.5" />
            ) : (
              <Copy className="h-3.5 w-3.5 mr-1.5" />
            )}
            {copied ? "Copied" : "Copy all"}
          </Button>
        </DialogHeader>
        <ScrollArea className="h-[60vh] pr-4">
          <div className="flex flex-col gap-3">
            {messages.map((msg, i) => (
              <div key={i} className="flex flex-col gap-1">
                <Badge
                  variant={
                    msg.role === "error"
                      ? "destructive"
                      : msg.role === "system"
                        ? "default"
                        : msg.role === "assistant"
                          ? "secondary"
                          : "outline"
                  }
                  className="w-fit text-xs"
                >
                  {msg.role}
                </Badge>
                <pre
                  className={`whitespace-pre-wrap break-words rounded-md p-3 font-mono text-xs ${
                    msg.role === "error"
                      ? "bg-destructive/10 text-destructive"
                      : "bg-muted"
                  }`}
                >
                  {msg.content}
                </pre>
              </div>
            ))}
          </div>
        </ScrollArea>
      </DialogContent>
    </Dialog>
  );
}
