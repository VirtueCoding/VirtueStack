"use client";

import { useState, useEffect } from "react";
import { Bell, Mail, MessageCircle, Loader2, Save } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
import { useToast } from "@/components/ui/use-toast";
import { notificationApi, NotificationPreferences, ApiClientError } from "@/lib/api-client";

const EVENT_TYPE_LABELS: Record<string, string> = {
  "vm.created": "VM Created",
  "vm.deleted": "VM Deleted",
  "vm.suspended": "VM Suspended",
  "backup.failed": "Backup Failed",
  "node.offline": "Node Offline",
  "bandwidth.exceeded": "Bandwidth Exceeded",
};

const EVENT_TYPE_DESCRIPTIONS: Record<string, string> = {
  "vm.created": "Get notified when a new VM is created",
  "vm.deleted": "Get notified when a VM is deleted",
  "vm.suspended": "Get notified when a VM is suspended",
  "backup.failed": "Get notified when a backup fails",
  "node.offline": "Get notified when a node goes offline",
  "bandwidth.exceeded": "Get notified when bandwidth limit is exceeded",
};

interface NotificationsTabProps {
  initialPreferences?: NotificationPreferences | null;
}

export function NotificationsTab({ initialPreferences }: NotificationsTabProps) {
  const { toast } = useToast();
  const [preferences, setPreferences] = useState<NotificationPreferences | null>(initialPreferences || null);
  const [isLoading, setIsLoading] = useState(!initialPreferences);
  const [isSaving, setIsSaving] = useState(false);
  const [availableEvents, setAvailableEvents] = useState<string[]>([]);

  useEffect(() => {
    if (!initialPreferences) {
      fetchPreferences();
    }
    fetchEventTypes();
  }, []);

  const fetchPreferences = async () => {
    try {
      setIsLoading(true);
      const data = await notificationApi.getPreferences();
      setPreferences(data);
    } catch (err) {
      toast({
        title: "Error",
        description: err instanceof ApiClientError ? err.message : "Failed to load notification preferences",
        variant: "destructive",
      });
    } finally {
      setIsLoading(false);
    }
  };

  const fetchEventTypes = async () => {
    try {
      const data = await notificationApi.getEventTypes();
      setAvailableEvents(data.events || []);
    } catch {
      // If we can't get event types, use defaults
      setAvailableEvents(Object.keys(EVENT_TYPE_LABELS));
    }
  };

  const handleToggleEmail = async (enabled: boolean) => {
    if (!preferences) return;
    setPreferences({ ...preferences, email_enabled: enabled });
  };

  const handleToggleTelegram = async (enabled: boolean) => {
    if (!preferences) return;
    setPreferences({ ...preferences, telegram_enabled: enabled });
  };

  const handleToggleEvent = (eventType: string, enabled: boolean) => {
    if (!preferences) return;
    const events = enabled
      ? [...preferences.events, eventType]
      : preferences.events.filter((e) => e !== eventType);
    setPreferences({ ...preferences, events });
  };

  const handleSave = async () => {
    if (!preferences) return;
    setIsSaving(true);
    try {
      const updated = await notificationApi.updatePreferences({
        email_enabled: preferences.email_enabled,
        telegram_enabled: preferences.telegram_enabled,
        events: preferences.events,
      });
      setPreferences(updated);
      toast({
        title: "Preferences Saved",
        description: "Your notification preferences have been updated.",
      });
    } catch (err) {
      toast({
        title: "Error",
        description: err instanceof ApiClientError ? err.message : "Failed to save notification preferences",
        variant: "destructive",
      });
    } finally {
      setIsSaving(false);
    }
  };

  if (isLoading) {
    return (
      <div className="flex justify-center p-8">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (!preferences) {
    return (
      <div className="text-center p-8 text-muted-foreground">
        Unable to load notification preferences.
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Bell className="h-5 w-5" />
            Notification Channels
          </CardTitle>
          <CardDescription>
            Choose how you want to receive notifications
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-4">
              <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary/10">
                <Mail className="h-5 w-5 text-primary" />
              </div>
              <div>
                <Label htmlFor="email-notifications" className="font-medium">
                  Email Notifications
                </Label>
                <p className="text-sm text-muted-foreground">
                  Receive notifications via email
                </p>
              </div>
            </div>
            <Switch
              id="email-notifications"
              checked={preferences.email_enabled}
              onCheckedChange={handleToggleEmail}
            />
          </div>

          <div className="flex items-center justify-between">
            <div className="flex items-center gap-4">
              <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary/10">
                <MessageCircle className="h-5 w-5 text-primary" />
              </div>
              <div>
                <Label htmlFor="telegram-notifications" className="font-medium">
                  Telegram Notifications
                </Label>
                <p className="text-sm text-muted-foreground">
                  Receive notifications via Telegram bot
                </p>
              </div>
            </div>
            <Switch
              id="telegram-notifications"
              checked={preferences.telegram_enabled}
              onCheckedChange={handleToggleTelegram}
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Event Notifications</CardTitle>
          <CardDescription>
            Select which events you want to be notified about
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {availableEvents.map((eventType) => {
            const isEnabled = preferences.events.includes(eventType);
            const label = EVENT_TYPE_LABELS[eventType] || eventType;
            const description = EVENT_TYPE_DESCRIPTIONS[eventType] || `Get notified about ${eventType}`;

            return (
              <div key={eventType} className="flex items-center justify-between">
                <div>
                  <Label className="font-medium">{label}</Label>
                  <p className="text-sm text-muted-foreground">{description}</p>
                </div>
                <Switch
                  checked={isEnabled}
                  onCheckedChange={(checked) => handleToggleEvent(eventType, checked)}
                />
              </div>
            );
          })}
        </CardContent>
      </Card>

      <div className="flex justify-end">
        <Button onClick={handleSave} disabled={isSaving}>
          {isSaving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          <Save className="mr-2 h-4 w-4" />
          Save Preferences
        </Button>
      </div>
    </div>
  );
}