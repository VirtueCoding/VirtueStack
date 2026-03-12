"use client";

import { useState, useEffect } from "react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Badge } from "@/components/ui/badge";
import { useToast } from "@/components/ui/use-toast";
import { settingsApi, ApiKey, Webhook as WebhookType } from "@/lib/api-client";
import {
  User,
  Shield,
  Key,
  Webhook,
  Copy,
  Check,
  RefreshCw,
  Play,
  QrCode,
  Mail,
  Lock,
  Smartphone,
  Calendar,
  Loader2,
} from "lucide-react";

export default function SettingsPage() {
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [twoFactorEnabled, setTwoFactorEnabled] = useState(false);
  
  const [apiKeys, setApiKeys] = useState<ApiKey[]>([]);
  const [loadingKeys, setLoadingKeys] = useState(true);
  
  const [webhooks, setWebhooks] = useState<WebhookType[]>([]);
  const [loadingWebhooks, setLoadingWebhooks] = useState(true);

  const { toast } = useToast();

  useEffect(() => {
    async function loadData() {
      try {
        const [keysData, webhooksData] = await Promise.all([
          settingsApi.getApiKeys().catch((err) => {
            console.error(err);
            return [];
          }),
          settingsApi.getWebhooks().catch((err) => {
            console.error(err);
            return [];
          })
        ]);
        setApiKeys(keysData);
        setWebhooks(webhooksData);
      } catch (error) {
        console.error("Failed to load settings data", error);
        toast({
          title: "Error",
          description: "Failed to load settings data.",
          variant: "destructive",
        });
      } finally {
        setLoadingKeys(false);
        setLoadingWebhooks(false);
      }
    }
    loadData();
  }, [toast]);

  const handleCopy = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
  };

  return (
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
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <User className="h-5 w-5" />
                Profile Information
              </CardTitle>
              <CardDescription>
                Update your personal information and email address
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid gap-2">
                <Label htmlFor="name">Full Name</Label>
                <Input
                  id="name"
                  placeholder="Enter your full name"
                  defaultValue="John Doe"
                  className="max-w-md"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="email">Email Address</Label>
                <div className="relative max-w-md">
                  <Mail className="absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
                  <Input
                    id="email"
                    type="email"
                    placeholder="email@example.com"
                    defaultValue="john@example.com"
                    className="pl-10"
                  />
                </div>
              </div>
              <Button className="mt-2">Save Changes</Button>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Lock className="h-5 w-5" />
                Change Password
              </CardTitle>
              <CardDescription>
                Update your password to keep your account secure
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid gap-2">
                <Label htmlFor="current-password">Current Password</Label>
                <Input
                  id="current-password"
                  type="password"
                  className="max-w-md"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="new-password">New Password</Label>
                <Input
                  id="new-password"
                  type="password"
                  className="max-w-md"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="confirm-password">Confirm New Password</Label>
                <Input
                  id="confirm-password"
                  type="password"
                  className="max-w-md"
                />
              </div>
              <Button className="mt-2" variant="secondary">
                Update Password
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        {/* Security Tab */}
        <TabsContent value="security" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Smartphone className="h-5 w-5" />
                Two-Factor Authentication
              </CardTitle>
              <CardDescription>
                Add an extra layer of security to your account
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center justify-between rounded-lg border p-4">
                <div className="space-y-0.5">
                  <Label className="text-base">Enable 2FA</Label>
                  <p className="text-sm text-muted-foreground">
                    Require a verification code when signing in
                  </p>
                </div>
                <Switch
                  checked={twoFactorEnabled}
                  onCheckedChange={setTwoFactorEnabled}
                />
              </div>

              {twoFactorEnabled && (
                <div className="rounded-lg border bg-muted p-6">
                  <div className="flex flex-col items-center space-y-4">
                    <div className="rounded-lg bg-background p-4">
                      <QrCode className="h-32 w-32 text-muted-foreground" />
                    </div>
                    <p className="text-sm text-muted-foreground text-center">
                      Scan this QR code with your authenticator app
                    </p>
                    <div className="flex gap-2">
                      <Button variant="outline" size="sm">
                        <RefreshCw className="mr-2 h-4 w-4" />
                        Regenerate
                      </Button>
                      <Button size="sm">I've Scanned It</Button>
                    </div>
                  </div>
                </div>
              )}

              {!twoFactorEnabled && (
                <div className="rounded-lg border bg-muted p-4">
                  <p className="text-sm text-muted-foreground text-center">
                    Enable 2FA above to see the QR code setup
                  </p>
                </div>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Backup Codes</CardTitle>
              <CardDescription>
                Download your backup codes for account recovery
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="rounded-lg border bg-muted p-4">
                <p className="text-sm text-muted-foreground text-center">
                  Backup codes will be available after enabling 2FA
                </p>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        {/* API Keys Tab */}
        <TabsContent value="api-keys" className="space-y-6">
          <Card>
            <CardHeader>
              <div className="flex items-center justify-between">
                <div>
                  <CardTitle className="flex items-center gap-2">
                    <Key className="h-5 w-5" />
                    API Keys
                  </CardTitle>
                  <CardDescription>
                    Manage your API keys for programmatic access
                  </CardDescription>
                </div>
                <Button size="sm">
                  <Key className="mr-2 h-4 w-4" />
                  Create New Key
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              {loadingKeys ? (
                <div className="flex justify-center p-8">
                  <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
                </div>
              ) : apiKeys.length === 0 ? (
                <div className="text-center p-8 text-muted-foreground">
                  No API keys found. Create one to get started.
                </div>
              ) : (
                <div className="space-y-4">
                  {apiKeys.map((apiKey) => (
                    <div
                      key={apiKey.id}
                      className="flex flex-col gap-4 rounded-lg border p-4 md:flex-row md:items-center md:justify-between"
                    >
                      <div className="space-y-2">
                        <div className="flex items-center gap-2">
                          <span className="font-medium">{apiKey.name}</span>
                          <Badge variant={apiKey.key.includes("live") ? "default" : "secondary"}>
                            {apiKey.key.includes("live") ? "Production" : "Test"}
                          </Badge>
                        </div>
                        <div className="flex items-center gap-4 text-sm text-muted-foreground">
                          <div className="flex items-center gap-1">
                            <Calendar className="h-3 w-3" />
                            Created: {apiKey.created}
                          </div>
                          <div className="flex items-center gap-1">
                            <Calendar className="h-3 w-3" />
                            Last used: {apiKey.lastUsed}
                          </div>
                        </div>
                        <div className="flex items-center gap-2">
                          <code className="rounded bg-muted px-2 py-1 text-sm">
                            {apiKey.key}
                          </code>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-8 w-8"
                            onClick={() => handleCopy(apiKey.key, apiKey.id)}
                          >
                            {copiedId === apiKey.id ? (
                              <Check className="h-4 w-4" />
                            ) : (
                              <Copy className="h-4 w-4" />
                            )}
                          </Button>
                        </div>
                      </div>
                      <div className="flex gap-2">
                        <Button variant="outline" size="sm">
                          <RefreshCw className="mr-2 h-4 w-4" />
                          Regenerate
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* Webhooks Tab */}
        <TabsContent value="webhooks" className="space-y-6">
          <Card>
            <CardHeader>
              <div className="flex items-center justify-between">
                <div>
                  <CardTitle className="flex items-center gap-2">
                    <Webhook className="h-5 w-5" />
                    Webhooks
                  </CardTitle>
                  <CardDescription>
                    Configure webhook endpoints to receive event notifications
                  </CardDescription>
                </div>
                <Button size="sm">
                  <Webhook className="mr-2 h-4 w-4" />
                  Add Webhook
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              {loadingWebhooks ? (
                <div className="flex justify-center p-8">
                  <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
                </div>
              ) : webhooks.length === 0 ? (
                <div className="text-center p-8 text-muted-foreground">
                  No webhooks found. Create one to get started.
                </div>
              ) : (
                <div className="space-y-4">
                  {webhooks.map((webhook) => (
                  <div
                    key={webhook.id}
                    className="flex flex-col gap-4 rounded-lg border p-4 md:flex-row md:items-start md:justify-between"
                  >
                    <div className="space-y-2">
                      <div className="flex items-center gap-2">
                        <span className="font-medium">{webhook.url}</span>
                        <Badge
                          variant={
                            webhook.status === "active" ? "default" : "secondary"
                          }
                        >
                          {webhook.status}
                        </Badge>
                      </div>
                      <div className="flex flex-wrap gap-1">
                        {webhook.events.map((event) => (
                          <Badge key={event} variant="outline" className="text-xs">
                            {event}
                          </Badge>
                        ))}
                      </div>
                      <div className="flex items-center gap-1 text-sm text-muted-foreground">
                        <Calendar className="h-3 w-3" />
                        Last triggered:{" "}
                        {new Date(webhook.lastTriggered).toLocaleString()}
                      </div>
                    </div>
                    <div className="flex gap-2">
                      <Button variant="outline" size="sm">
                        <Play className="mr-2 h-4 w-4" />
                        Test
                      </Button>
                      <Button variant="outline" size="sm">
                        Edit
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
