"use client";

import { useQuery } from "@tanstack/react-query";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { settingsApi, vmApi, VM } from "@/lib/api-client";
import { useAuth } from "@/lib/auth-context";
import { RequireAuth } from "@/lib/require-auth";
import { shouldEnableSettingsQueries } from "@/lib/settings-auth";
import { User, Shield, Key, Webhook, Bell } from "lucide-react";
import { ProfileTab } from "@/components/settings/ProfileTab";
import { SecurityTab } from "@/components/settings/SecurityTab";
import { ApiKeysTab } from "@/components/settings/ApiKeysTab";
import { WebhooksTab } from "@/components/settings/WebhooksTab";
import { NotificationsTab } from "@/components/settings/NotificationsTab";

export default function SettingsPage() {
  const {
    isAuthenticated,
    isLoading: isAuthLoading,
    hasBootstrapError,
  } = useAuth();
  const settingsQueriesEnabled = shouldEnableSettingsQueries({
    isAuthenticated,
    isLoading: isAuthLoading,
    hasBootstrapError,
  });

  const { data: profile, isLoading: profileLoading } = useQuery({
    queryKey: ["profile"],
    queryFn: () => settingsApi.getProfile(),
    enabled: settingsQueriesEnabled,
  });

  const { data: apiKeys, isLoading: apiKeysLoading } = useQuery({
    queryKey: ["api-keys"],
    queryFn: () => settingsApi.getApiKeys(),
    enabled: settingsQueriesEnabled,
  });

  const { data: vms, isLoading: vmsLoading } = useQuery({
    queryKey: ["vms", "api-key-scope"],
    queryFn: async () => {
      const perPage = 100;
      let cursor: string | undefined;
      const allVms: VM[] = [];
      let hasMore = true;

      while (hasMore) {
        const pageResponse = await vmApi.getVMs({ perPage, cursor });
        const pageVms = pageResponse.data || [];
        allVms.push(...pageVms);
        hasMore = pageResponse.meta?.has_more ?? false;
        cursor = pageResponse.meta?.next_cursor;
      }

      return allVms;
    },
    enabled: settingsQueriesEnabled,
  });

  const { data: webhooks, isLoading: webhooksLoading } = useQuery({
    queryKey: ["webhooks"],
    queryFn: () => settingsApi.getWebhooks(),
    enabled: settingsQueriesEnabled,
  });

  const { data: twoFactorStatus, isLoading: twoFactorLoading } = useQuery({
    queryKey: ["2fa-status"],
    queryFn: () => settingsApi.get2FAStatus(),
    enabled: settingsQueriesEnabled,
  });

  return (
    <RequireAuth>
      <div className="container mx-auto py-8 px-4 md:px-6 lg:px-8">
        <div className="mb-8">
          <h1 className="text-3xl font-bold tracking-tight">Account Settings</h1>
          <p className="text-muted-foreground mt-2">
            Manage your account settings, security, and integrations
          </p>
        </div>

        <Tabs defaultValue="profile" className="space-y-6">
          <TabsList className="grid w-full max-w-md grid-cols-3 lg:max-w-xl">
            <TabsTrigger value="profile" className="gap-2">
              <User className="h-4 w-4" />
              Profile
            </TabsTrigger>
            <TabsTrigger value="security" className="gap-2">
              <Shield className="h-4 w-4" />
              Security
            </TabsTrigger>
            <TabsTrigger value="notifications" className="gap-2">
              <Bell className="h-4 w-4" />
              Notifications
            </TabsTrigger>
            <TabsTrigger value="api-keys" className="gap-2">
              <Key className="h-4 w-4" />
              API Keys
            </TabsTrigger>
            <TabsTrigger value="webhooks" className="gap-2">
              <Webhook className="h-4 w-4" />
              Webhooks
            </TabsTrigger>
          </TabsList>

          {/* Profile Tab */}
          <TabsContent value="profile" className="space-y-6">
            <ProfileTab
              profile={profile}
              isLoading={profileLoading}
            />
          </TabsContent>

          {/* Security Tab */}
          <TabsContent value="security" className="space-y-6">
            <SecurityTab
              twoFactorStatus={twoFactorStatus}
              isLoading={twoFactorLoading}
            />
          </TabsContent>

          {/* Notifications Tab */}
          <TabsContent value="notifications" className="space-y-6">
            <NotificationsTab />
          </TabsContent>

          {/* API Keys Tab */}
          <TabsContent value="api-keys" className="space-y-6">
            <ApiKeysTab
              apiKeys={apiKeys}
              vms={vms}
              isLoading={apiKeysLoading}
              isVMsLoading={vmsLoading}
            />
          </TabsContent>

          {/* Webhooks Tab */}
          <TabsContent value="webhooks" className="space-y-6">
            <WebhooksTab
              webhooks={webhooks}
              isLoading={webhooksLoading}
            />
          </TabsContent>
        </Tabs>
      </div>
    </RequireAuth>
  );
}
