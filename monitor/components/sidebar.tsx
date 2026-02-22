"use client";

import { useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { PanelLeftClose, PanelLeftOpen } from "lucide-react";
import { cn } from "@/lib/utils";
import { clearAuth } from "@/lib/auth";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";

const navItems = [
  { href: "/", label: "Overview" },
  { href: "/tickets", label: "Tickets" },
  { href: "/logs", label: "Logs" },
];

export function Sidebar() {
  const pathname = usePathname();
  const [collapsed, setCollapsed] = useState(false);

  return (
    <aside
      className={cn(
        "flex h-screen flex-col border-r bg-card py-4 transition-all duration-200",
        collapsed ? "w-14 px-2" : "w-56 px-3"
      )}
    >
      <div className={cn("mb-4 flex items-center", collapsed ? "justify-center" : "justify-between px-2")}>
        <div className="flex items-center gap-2">
          <Image src="/h1v3-logo.svg" alt="h1v3" width={24} height={24} className="shrink-0" />
          {!collapsed && <h1 className="text-lg font-semibold tracking-tight">Monitor</h1>}
        </div>
        <Button
          variant="ghost"
          size="icon"
          className={cn("h-7 w-7 text-muted-foreground", collapsed && "mt-2")}
          onClick={() => setCollapsed((c) => !c)}
        >
          {collapsed ? <PanelLeftOpen className="h-4 w-4" /> : <PanelLeftClose className="h-4 w-4" />}
        </Button>
      </div>
      <Separator className="mb-4" />
      <nav className="flex flex-1 flex-col gap-1">
        {navItems.map((item) => (
          <Link
            key={item.href}
            href={item.href}
            title={collapsed ? item.label : undefined}
            className={cn(
              "rounded-md py-2 text-sm font-medium transition-colors hover:bg-accent",
              collapsed ? "px-0 text-center" : "px-3",
              (item.href === "/"
                ? pathname === "/"
                : pathname.startsWith(item.href))
                ? "bg-accent text-accent-foreground"
                : "text-muted-foreground"
            )}
          >
            {collapsed ? item.label.charAt(0) : item.label}
          </Link>
        ))}
      </nav>
      <Separator className="my-2" />
      <Button
        variant="ghost"
        size="sm"
        className={cn("text-muted-foreground", collapsed ? "justify-center px-0" : "justify-start")}
        title={collapsed ? "Disconnect" : undefined}
        onClick={() => {
          clearAuth();
          window.location.href = "/login";
        }}
      >
        {collapsed ? "X" : "Disconnect"}
      </Button>
    </aside>
  );
}
