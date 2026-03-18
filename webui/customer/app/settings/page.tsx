"use client";

import { useQuery } from "@tanstack/react-query";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { settingsApi } from "@/lib/api-client";
import { RequireAuth } from "@/lib/require-auth";
import { User, Shield, Key, Webhook } from "lucide-react";
import { ProfileTab } from "@/components/settings/ProfileTab";
import { SecurityTab } from "@/components/settings/SecurityTab";
import { ApiKeysTab } from "@/components/settings/ApiKeysTab";
import { WebhooksTab } from "@/components/settings/WebhooksTab";

export default function SettingsPage() {
  const { data: profile, isLoading: profileLoading } = useQuery({
    queryKey: ["profile"],
    queryFn: () => settingsApi.getProfile(),
  });

  const { data: apiKeys, isLoading: apiKeysLoading } = useQuery({
    queryKey: ["api-keys"],
    queryFn: () => settingsApi.getApiKeys(),
  });

  const { data: webhooks, isLoading: webhooksLoading } = useQuery({
    queryKey: ["webhooks"],
    queryFn: () => settingsApi.getWebhooks(),
  });

  const { data: twoFactorStatus, isLoading: twoFactorLoading } = useQuery({
    queryKey: ["2fa-status"],
    queryFn: () => settingsApi.get2FAStatus(),
  });

  const { data: backupCodesData } = useQuery({
    queryKey: ["backup-codes"],
    queryFn: () => settingsApi.getBackupCodes(),
    enabled: twoFactorStatus?.enabled || false,
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
          <TabsList className="grid w-full max-w-md grid-cols-2 lg:max-w-lg">
            <TabsTrigger value="profile" className="gap-2">
              <User className="h-4 w-4" />
              Profile
            </TabsTrigger>
            <TabsTrigger value="security" className="gap-2">
              <Shield className="h-4 w-4" />
              Security
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
              backupCodesData={backupCodesData}
              isLoading={twoFactorLoading}
            />
          </TabsContent>

          {/* API Keys Tab */}
          <TabsContent value="api-keys" className="space-y-6">
            <ApiKeysTab
              apiKeys={apiKeys}
              isLoading={apiKeysLoading}
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