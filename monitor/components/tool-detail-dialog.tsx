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

export function ToolDetailDialog({
  title,
  content,
  open,
  onOpenChange,
}: {
  title: string;
  content: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [copied, setCopied] = useState(false);

  function handleCopy() {
    navigator.clipboard.writeText(content).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl max-h-[80vh]">
        <DialogHeader className="flex flex-row items-center justify-between gap-2 pr-8">
          <DialogTitle>{title}</DialogTitle>
          <Button variant="outline" size="sm" onClick={handleCopy}>
            {copied ? (
              <Check className="h-3.5 w-3.5 mr-1.5" />
            ) : (
              <Copy className="h-3.5 w-3.5 mr-1.5" />
            )}
            {copied ? "Copied" : "Copy"}
          </Button>
        </DialogHeader>
        <ScrollArea className="h-[60vh] pr-4">
          <pre className="whitespace-pre-wrap break-words rounded-md bg-muted p-3 font-mono text-xs">
            {content}
          </pre>
        </ScrollArea>
      </DialogContent>
    </Dialog>
  );
}
