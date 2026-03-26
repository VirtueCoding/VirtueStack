"use client";

import { useEffect, useState } from "react";
import { useForm, useWatch } from "react-hook-form";
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
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { settingsApi, ApiKey, VM } from "@/lib/api-client";
import { useMutationToast } from "@/lib/utils/toast-helpers";
import { Key, Calendar, Loader2, Trash2, RefreshCw, Plus, Copy, Check, Globe, Server } from "lucide-react";

const apiKeySchema = z.object({
  name: z.string().min(1, "Name is required"),
  permissions: z.array(z.string()).min(1, "At least one permission is required"),
  allowed_ips: z.string().optional(),
  restrict_vm_scope: z.boolean(),
  vm_ids: z.array(z.string()),
  expires_at: z.string().optional(),
}).superRefine((data, ctx) => {
  if (data.restrict_vm_scope && data.vm_ids.length === 0) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      path: ["vm_ids"],
      message: "Select at least one VM to scope this key",
    });
  }
});

type ApiKeyFormData = z.infer<typeof apiKeySchema>;

const AVAILABLE_PERMISSIONS = [
  { value: "vm:read", label: "vm:read — view VM details, metrics, bandwidth, and IPs" },
  { value: "vm:write", label: "vm:write — manage rDNS and ISO operations" },
  { value: "vm:power", label: "vm:power — start, stop, restart, and request console access" },
  { value: "backup:read", label: "backup:read — view backup inventory" },
  { value: "backup:write", label: "backup:write — create, restore, and remove backups" },
  { value: "snapshot:read", label: "snapshot:read — view snapshot inventory" },
  { value: "snapshot:write", label: "snapshot:write — create, restore, and remove snapshots" },
];

const MAX_VM_NAMES_TO_DISPLAY = 2;

interface ApiKeysTabProps {
  apiKeys: ApiKey[] | null | undefined;
  vms: VM[] | null | undefined;
  isLoading: boolean;
  isVMsLoading: boolean;
}

function formatVMLabel(vm: VM) {
  return vm.name || vm.hostname;
}

function describeVMScope(vmIDs: string[] | undefined, vms: VM[] | null | undefined) {
  if (!vmIDs || vmIDs.length === 0) {
    return "All VMs";
  }

  const names = vmIDs
    .map((vmID) => vms?.find((vm) => vm.id === vmID))
    .filter((vm): vm is VM => Boolean(vm))
    .map(formatVMLabel);

  if (
    names.length === vmIDs.length &&
    names.length > 0 &&
    names.length <= MAX_VM_NAMES_TO_DISPLAY
  ) {
    return names.join(", ");
  }

  return `${vmIDs.length} selected VM${vmIDs.length === 1 ? "" : "s"}`;
}

