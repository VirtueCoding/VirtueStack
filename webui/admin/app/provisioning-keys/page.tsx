"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Copy, KeyRound, Loader2, Pencil, Plus, RefreshCw, Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { useToast } from "@/components/ui/use-toast";
import {
  adminAuthApi,
  adminProvisioningKeysApi,
  type AdminProvisioningKey,
  type ProvisioningKeySecretResponse,
} from "@/lib/api-client";

type ProvisioningKeyFormState = {
  name: string;
  description: string;
  allowedIPs: string;
  expiresAt: string;
};

const emptyFormState: ProvisioningKeyFormState = {
  name: "",
  description: "",
  allowedIPs: "",
  expiresAt: "",
};

function formatDateTime(value?: string): string {
  if (!value) return "—";
  return new Date(value).toLocaleString();
}

function toDateTimeLocal(value?: string): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";

  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

function toISOStringOrUndefined(value: string): string | undefined {
  if (!value) return undefined;
  return new Date(value).toISOString();
}

function splitAllowedIPs(value: string): string[] {
  return value
    .split(/[\n,]+/)
    .map((entry) => entry.trim())
    .filter(Boolean);
}

function joinAllowedIPs(values?: string[]): string {
  return values?.join("\n") || "";
}

function getProvisioningKeyStatus(key: AdminProvisioningKey): {
  label: string;
  variant: "success" | "warning" | "destructive" | "secondary";
} {
  if (key.revoked_at) {
    return { label: "revoked", variant: "secondary" };
  }

  if (key.expires_at && new Date(key.expires_at).getTime() <= Date.now()) {
    return { label: "expired", variant: "warning" };
  }

  return { label: "active", variant: "success" };
}

