"use client";

import { LogOut, Settings, ChevronLeft } from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";

import { cn } from "@/lib/utils";
import { useAuth } from "@/lib/auth-context";
import { adminNavGroups } from "@/lib/navigation";
import { isAdminNavItemActive } from "@/lib/pathname";
import { Button, ScrollArea } from "@virtuestack/ui";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@virtuestack/ui";
import { Avatar, AvatarFallback, AvatarImage } from "@virtuestack/ui";
import { NotificationBell } from "@/components/notification-bell";

interface SidebarProps {
  collapsed?: boolean;
  onToggle?: () => void;
}

export function Sidebar({ collapsed = false, onToggle }: SidebarProps) {
  const pathname = usePathname();
  const { user, logout } = useAuth();

  const userEmail = user?.email || "Admin";
  const localPart = userEmail.split("@")[0];
  const initials = localPart?.trim()
    ? localPart.slice(0, 2).toUpperCase()
    : "??";

  return (
    <div
      className={cn(
        "relative flex h-screen flex-col border-r bg-sidebar transition-all duration-300",
        collapsed ? "w-16" : "w-64"
      )}
    >
      <div className="flex h-14 items-center border-b border-sidebar-border px-4">
        {!collapsed && (
          <span className="text-lg font-semibold tracking-tight">
            VirtueStack
          </span>
        )}
        <div
          className={cn(
            "ml-auto flex items-center gap-1",
            collapsed && "mx-auto"
          )}
        >
          {!collapsed && <NotificationBell />}
          <Button variant="ghost" size="icon" onClick={onToggle}>
            <ChevronLeft
              className={cn(
                "h-4 w-4 transition-transform duration-200",
                collapsed && "rotate-180"
              )}
            />
            <span className="sr-only">Toggle sidebar</span>
          </Button>
        </div>
      </div>

      <ScrollArea className="flex-1 py-2">
        <nav className="flex flex-col gap-1 px-2">
          {adminNavGroups.map((group) => (
            <div key={group.label} className="mt-2 first:mt-0">
              {!collapsed && (
                <p className="mb-1 px-3 text-[11px] font-medium uppercase tracking-wider text-muted-foreground/70">
                  {group.label}
                </p>
              )}
              {collapsed && <div className="mx-auto my-1 h-px w-6 bg-sidebar-border" />}
              {group.items.map((item) => {
                const Icon = item.icon;
                const isActive = isAdminNavItemActive(pathname, item.href);
                return (
                  <Link
                    key={item.href}
                    href={item.href}
                    title={collapsed ? item.label : undefined}
                    className={cn(
                      "group relative flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-all duration-150",
                      isActive
                        ? "bg-primary/10 text-primary dark:text-primary"
                        : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
                      collapsed && "justify-center px-2"
                    )}
                  >
                    {isActive && (
                      <span className="absolute left-0 top-1/2 h-5 w-[3px] -translate-y-1/2 rounded-r-full bg-primary" />
                    )}
                    <Icon
                      className={cn(
                        "h-4 w-4 shrink-0 transition-colors",
                        isActive && "text-primary"
                      )}
                    />
                    {!collapsed && <span>{item.label}</span>}
                  </Link>
                );
              })}
            </div>
          ))}
        </nav>
      </ScrollArea>

      <div className="border-t border-sidebar-border p-3">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              className={cn(
                "w-full justify-start gap-3 rounded-lg",
                collapsed && "justify-center px-2"
              )}
            >
              <Avatar className="h-8 w-8">
                <AvatarFallback className="bg-primary/10 text-primary text-xs">
                  {initials}
                </AvatarFallback>
              </Avatar>
              {!collapsed && (
                <div className="flex flex-col items-start text-xs">
                  <span className="font-medium">{localPart}</span>
                  <span className="text-muted-foreground">
                    {user?.role || "Admin"}
                  </span>
                </div>
              )}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-56">
            <DropdownMenuLabel>My Account</DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild>
              <Link href="/settings">
                <Settings className="mr-2 h-4 w-4" />
                Settings
              </Link>
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => logout()}>
              <LogOut className="mr-2 h-4 w-4" />
              Log out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  );
}
