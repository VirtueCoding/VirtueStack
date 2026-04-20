"use client";

import { useState } from "react";
import { Bell, Check, Loader2 } from "lucide-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@virtuestack/ui";
import { Badge } from "@virtuestack/ui";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@virtuestack/ui";
import { ScrollArea } from "@virtuestack/ui";
import { useNotifications } from "@/hooks/use-notifications";
import { inAppNotificationApi } from "@/lib/api-client";
import type { InAppNotification } from "@/lib/api-client";

export function NotificationBell() {
  const { unreadCount } = useNotifications();
  const [isOpen, setIsOpen] = useState(false);
  const queryClient = useQueryClient();

  const { data, isLoading } = useQuery({
    queryKey: ["notifications"],
    queryFn: () => inAppNotificationApi.list({ per_page: 20 }),
    enabled: isOpen,
  });

  const notifications = data?.data ?? [];

  const handleMarkAsRead = async (id: string) => {
    await inAppNotificationApi.markAsRead(id);
    queryClient.invalidateQueries({ queryKey: ["notifications"] });
  };

  const handleMarkAllAsRead = async () => {
    await inAppNotificationApi.markAllAsRead();
    queryClient.invalidateQueries({ queryKey: ["notifications"] });
  };

  return (
    <DropdownMenu open={isOpen} onOpenChange={setIsOpen}>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" className="relative">
          <Bell className="h-5 w-5" />
          {unreadCount > 0 && (
            <Badge
              variant="destructive"
              className="absolute -right-1 -top-1 flex h-5 min-w-[1.25rem] items-center justify-center rounded-full px-1 text-xs"
            >
              {unreadCount > 99 ? "99+" : unreadCount}
            </Badge>
          )}
          <span className="sr-only">Notifications</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-80">
        <DropdownMenuLabel className="flex items-center justify-between">
          <span>Notifications</span>
          {unreadCount > 0 && (
            <Button
              variant="ghost"
              size="sm"
              className="h-auto p-0 text-xs text-muted-foreground"
              onClick={handleMarkAllAsRead}
            >
              <Check className="mr-1 h-3 w-3" />
              Mark all read
            </Button>
          )}
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <ScrollArea className="max-h-[300px]">
          {isLoading && (
            <div className="flex items-center justify-center py-4">
              <Loader2 className="h-4 w-4 animate-spin" />
            </div>
          )}
          {!isLoading && notifications.length === 0 && (
            <div className="py-4 text-center text-sm text-muted-foreground">
              No notifications
            </div>
          )}
          {notifications.map((notif: InAppNotification) => (
            <DropdownMenuItem
              key={notif.id}
              className="flex cursor-pointer flex-col items-start gap-1 p-3"
              onClick={() => !notif.read && handleMarkAsRead(notif.id)}
            >
              <div className="flex w-full items-start justify-between gap-2">
                <span className={`text-sm font-medium ${notif.read ? "text-muted-foreground" : ""}`}>
                  {notif.title}
                </span>
                {!notif.read && (
                  <span className="mt-1 h-2 w-2 shrink-0 rounded-full bg-primary" />
                )}
              </div>
              <span className="text-xs text-muted-foreground line-clamp-2">{notif.message}</span>
              <span className="text-xs text-muted-foreground">
                {formatRelativeTime(notif.created_at)}
              </span>
            </DropdownMenuItem>
          ))}
        </ScrollArea>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function formatRelativeTime(dateStr: string): string {
  const now = Date.now();
  const date = new Date(dateStr).getTime();
  const diffMs = now - date;
  const diffMin = Math.floor(diffMs / 60000);

  if (diffMin < 1) return "Just now";
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffHrs = Math.floor(diffMin / 60);
  if (diffHrs < 24) return `${diffHrs}h ago`;
  const diffDays = Math.floor(diffHrs / 24);
  if (diffDays < 7) return `${diffDays}d ago`;
  return new Date(dateStr).toLocaleDateString();
}
