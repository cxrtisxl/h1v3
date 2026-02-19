"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
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

  return (
    <aside className="flex h-screen w-56 flex-col border-r bg-card px-3 py-4">
      <div className="mb-4 px-2">
        <h1 className="text-lg font-semibold tracking-tight">h1v3 monitor</h1>
      </div>
      <Separator className="mb-4" />
      <nav className="flex flex-1 flex-col gap-1">
        {navItems.map((item) => (
          <Link
            key={item.href}
            href={item.href}
            className={cn(
              "rounded-md px-3 py-2 text-sm font-medium transition-colors hover:bg-accent",
              (item.href === "/"
                ? pathname === "/"
                : pathname.startsWith(item.href))
                ? "bg-accent text-accent-foreground"
                : "text-muted-foreground"
            )}
          >
            {item.label}
          </Link>
        ))}
      </nav>
      <Separator className="my-2" />
      <Button
        variant="ghost"
        size="sm"
        className="justify-start text-muted-foreground"
        onClick={() => {
          clearAuth();
          window.location.href = "/login";
        }}
      >
        Disconnect
      </Button>
    </aside>
  );
}
