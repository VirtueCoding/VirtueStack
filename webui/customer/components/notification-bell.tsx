"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { NotificationBellView } from "@virtuestack/ui";

import { inAppNotificationApi } from "@/lib/api-client";
import { useNotifications } from "@/lib/hooks/use-notifications";

export function NotificationBell() {
  const { unreadCount } = useNotifications();
  const queryClient = useQueryClient();
  const { data, isLoading, refetch } = useQuery({
    queryKey: ["notifications"],
    queryFn: () => inAppNotificationApi.list({ per_page: 20 }),
    enabled: false,
  });

  const invalidateNotifications = async () => {
    await queryClient.invalidateQueries({ queryKey: ["notifications"] });
  };

  return (
    <NotificationBellView
      unreadCount={unreadCount}
      notifications={data?.data ?? []}
      isLoading={isLoading}
      onOpen={() => {
        void refetch();
      }}
      onMarkAsRead={async (id) => {
        await inAppNotificationApi.markAsRead(id);
        await invalidateNotifications();
      }}
      onMarkAllAsRead={async () => {
        await inAppNotificationApi.markAllAsRead();
        await invalidateNotifications();
      }}
    />
  );
}
