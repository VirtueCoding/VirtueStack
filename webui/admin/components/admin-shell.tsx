"use client";

import { useState } from "react";
import { usePathname } from "next/navigation";

import { MobileNav } from "@/components/mobile-nav";
import { NotificationBell } from "@/components/notification-bell";
import { Sidebar } from "@/components/sidebar";
import { isAdminLoginPath } from "@/lib/pathname";
import { ThemeToggle } from "@/components/theme-toggle";
import { RequireAuth } from "@/lib/require-auth";

export function AdminShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);

  if (isAdminLoginPath(pathname)) {
    return <>{children}</>;
  }

  return (
    <RequireAuth>
      <div className="flex min-h-screen">
        <div className="hidden md:block">
          <Sidebar
            collapsed={sidebarCollapsed}
            onToggle={() => setSidebarCollapsed((current) => !current)}
          />
        </div>

        <div className="flex flex-1 flex-col">
          <header className="sticky top-0 z-10 flex h-14 items-center gap-4 border-b bg-background/80 px-6 backdrop-blur-md supports-[backdrop-filter]:bg-background/60">
            <MobileNav />
            <div className="flex-1" />
            <div className="ml-auto flex items-center gap-2">
              <NotificationBell />
              <ThemeToggle />
            </div>
          </header>

          <main className="flex-1 overflow-auto">{children}</main>
        </div>
      </div>
    </RequireAuth>
  );
}
