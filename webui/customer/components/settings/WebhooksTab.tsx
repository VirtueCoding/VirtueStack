"use client";

import { useState, useEffect } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { useToast } from "@/components/ui/use-toast";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { settingsApi, Webhook as WebhookType } from "@/lib/api-client";
import { useMutationToast } from "@/lib/utils/toast-helpers";
import { Webhook, Calendar, Loader2, Trash2, Edit3, Plus, Play } from "lucide-react";

const webhookSchema = z.object({
  url: z.string().url("Invalid URL"),
  events: z.array(z.string()).min(1, "At least one event is required"),
  // secret is required when creating a new webhook, but omitted when editing
  // (the secret field is hidden in the edit form).
  secret: z.string().optional(),
});

type WebhookFormData = z.infer<typeof webhookSchema>;

const AVAILABLE_EVENTS = [
  "vm.created",
  "vm.started",
  "vm.stopped",
  "vm.deleted",
  "vm.reinstalled",
  "vm.migrated",
  "backup.completed",
  "backup.failed",
  "snapshot.created",
  "bandwidth.threshold",
];

interface WebhooksTabProps {
  webhooks: WebhookType[] | null | undefined;
  isLoading: boolean;
}

export function WebhooksTab({ webhooks, isLoading }: WebhooksTabProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const { createMutationOnError } = useMutationToast();
  const [webhookDialogOpen, setWebhookDialogOpen] = useState(false);
  const [editingWebhook, setEditingWebhook] = useState<WebhookType | null>(null);
  const [deleteWebhookDialogOpen, setDeleteWebhookDialogOpen] = useState(false);
  const [selectedWebhookId, setSelectedWebhookId] = useState<string | null>(null);

  const webhookForm = useForm<WebhookFormData>({
    resolver: zodResolver(webhookSchema),
    defaultValues: {
      url: "",
      events: [],
      secret: "",
    },
  });

  useEffect(() => {
    if (editingWebhook) {
      webhookForm.reset({
        url: editingWebhook.url,
        events: editingWebhook.events,
        secret: "",
      });
    } else {
      webhookForm.reset({
        url: "",
        events: [],
        secret: "",
      });
    }
  }, [editingWebhook, webhookForm]);

  const createWebhookMutation = useMutation({
    mutationFn: settingsApi.createWebhook,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["webhooks"] });
      setWebhookDialogOpen(false);
      webhookForm.reset();
      toast({
        title: "Success",
        description: "Webhook created successfully",
      });
    },
    onError: createMutationOnError("Failed to create webhook"),
  });

  const updateWebhookMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: { url?: string; events?: string[]; secret?: string; is_active?: boolean } }) =>
      settingsApi.updateWebhook(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["webhooks"] });
      setWebhookDialogOpen(false);
      setEditingWebhook(null);
      webhookForm.reset();
      toast({
        title: "Success",
        description: "Webhook updated successfully",
      });
    },
    onError: createMutationOnError("Failed to update webhook"),
  });

  const deleteWebhookMutation = useMutation({
    mutationFn: settingsApi.deleteWebhook,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["webhooks"] });
      setDeleteWebhookDialogOpen(false);
      setSelectedWebhookId(null);
      toast({
        title: "Success",
        description: "Webhook deleted successfully",
      });
    },
    onError: createMutationOnError("Failed to delete webhook"),
  });

  const testWebhookMutation = useMutation({
    mutationFn: settingsApi.testWebhook,
    onSuccess: (data) => {
      if (data.success) {
        toast({
          title: "Success",
          description: data.message || "Webhook test queued for delivery",
        });
      } else {
        toast({
          title: "Warning",
          description: `Webhook test failed: ${data.error || "Unknown error"}`,
          variant: "destructive",
        });
      }
      queryClient.invalidateQueries({ queryKey: ["webhooks"] });
    },
    onError: createMutationOnError("Failed to test webhook"),
  });

  const handleCreateWebhook = (data: WebhookFormData) => {
    if (editingWebhook) {
      updateWebhookMutation.mutate({
        id: editingWebhook.id,
        data: {
          url: data.url,
          events: data.events,
        },
      });
    } else {
      createWebhookMutation.mutate({
        url: data.url,
        events: data.events,
        secret: data.secret ?? "",
      });
    }
  };

  const handleEditWebhook = (webhook: WebhookType) => {
    setEditingWebhook(webhook);
    setWebhookDialogOpen(true);
  };

  const handleDeleteWebhook = (webhookId: string) => {
    setSelectedWebhookId(webhookId);
    setDeleteWebhookDialogOpen(true);
  };

  const confirmDeleteWebhook = () => {
    if (selectedWebhookId) {
      deleteWebhookMutation.mutate(selectedWebhookId);
    }
  };

  const handleTestWebhook = (webhookId: string) => {
    testWebhookMutation.mutate(webhookId);
  };

  if (isLoading) {
    return (
      <div className="flex justify-center p-8">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <>
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
            <Button size="sm" onClick={() => { setEditingWebhook(null); setWebhookDialogOpen(true); }}>
              <Plus className="mr-2 h-4 w-4" />
              Add Webhook
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {!webhooks || webhooks.length === 0 ? (
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
                  <div className="space-y-2 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="font-medium truncate">{webhook.url}</span>
                      <Badge
                        variant={webhook.is_active ? "default" : "secondary"}
                      >
                        {webhook.is_active ? "active" : "inactive"}
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
                      Last triggered: {webhook.last_success_at
                        ? new Date(webhook.last_success_at).toLocaleString()
                        : "Never"}
                      {webhook.fail_count > 0 && (
                        <span className="text-destructive ml-2">
                          ({webhook.fail_count} failures)
                        </span>
                      )}
                    </div>
                  </div>
                  <div className="flex gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleTestWebhook(webhook.id)}
                      disabled={testWebhookMutation.isPending}
                    >
                      <Play className="mr-2 h-4 w-4" />
                      Test
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleEditWebhook(webhook)}
                    >
                      <Edit3 className="mr-2 h-4 w-4" />
                      Edit
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      className="text-destructive hover:bg-destructive hover:text-destructive-foreground"
                      onClick={() => handleDeleteWebhook(webhook.id)}
                    >
                      <Trash2 className="mr-2 h-4 w-4" />
                      Delete
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Webhook Dialog */}
      <Dialog open={webhookDialogOpen} onOpenChange={(open) => {
        setWebhookDialogOpen(open);
        if (!open) setEditingWebhook(null);
      }}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{editingWebhook ? "Edit Webhook" : "Add Webhook"}</DialogTitle>
            <DialogDescription>
              {editingWebhook
                ? "Update your webhook configuration."
                : "Configure a new webhook endpoint to receive event notifications."}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={webhookForm.handleSubmit(handleCreateWebhook)} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="webhook-url">Endpoint URL</Label>
              <Input
                id="webhook-url"
                placeholder="https://example.com/webhook"
                {...webhookForm.register("url")}
              />
              {webhookForm.formState.errors.url && (
                <p className="text-sm text-destructive">{webhookForm.formState.errors.url.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="webhook-events">Events</Label>
              <Select
                onValueChange={(value) => {
                  const current = webhookForm.getValues("events") || [];
                  if (!current.includes(value)) {
                    webhookForm.setValue("events", [...current, value], { shouldValidate: true });
                  }
                }}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select events" />
                </SelectTrigger>
                <SelectContent>
                  {AVAILABLE_EVENTS.map((event) => (
                    <SelectItem key={event} value={event}>
                      {event}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <div className="flex flex-wrap gap-1 mt-2">
                {webhookForm.watch("events")?.map((event) => (
                  <Badge key={event} variant="secondary" className="cursor-pointer" onClick={() => {
                    const current = webhookForm.getValues("events") || [];
                    webhookForm.setValue("events", current.filter(e => e !== event), { shouldValidate: true });
                  }}>
                    {event} x
                  </Badge>
                ))}
              </div>
              {webhookForm.formState.errors.events && (
                <p className="text-sm text-destructive">{webhookForm.formState.errors.events.message}</p>
              )}
            </div>
            {!editingWebhook && (
              <div className="space-y-2">
                <Label htmlFor="webhook-secret">Secret</Label>
                <Input
                  id="webhook-secret"
                  type="password"
                  placeholder="Webhook secret for signature verification"
                  {...webhookForm.register("secret")}
                />
                {webhookForm.formState.errors.secret && (
                  <p className="text-sm text-destructive">{webhookForm.formState.errors.secret.message}</p>
                )}
              </div>
            )}
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => { setWebhookDialogOpen(false); setEditingWebhook(null); }}>
                Cancel
              </Button>
              <Button type="submit" disabled={createWebhookMutation.isPending || updateWebhookMutation.isPending}>
                {(createWebhookMutation.isPending || updateWebhookMutation.isPending) && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {editingWebhook ? "Update" : "Add"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Delete Webhook Dialog */}
      <Dialog open={deleteWebhookDialogOpen} onOpenChange={setDeleteWebhookDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Delete Webhook</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete this webhook? This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setDeleteWebhookDialogOpen(false)}>
              Cancel
            </Button>
            <Button
              type="button"
              variant="destructive"
              onClick={confirmDeleteWebhook}
              disabled={deleteWebhookMutation.isPending}
            >
              {deleteWebhookMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}