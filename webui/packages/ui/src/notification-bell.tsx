"use client"

import { useState } from "react"
import { Bell, Check, Loader2 } from "lucide-react"

import { Badge } from "./badge"
import { Button } from "./button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "./dropdown-menu"
import { ScrollArea } from "./scroll-area"

export interface NotificationBellItem {
  id: string
  title: string
  message: string
  read: boolean
  created_at: string
}

export interface NotificationBellProps {
  unreadCount: number
  notifications: NotificationBellItem[]
  isLoading: boolean
  onOpen: () => void
  onMarkAsRead: (id: string) => Promise<void>
  onMarkAllAsRead: () => Promise<void>
}

export function NotificationBellView({
  unreadCount,
  notifications,
  isLoading,
  onOpen,
  onMarkAsRead,
  onMarkAllAsRead,
}: NotificationBellProps) {
  const [isOpen, setIsOpen] = useState(false)

  const handleOpenChange = (open: boolean) => {
    setIsOpen(open)
    if (open) {
      onOpen()
    }
  }

  return (
    <DropdownMenu open={isOpen} onOpenChange={handleOpenChange}>
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
              onClick={onMarkAllAsRead}
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
          {notifications.map((notification) => (
            <DropdownMenuItem
              key={notification.id}
              className="flex cursor-pointer flex-col items-start gap-1 p-3"
              onClick={() => {
                if (!notification.read) {
                  void onMarkAsRead(notification.id)
                }
              }}
            >
              <div className="flex w-full items-start justify-between gap-2">
                <span className={`text-sm font-medium ${notification.read ? "text-muted-foreground" : ""}`}>
                  {notification.title}
                </span>
                {!notification.read && (
                  <span className="mt-1 h-2 w-2 shrink-0 rounded-full bg-primary" />
                )}
              </div>
              <span className="line-clamp-2 text-xs text-muted-foreground">
                {notification.message}
              </span>
              <span className="text-xs text-muted-foreground">
                {formatRelativeTime(notification.created_at)}
              </span>
            </DropdownMenuItem>
          ))}
        </ScrollArea>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function formatRelativeTime(dateStr: string): string {
  const now = Date.now()
  const date = new Date(dateStr).getTime()
  const diffMs = now - date
  const diffMin = Math.floor(diffMs / 60000)

  if (diffMin < 1) return "Just now"
  if (diffMin < 60) return `${diffMin}m ago`
  const diffHrs = Math.floor(diffMin / 60)
  if (diffHrs < 24) return `${diffHrs}h ago`
  const diffDays = Math.floor(diffHrs / 24)
  if (diffDays < 7) return `${diffDays}d ago`
  return new Date(dateStr).toLocaleDateString()
}
