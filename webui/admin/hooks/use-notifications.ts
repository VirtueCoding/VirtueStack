"use client";

import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth-context";
import { inAppNotificationApi } from "@/lib/api-client";

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || "/api/v1";

interface UseNotificationsResult {
  unreadCount: number;
  isConnected: boolean;
}

export function useNotifications(): UseNotificationsResult {
  const [unreadCount, setUnreadCount] = useState(0);
  const [isConnected, setIsConnected] = useState(false);
  const eventSourceRef = useRef<EventSource | null>(null);
  const retryTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const retryDelayRef = useRef(1000);
  const { isAuthenticated } = useAuth();
  const queryClient = useQueryClient();

  useEffect(() => {
    if (!isAuthenticated) return;
    let disposed = false;

    const clearRetry = () => {
      if (retryTimeoutRef.current) {
        clearTimeout(retryTimeoutRef.current);
        retryTimeoutRef.current = null;
      }
    };

    const connect = () => {
      if (disposed || eventSourceRef.current) return;

      const url = `${API_BASE_URL}/admin/notifications/stream`;
      const es = new EventSource(url, { withCredentials: true });
      eventSourceRef.current = es;

      es.addEventListener("unread_count", (e: MessageEvent) => {
        const data = JSON.parse(e.data) as { count: number };
        setUnreadCount(data.count);
      });

      es.addEventListener("notification", () => {
        queryClient.invalidateQueries({ queryKey: ["notifications"] });
      });

      es.addEventListener("unread_count_changed", (e: MessageEvent) => {
        const data = JSON.parse(e.data) as { count: number };
        setUnreadCount(data.count);
      });

      es.onopen = () => {
        if (disposed) return;
        setIsConnected(true);
        retryDelayRef.current = 1000;
      };

      es.onerror = () => {
        if (disposed) return;
        setIsConnected(false);
        es.close();
        eventSourceRef.current = null;
        const delay = Math.min(retryDelayRef.current, 30000);
        retryDelayRef.current = delay * 2;
        clearRetry();
        retryTimeoutRef.current = setTimeout(connect, delay);
      };
    };

    inAppNotificationApi.getUnreadCount().then(
      (resp) => setUnreadCount(resp.count),
      () => { /* ignore errors on initial fetch */ },
    );

    connect();

    return () => {
      disposed = true;
      clearRetry();
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
        eventSourceRef.current = null;
      }
      setIsConnected(false);
    };
  }, [isAuthenticated, queryClient]);

  return { unreadCount, isConnected };
}