export function ApiKeysTab({ apiKeys, vms, isLoading, isVMsLoading }: ApiKeysTabProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const { createMutationOnError } = useMutationToast();
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [apiKeyDialogOpen, setApiKeyDialogOpen] = useState(false);
  const [deleteKeyDialogOpen, setDeleteKeyDialogOpen] = useState(false);
  const [rotateKeyDialogOpen, setRotateKeyDialogOpen] = useState(false);
  const [selectedKeyId, setSelectedKeyId] = useState<string | null>(null);
  const [rotatedKeyValue, setRotatedKeyValue] = useState<string | null>(null);
  const [rotatedKeyDialogOpen, setRotatedKeyDialogOpen] = useState(false);
  const [createdKeyValue, setCreatedKeyValue] = useState<string | null>(null);
  const [createdKeyDialogOpen, setCreatedKeyDialogOpen] = useState(false);
  const [currentTime, setCurrentTime] = useState(() => Date.now());

  const apiKeyForm = useForm<ApiKeyFormData>({
    resolver: zodResolver(apiKeySchema),
    defaultValues: {
      name: "",
      permissions: [],
      allowed_ips: "",
      restrict_vm_scope: false,
      vm_ids: [],
      expires_at: "",
    },
  });

  useEffect(() => {
    const intervalId = window.setInterval(() => {
      setCurrentTime(Date.now());
    }, 60_000);

    return () => window.clearInterval(intervalId);
  }, []);

  const createApiKeyMutation = useMutation({
    mutationFn: settingsApi.createApiKey,
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ["api-keys"] });
      setApiKeyDialogOpen(false);
      apiKeyForm.reset();
      if (data.key) {
        setCreatedKeyValue(data.key);
        setCreatedKeyDialogOpen(true);
      } else {
        toast({
          title: "Success",
          description: "API key created successfully",
        });
      }
    },
    onError: createMutationOnError("Failed to create API key"),
  });

  const rotateApiKeyMutation = useMutation({
    mutationFn: settingsApi.rotateApiKey,
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ["api-keys"] });
      if (data.key) {
        setRotatedKeyValue(data.key);
        setRotatedKeyDialogOpen(true);
      } else {
        toast({
          title: "Success",
          description: "API key rotated successfully",
        });
      }
    },
    onError: createMutationOnError("Failed to rotate API key"),
  });

  const deleteApiKeyMutation = useMutation({
    mutationFn: settingsApi.deleteApiKey,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["api-keys"] });
      setDeleteKeyDialogOpen(false);
      setSelectedKeyId(null);
      toast({
        title: "Success",
        description: "API key revoked successfully",
      });
    },
    onError: createMutationOnError("Failed to delete API key"),
  });

  const handleCopy = async (text: string, id: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopiedId(id);
      setTimeout(() => setCopiedId(null), 2000);
      toast({
        title: "Copied",
        description: "Copied to clipboard",
      });
    } catch {
      toast({
        title: "Copy failed",
        description: "Unable to copy to clipboard",
        variant: "destructive",
      });
    }
  };

  const handleCreateApiKey = (data: ApiKeyFormData) => {
    // Parse allowed_ips from textarea (one per line)
    const allowedIps = data.allowed_ips
      ? data.allowed_ips.split("\n").map(ip => ip.trim()).filter(ip => ip.length > 0)
      : undefined;

    createApiKeyMutation.mutate({
      name: data.name,
      permissions: data.permissions,
      allowed_ips: allowedIps,
      vm_ids: data.restrict_vm_scope ? data.vm_ids : undefined,
      expires_at: data.expires_at ? new Date(data.expires_at).toISOString() : undefined,
    });
  };

  const restrictVMScope = useWatch({ control: apiKeyForm.control, name: "restrict_vm_scope" });
  const selectedVMIDs = useWatch({ control: apiKeyForm.control, name: "vm_ids" }) || [];

  const handleRestrictVMScopeChange = (checked: boolean) => {
    apiKeyForm.setValue("restrict_vm_scope", checked, { shouldValidate: true });
    if (!checked) {
      apiKeyForm.setValue("vm_ids", [], { shouldValidate: true });
    }
  };

  const handleScopedVMToggle = (vmID: string, checked: boolean) => {
    const currentVMIDs = apiKeyForm.getValues("vm_ids");
    const nextVMIDs = checked
      ? [...currentVMIDs, vmID]
      : currentVMIDs.filter((currentID) => currentID !== vmID);
    apiKeyForm.setValue("vm_ids", nextVMIDs, { shouldValidate: true });
  };

  const handleRotateKey = (keyId: string) => {
    setSelectedKeyId(keyId);
    setRotateKeyDialogOpen(true);
  };

  const confirmRotateKey = () => {
    if (selectedKeyId) {
      rotateApiKeyMutation.mutate(selectedKeyId);
      setRotateKeyDialogOpen(false);
      setSelectedKeyId(null);
    }
  };

  const handleDeleteKey = (keyId: string) => {
    setSelectedKeyId(keyId);
    setDeleteKeyDialogOpen(true);
  };

  const confirmDeleteKey = () => {
    if (selectedKeyId) {
      deleteApiKeyMutation.mutate(selectedKeyId);
    }
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
                <Key className="h-5 w-5" />
                API Keys
              </CardTitle>
              <CardDescription>
                Manage your API keys for programmatic access
              </CardDescription>
            </div>
            <Button size="sm" onClick={() => setApiKeyDialogOpen(true)}>
              <Plus className="mr-2 h-4 w-4" />
              Create New Key
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {!apiKeys || apiKeys.length === 0 ? (
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
                      {apiKey.is_active ? (
                        <Badge variant="default">Active</Badge>
                      ) : (
                        <Badge variant="secondary">Inactive</Badge>
                      )}
                      {apiKey.expires_at && new Date(apiKey.expires_at).getTime() <= currentTime && (
                        <Badge variant="secondary">Expired</Badge>
                      )}
                    </div>
                    <div className="flex flex-wrap items-center gap-4 text-sm text-muted-foreground">
                      <div className="flex items-center gap-1">
                        <Calendar className="h-3 w-3" />
                        Created: {new Date(apiKey.created_at).toLocaleDateString()}
                      </div>
                      <div className="flex items-center gap-1">
                        <Calendar className="h-3 w-3" />
                        Last used: {apiKey.last_used_at ? new Date(apiKey.last_used_at).toLocaleDateString() : 'Never'}
                      </div>
                      {apiKey.allowed_ips && apiKey.allowed_ips.length > 0 && (
                        <div className="flex items-center gap-1">
                          <Globe className="h-3 w-3" />
                          IPs: {apiKey.allowed_ips.length} whitelisted
                        </div>
                      )}
                      <div className="flex items-center gap-1">
                        <Server className="h-3 w-3" />
                        Scope: {describeVMScope(apiKey.vm_ids, vms)}
                      </div>
                      {apiKey.expires_at && (
                        <div className="flex items-center gap-1">
                          <Calendar className="h-3 w-3" />
                          Expires: {new Date(apiKey.expires_at).toLocaleDateString()}
                        </div>
                      )}
                    </div>
                    {apiKey.key && (
                      <div className="flex items-center gap-2">
                        <code className="rounded bg-muted px-2 py-1 text-sm font-mono">
                          {apiKey.key}
                        </code>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-8 w-8"
                          onClick={() => handleCopy(apiKey.key!, apiKey.id)}
                        >
                          {copiedId === apiKey.id ? (
                            <Check className="h-4 w-4" />
                          ) : (
                            <Copy className="h-4 w-4" />
                          )}
                        </Button>
                      </div>
                    )}
                  </div>
                  <div className="flex gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleRotateKey(apiKey.id)}
                      disabled={rotateApiKeyMutation.isPending}
                    >
                      <RefreshCw className="mr-2 h-4 w-4" />
                      Regenerate
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      className="text-destructive hover:bg-destructive hover:text-destructive-foreground"
                      onClick={() => handleDeleteKey(apiKey.id)}
                    >
                      <Trash2 className="mr-2 h-4 w-4" />
                      Revoke
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* API Key Dialog */}
      <Dialog open={apiKeyDialogOpen} onOpenChange={setApiKeyDialogOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Create API Key</DialogTitle>
            <DialogDescription>
              Create a new API key for programmatic access to your account.
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={apiKeyForm.handleSubmit(handleCreateApiKey)} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="key-name">Name</Label>
              <Input
                id="key-name"
                placeholder="e.g., Production API Key"
                {...apiKeyForm.register("name")}
              />
              {apiKeyForm.formState.errors.name && (
                <p className="text-sm text-destructive">{apiKeyForm.formState.errors.name.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label>Permissions</Label>
              <div className="space-y-2 border rounded-md p-3">
                {AVAILABLE_PERMISSIONS.map((permission) => (
                  <div key={permission.value} className="flex items-center space-x-2">
                    <input
                      type="checkbox"
                      id={permission.value}
                      value={permission.value}
                      {...apiKeyForm.register("permissions")}
                      className="rounded border-gray-300"
                    />
                    <Label htmlFor={permission.value} className="text-sm font-normal cursor-pointer">
                      {permission.label}
                    </Label>
                  </div>
                ))}
              </div>
              {apiKeyForm.formState.errors.permissions && (
                <p className="text-sm text-destructive">{apiKeyForm.formState.errors.permissions.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="allowed-ips">IP Whitelist (Optional)</Label>
              <textarea
                id="allowed-ips"
                className="flex min-h-[80px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
                placeholder="192.168.1.1&#10;10.0.0.0/24&#10;2001:db8::/32"
                rows={3}
                {...apiKeyForm.register("allowed_ips")}
              />
              <p className="text-xs text-muted-foreground">
                One IP address or CIDR range per line. Leave empty to allow all IPs.
              </p>
            </div>
            <div className="space-y-2">
              <Label className="flex items-center gap-2">
                <Server className="h-4 w-4" />
                VM Scope (Optional)
              </Label>
              <div className="space-y-3 rounded-md border p-3">
                <label className="flex items-center gap-2 text-sm font-medium">
                  <input
                    type="checkbox"
                    checked={restrictVMScope}
                    onChange={(event) => handleRestrictVMScopeChange(event.target.checked)}
                    className="rounded border-gray-300"
                  />
                  Restrict this key to selected VMs only
                </label>
                <p className="text-xs text-muted-foreground">
                  Leave this disabled to allow the key to access every VM on your account.
                </p>
                {restrictVMScope && (
                  <div className="space-y-2">
                    {isVMsLoading ? (
                      <div className="flex items-center gap-2 rounded-md border border-dashed p-3 text-sm text-muted-foreground">
                        <Loader2 className="h-4 w-4 animate-spin" />
                        Loading your VMs…
                      </div>
                    ) : !vms || vms.length === 0 ? (
                      <div className="rounded-md border border-dashed p-3 text-sm text-muted-foreground">
                        No VMs are available to scope yet.
                      </div>
                    ) : (
                      <div className="max-h-48 space-y-2 overflow-y-auto rounded-md border p-2">
                        {vms.map((vm) => (
                          <label
                            key={vm.id}
                            className="flex items-start gap-2 rounded-md p-2 text-sm hover:bg-muted/50"
                          >
                            <input
                              type="checkbox"
                              checked={selectedVMIDs.includes(vm.id)}
                              onChange={(event) => handleScopedVMToggle(vm.id, event.target.checked)}
                              className="mt-0.5 rounded border-gray-300"
                            />
                            <span className="space-y-1">
                              <span className="block font-medium">{formatVMLabel(vm)}</span>
                              <span className="block text-xs text-muted-foreground">
                                {vm.hostname}
                              </span>
                            </span>
                          </label>
                        ))}
                      </div>
                    )}
                    {apiKeyForm.formState.errors.vm_ids && (
                      <p className="text-sm text-destructive">{apiKeyForm.formState.errors.vm_ids.message}</p>
                    )}
                  </div>
                )}
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="expires-at">Expiration (Optional)</Label>
              <Input
                id="expires-at"
                type="datetime-local"
                {...apiKeyForm.register("expires_at")}
              />
              <p className="text-xs text-muted-foreground">
                Leave empty to keep the key valid until it is manually revoked.
              </p>
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setApiKeyDialogOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={createApiKeyMutation.isPending}>
                {createApiKeyMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Create Key
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Delete API Key Dialog */}
      <Dialog open={deleteKeyDialogOpen} onOpenChange={setDeleteKeyDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Revoke API Key</DialogTitle>
            <DialogDescription>
              Are you sure you want to revoke this API key? It will stop working immediately and cannot be used again.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setDeleteKeyDialogOpen(false)}>
              Cancel
            </Button>
            <Button
              type="button"
              variant="destructive"
              onClick={confirmDeleteKey}
              disabled={deleteApiKeyMutation.isPending}
            >
              {deleteApiKeyMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Revoke
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={createdKeyDialogOpen} onOpenChange={(open) => {
        if (!open) {
          setCreatedKeyDialogOpen(false);
          setCreatedKeyValue(null);
        }
      }}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>API Key Created</DialogTitle>
            <DialogDescription>
              Your new API key is shown below. Copy it now — it will not be displayed again.
            </DialogDescription>
          </DialogHeader>
          <div className="flex items-center gap-2 rounded-md border bg-muted p-3">
            <code className="flex-1 break-all text-sm font-mono">
              {createdKeyValue}
            </code>
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8 shrink-0"
              onClick={() => createdKeyValue && handleCopy(createdKeyValue, "created")}
            >
              {copiedId === "created" ? (
                <Check className="h-4 w-4" />
              ) : (
                <Copy className="h-4 w-4" />
              )}
            </Button>
          </div>
          <DialogFooter>
            <Button type="button" onClick={() => {
              setCreatedKeyDialogOpen(false);
              setCreatedKeyValue(null);
            }}>
              Done
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={rotateKeyDialogOpen} onOpenChange={setRotateKeyDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Rotate API Key</DialogTitle>
            <DialogDescription>
              Are you sure you want to rotate this API key? The old key will stop working immediately.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setRotateKeyDialogOpen(false)}>
              Cancel
            </Button>
            <Button
              type="button"
              onClick={confirmRotateKey}
              disabled={rotateApiKeyMutation.isPending}
            >
              {rotateApiKeyMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Rotate
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* New Key Value Dialog — shown once after rotation */}
      <Dialog open={rotatedKeyDialogOpen} onOpenChange={(open) => {
        if (!open) {
          setRotatedKeyDialogOpen(false);
          setRotatedKeyValue(null);
        }
      }}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>API Key Rotated</DialogTitle>
            <DialogDescription>
              Your new API key is shown below. Copy it now — it will not be displayed again.
            </DialogDescription>
          </DialogHeader>
          <div className="flex items-center gap-2 rounded-md border bg-muted p-3">
            <code className="flex-1 break-all text-sm font-mono">
              {rotatedKeyValue}
            </code>
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8 shrink-0"
              onClick={() => rotatedKeyValue && handleCopy(rotatedKeyValue, "rotated")}
            >
              {copiedId === "rotated" ? (
                <Check className="h-4 w-4" />
              ) : (
                <Copy className="h-4 w-4" />
              )}
            </Button>
          </div>
          <DialogFooter>
            <Button type="button" onClick={() => {
              setRotatedKeyDialogOpen(false);
              setRotatedKeyValue(null);
            }}>
              Done
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