export default function ProvisioningKeysPage() {
  const { toast } = useToast();
  const [keys, setKeys] = useState<AdminProvisioningKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [includeRevoked, setIncludeRevoked] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [revokeOpen, setRevokeOpen] = useState(false);
  const [secretOpen, setSecretOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [selectedKey, setSelectedKey] = useState<AdminProvisioningKey | null>(null);
  const [createdSecret, setCreatedSecret] = useState<ProvisioningKeySecretResponse | null>(null);
  const [reauthPassword, setReauthPassword] = useState("");
  const [form, setForm] = useState<ProvisioningKeyFormState>(emptyFormState);

  const loadKeys = useCallback(async () => {
    try {
      const data = await adminProvisioningKeysApi.getProvisioningKeys(includeRevoked);
      setKeys(data || []);
    } catch (error) {
      toast({
        title: "Error",
        description: error instanceof Error ? error.message : "Failed to load provisioning keys.",
        variant: "destructive",
      });
    } finally {
      setLoading(false);
    }
  }, [includeRevoked, toast]);

  useEffect(() => {
    loadKeys();
  }, [loadKeys]);

  const activeKeyCount = useMemo(
    () => keys.filter((key) => getProvisioningKeyStatus(key).label === "active").length,
    [keys],
  );

  const updateForm = (field: keyof ProvisioningKeyFormState, value: string) => {
    setForm((current) => ({ ...current, [field]: value }));
  };

  const resetDialogs = () => {
    setForm(emptyFormState);
    setSelectedKey(null);
    setReauthPassword("");
  };

  const openCreateDialog = () => {
    resetDialogs();
    setCreateOpen(true);
  };

  const openEditDialog = (key: AdminProvisioningKey) => {
    setSelectedKey(key);
    setForm({
      name: key.name,
      description: key.description || "",
      allowedIPs: joinAllowedIPs(key.allowed_ips),
      expiresAt: toDateTimeLocal(key.expires_at),
    });
    setEditOpen(true);
  };

  const openRevokeDialog = (key: AdminProvisioningKey) => {
    setSelectedKey(key);
    setReauthPassword("");
    setRevokeOpen(true);
  };

  const handleCopySecret = async () => {
    if (!createdSecret) return;

    try {
      await navigator.clipboard.writeText(createdSecret.key);
      toast({ title: "Copied", description: "Provisioning key copied to clipboard." });
    } catch {
      toast({
        title: "Copy Failed",
        description: "Copy the provisioning key manually before closing this dialog.",
        variant: "destructive",
      });
    }
  };

  const handleCreate = async () => {
    if (!form.name.trim()) {
      toast({
        title: "Validation Error",
        description: "Name is required.",
        variant: "destructive",
      });
      return;
    }

    setSubmitting(true);
    try {
      const response = await adminProvisioningKeysApi.createProvisioningKey({
        name: form.name.trim(),
        description: form.description.trim() || undefined,
        allowed_ips: splitAllowedIPs(form.allowedIPs),
        expires_at: toISOStringOrUndefined(form.expiresAt),
      });

      toast({
        title: "Provisioning Key Created",
        description: `${response.name} is ready for WHMCS or other provisioning clients.`,
      });
      setCreateOpen(false);
      setCreatedSecret(response);
      setSecretOpen(true);
      resetDialogs();
      await loadKeys();
    } catch (error) {
      toast({
        title: "Create Failed",
        description: error instanceof Error ? error.message : "Failed to create provisioning key.",
        variant: "destructive",
      });
    } finally {
      setSubmitting(false);
    }
  };

  const handleUpdate = async () => {
    if (!selectedKey) return;
    if (!form.name.trim()) {
      toast({
        title: "Validation Error",
        description: "Name is required.",
        variant: "destructive",
      });
      return;
    }

    setSubmitting(true);
    try {
      await adminProvisioningKeysApi.updateProvisioningKey(selectedKey.id, {
        name: form.name.trim(),
        description: form.description.trim(),
        allowed_ips: splitAllowedIPs(form.allowedIPs),
        expires_at: form.expiresAt ? new Date(form.expiresAt).toISOString() : null,
      });

      toast({
        title: "Provisioning Key Updated",
        description: `${form.name.trim()} has been updated.`,
      });
      setEditOpen(false);
      resetDialogs();
      await loadKeys();
    } catch (error) {
      toast({
        title: "Update Failed",
        description: error instanceof Error ? error.message : "Failed to update provisioning key.",
        variant: "destructive",
      });
    } finally {
      setSubmitting(false);
    }
  };

  const handleRevoke = async () => {
    if (!selectedKey) return;
    if (!reauthPassword.trim()) {
      toast({
        title: "Password Required",
        description: "Enter your password to revoke a provisioning key.",
        variant: "destructive",
      });
      return;
    }

    setSubmitting(true);
    try {
      const { reauth_token } = await adminAuthApi.reauth(reauthPassword);
      await adminProvisioningKeysApi.revokeProvisioningKey(selectedKey.id, reauth_token);
      toast({
        title: "Provisioning Key Revoked",
        description: `${selectedKey.name} can no longer authenticate provisioning API requests.`,
      });
      setRevokeOpen(false);
      resetDialogs();
      await loadKeys();
    } catch (error) {
      toast({
        title: "Revoke Failed",
        description: error instanceof Error ? error.message : "Failed to revoke provisioning key.",
        variant: "destructive",
      });
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="min-h-screen bg-background p-6 md:p-8">
      <div className="mx-auto max-w-7xl space-y-8">
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">Provisioning Keys</h1>
            <p className="text-muted-foreground">
              Issue and manage long-lived API credentials for WHMCS and other provisioning integrations.
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" onClick={() => setIncludeRevoked((value) => !value)}>
              <RefreshCw className="mr-2 h-4 w-4" />
              {includeRevoked ? "Hide Revoked" : "Show Revoked"}
            </Button>
            <Button onClick={openCreateDialog}>
              <Plus className="mr-2 h-4 w-4" />
              Create Key
            </Button>
          </div>
        </div>

        <div className="grid gap-4 md:grid-cols-3">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-base">What this feature does</CardTitle>
              <CardDescription>
                Provisioning keys authenticate external automation against the provisioning API.
              </CardDescription>
            </CardHeader>
            <CardContent className="text-sm text-muted-foreground">
              They let systems like WHMCS create, suspend, resize, power-control, and terminate customer VMs without using an interactive admin login.
            </CardContent>
          </Card>
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-base">Security model</CardTitle>
              <CardDescription>
                Keys are stored hashed and the plaintext value is only shown once.
              </CardDescription>
            </CardHeader>
            <CardContent className="text-sm text-muted-foreground">
              Allowed IPs can restrict where the key is accepted from, and optional expiry or revoke actions shut off access immediately.
            </CardContent>
          </Card>
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-base">Current state</CardTitle>
              <CardDescription>
                {activeKeyCount} active key{activeKeyCount === 1 ? "" : "s"} visible in this environment.
              </CardDescription>
            </CardHeader>
            <CardContent className="text-sm text-muted-foreground">
              Use separate keys per integration so you can rotate or revoke one client without disrupting others.
            </CardContent>
          </Card>
        </div>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <KeyRound className="h-5 w-5" />
              Provisioning API Credentials
            </CardTitle>
            <CardDescription>
              These credentials are intended for machine-to-machine access to the provisioning API, not interactive admin sessions.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Allowed IPs</TableHead>
                    <TableHead>Last Used</TableHead>
                    <TableHead>Expires</TableHead>
                    <TableHead>Created</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {loading ? (
                    <TableRow>
                      <TableCell colSpan={7} className="h-24 text-center">
                        <div className="flex justify-center">
                          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                        </div>
                      </TableCell>
                    </TableRow>
                  ) : keys.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={7} className="h-24 text-center text-muted-foreground">
                        No provisioning keys found.
                      </TableCell>
                    </TableRow>
                  ) : (
                    keys.map((key) => {
                      const status = getProvisioningKeyStatus(key);

                      return (
                        <TableRow key={key.id}>
                          <TableCell>
                            <div className="font-medium">{key.name}</div>
                            <div className="text-xs text-muted-foreground">{key.description || "No description"}</div>
                          </TableCell>
                          <TableCell>
                            <Badge variant={status.variant} className="capitalize">
                              {status.label}
                            </Badge>
                          </TableCell>
                          <TableCell className="max-w-xs text-sm text-muted-foreground">
                            {key.allowed_ips?.length ? key.allowed_ips.join(", ") : "Any IP"}
                          </TableCell>
                          <TableCell className="text-sm text-muted-foreground">
                            {formatDateTime(key.last_used_at)}
                          </TableCell>
                          <TableCell className="text-sm text-muted-foreground">
                            {formatDateTime(key.expires_at)}
                          </TableCell>
                          <TableCell>
                            <div className="text-sm">{formatDateTime(key.created_at)}</div>
                            <div className="text-xs text-muted-foreground">{key.created_by}</div>
                          </TableCell>
                          <TableCell className="text-right">
                            <div className="flex justify-end gap-2">
                              <Button
                                variant="ghost"
                                size="icon"
                                onClick={() => openEditDialog(key)}
                                title="Edit provisioning key"
                              >
                                <Pencil className="h-4 w-4" />
                                <span className="sr-only">Edit</span>
                              </Button>
                              <Button
                                variant="ghost"
                                size="icon"
                                onClick={() => openRevokeDialog(key)}
                                title="Revoke provisioning key"
                                className="text-destructive hover:text-destructive"
                                disabled={Boolean(key.revoked_at)}
                              >
                                <Trash2 className="h-4 w-4" />
                                <span className="sr-only">Revoke</span>
                              </Button>
                            </div>
                          </TableCell>
                        </TableRow>
                      );
                    })
                  )}
                </TableBody>
              </Table>
            </div>
          </CardContent>
        </Card>

        <Dialog open={createOpen} onOpenChange={setCreateOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Create Provisioning Key</DialogTitle>
              <DialogDescription>
                The plaintext key is shown only once after creation, so copy it into your external system immediately.
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="create-name">Name</Label>
                <Input id="create-name" value={form.name} onChange={(event) => updateForm("name", event.target.value)} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="create-description">Description</Label>
                <Textarea
                  id="create-description"
                  value={form.description}
                  onChange={(event) => updateForm("description", event.target.value)}
                  placeholder="Optional: identify which integration uses this key"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="create-allowed-ips">Allowed IPs</Label>
                <Textarea
                  id="create-allowed-ips"
                  value={form.allowedIPs}
                  onChange={(event) => updateForm("allowedIPs", event.target.value)}
                  placeholder="One IP or CIDR per line. Leave blank to allow any IP."
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="create-expires-at">Expires At</Label>
                <Input
                  id="create-expires-at"
                  type="datetime-local"
                  value={form.expiresAt}
                  onChange={(event) => updateForm("expiresAt", event.target.value)}
                />
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setCreateOpen(false)} disabled={submitting}>
                Cancel
              </Button>
              <Button onClick={handleCreate} disabled={submitting}>
                {submitting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Plus className="mr-2 h-4 w-4" />}
                Create Key
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={editOpen} onOpenChange={setEditOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Edit Provisioning Key</DialogTitle>
              <DialogDescription>
                Update key metadata and restrictions without rotating the secret.
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="edit-name">Name</Label>
                <Input id="edit-name" value={form.name} onChange={(event) => updateForm("name", event.target.value)} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="edit-description">Description</Label>
                <Textarea
                  id="edit-description"
                  value={form.description}
                  onChange={(event) => updateForm("description", event.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="edit-allowed-ips">Allowed IPs</Label>
                <Textarea
                  id="edit-allowed-ips"
                  value={form.allowedIPs}
                  onChange={(event) => updateForm("allowedIPs", event.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="edit-expires-at">Expires At</Label>
                <Input
                  id="edit-expires-at"
                  type="datetime-local"
                  value={form.expiresAt}
                  onChange={(event) => updateForm("expiresAt", event.target.value)}
                />
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setEditOpen(false)} disabled={submitting}>
                Cancel
              </Button>
              <Button onClick={handleUpdate} disabled={submitting}>
                {submitting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Pencil className="mr-2 h-4 w-4" />}
                Save Changes
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={revokeOpen} onOpenChange={setRevokeOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Revoke Provisioning Key</DialogTitle>
              <DialogDescription>
                Revoking a key immediately blocks future provisioning API calls that use it.
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4">
              <div className="rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm text-muted-foreground">
                This action is intended for compromised, expired, or decommissioned integrations and cannot be undone.
              </div>
              <div className="space-y-2">
                <Label htmlFor="reauth-password">Confirm with your password</Label>
                <Input
                  id="reauth-password"
                  type="password"
                  value={reauthPassword}
                  onChange={(event) => setReauthPassword(event.target.value)}
                  placeholder="Enter your admin password"
                />
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setRevokeOpen(false)} disabled={submitting}>
                Cancel
              </Button>
              <Button variant="destructive" onClick={handleRevoke} disabled={submitting}>
                {submitting ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Trash2 className="mr-2 h-4 w-4" />}
                Revoke Key
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={secretOpen} onOpenChange={(open) => {
          setSecretOpen(open);
          if (!open) setCreatedSecret(null);
        }}>
          <DialogContent className="max-w-2xl">
            <DialogHeader>
              <DialogTitle>Copy the Provisioning Key Now</DialogTitle>
              <DialogDescription>
                The plaintext key is not stored and cannot be shown again after this dialog is closed.
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4">
              <div className="rounded-md border border-primary/30 bg-primary/5 p-3 text-sm text-muted-foreground">
                Store this secret in WHMCS or your automation platform immediately. If it is lost, create a new key and revoke this one.
              </div>
              <div className="space-y-2">
                <Label htmlFor="secret-key">Provisioning Key</Label>
                <div className="flex gap-2">
                  <Input id="secret-key" readOnly value={createdSecret?.key || ""} className="font-mono text-xs" />
                  <Button type="button" variant="outline" onClick={handleCopySecret}>
                    <Copy className="mr-2 h-4 w-4" />
                    Copy
                  </Button>
                </div>
              </div>
            </div>
            <DialogFooter>
              <Button onClick={() => setSecretOpen(false)}>Done</Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </div>
  );
}
