"use client";

import { useCallback, useEffect, useState } from "react";
import { Network, Loader2, Edit2, Trash2, Globe, Save, X } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@virtuestack/ui";
import { Button } from "@virtuestack/ui";
import { Input } from "@virtuestack/ui";
import { Badge } from "@virtuestack/ui";
import { useToast } from "@virtuestack/ui";
import { rdnsApi, IPAddressRecord, ApiClientError } from "@/lib/api-client";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@virtuestack/ui";

interface VMRDNSTabProps {
  vmId: string;
}

export function VMRDNSTab({ vmId }: VMRDNSTabProps) {
  const { toast } = useToast();
  const [ips, setIPs] = useState<IPAddressRecord[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [editingIPId, setEditingIPId] = useState<string | null>(null);
  const [editHostname, setEditHostname] = useState("");
  const [isSaving, setIsSaving] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [selectedIP, setSelectedIP] = useState<IPAddressRecord | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);

  const fetchIPs = useCallback(async () => {
    try {
      setIsLoading(true);
      const data = await rdnsApi.listIPs(vmId);
      setIPs(data || []);
    } catch (err) {
      toast({
        title: "Error",
        description: err instanceof ApiClientError ? err.message : "Failed to load IP addresses",
        variant: "destructive",
      });
    } finally {
      setIsLoading(false);
    }
  }, [toast, vmId]);

  useEffect(() => {
    void fetchIPs();
  }, [fetchIPs]);

  const handleEdit = (ip: IPAddressRecord) => {
    setEditingIPId(ip.id);
    setEditHostname(ip.rdns_hostname || "");
  };

  const handleCancelEdit = () => {
    setEditingIPId(null);
    setEditHostname("");
  };

  const handleSave = async (ip: IPAddressRecord) => {
    if (!editHostname.trim()) {
      toast({
        title: "Validation Error",
        description: "Hostname cannot be empty",
        variant: "destructive",
      });
      return;
    }

    // Basic hostname validation
    const hostnameRegex = /^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$/;
    if (!hostnameRegex.test(editHostname)) {
      toast({
        title: "Validation Error",
        description: "Invalid hostname format. Use valid DNS hostname (e.g., mail.example.com)",
        variant: "destructive",
      });
      return;
    }

    setIsSaving(true);
    try {
      await rdnsApi.updateRDNS(vmId, ip.id, editHostname);
      toast({
        title: "rDNS Updated",
        description: `PTR record for ${ip.address} has been updated to ${editHostname}`,
      });
      setEditingIPId(null);
      void fetchIPs();
    } catch (err) {
      toast({
        title: "Error",
        description: err instanceof ApiClientError ? err.message : "Failed to update rDNS",
        variant: "destructive",
      });
    } finally {
      setIsSaving(false);
    }
  };

  const handleDelete = (ip: IPAddressRecord) => {
    setSelectedIP(ip);
    setDeleteDialogOpen(true);
  };

  const confirmDelete = async () => {
    if (!selectedIP) return;
    setIsDeleting(true);
    try {
      await rdnsApi.deleteRDNS(vmId, selectedIP.id);
      toast({
        title: "rDNS Removed",
        description: `PTR record for ${selectedIP.address} has been removed`,
      });
      setDeleteDialogOpen(false);
      void fetchIPs();
    } catch (err) {
      toast({
        title: "Error",
        description: err instanceof ApiClientError ? err.message : "Failed to delete rDNS",
        variant: "destructive",
      });
    } finally {
      setIsDeleting(false);
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
          <CardTitle className="flex items-center gap-2">
            <Globe className="h-5 w-5" />
            Reverse DNS (PTR Records)
          </CardTitle>
          <CardDescription>
            Manage reverse DNS records for your IP addresses. PTR records allow IP-to-hostname lookups.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {ips.length === 0 ? (
            <div className="text-center p-8 text-muted-foreground">
              No IP addresses assigned to this VM.
            </div>
          ) : (
            <div className="space-y-4">
              {ips.map((ip) => {
                const isEditing = editingIPId === ip.id;
                return (
                  <div
                    key={ip.id}
                    className="flex items-center justify-between rounded-lg border p-4"
                  >
                    <div className="flex items-center gap-4">
                      <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary/10">
                        <Network className="h-5 w-5 text-primary" />
                      </div>
                      <div>
                        <div className="flex items-center gap-2">
                          <span className="font-mono font-medium">{ip.address}</span>
                          <Badge variant={ip.ip_version === 4 ? "default" : "secondary"}>
                            IPv{ip.ip_version}
                          </Badge>
                          {ip.is_primary && (
                            <Badge variant="outline">Primary</Badge>
                          )}
                        </div>
                        {isEditing ? (
                          <div className="flex items-center gap-2 mt-2">
                            <Input
                              value={editHostname}
                              onChange={(e) => setEditHostname(e.target.value)}
                              placeholder="e.g., mail.example.com"
                              className="h-8 w-64"
                            />
                            <Button
                              size="sm"
                              onClick={() => handleSave(ip)}
                              disabled={isSaving}
                            >
                              {isSaving ? (
                                <Loader2 className="h-4 w-4 animate-spin" />
                              ) : (
                                <Save className="h-4 w-4" />
                              )}
                            </Button>
                            <Button
                              size="sm"
                              variant="ghost"
                              onClick={handleCancelEdit}
                            >
                              <X className="h-4 w-4" />
                            </Button>
                          </div>
                        ) : (
                          <div className="text-sm text-muted-foreground mt-1">
                            PTR: {ip.rdns_hostname || <span className="italic">Not set</span>}
                          </div>
                        )}
                      </div>
                    </div>
                    {!isEditing && (
                      <div className="flex gap-2">
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => handleEdit(ip)}
                        >
                          <Edit2 className="mr-2 h-4 w-4" />
                          {ip.rdns_hostname ? "Edit" : "Set PTR"}
                        </Button>
                        {ip.rdns_hostname && (
                          <Button
                            variant="outline"
                            size="sm"
                            className="text-destructive hover:bg-destructive hover:text-destructive-foreground"
                            onClick={() => handleDelete(ip)}
                          >
                            <Trash2 className="mr-2 h-4 w-4" />
                            Remove
                          </Button>
                        )}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Delete Confirmation Dialog */}
      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Remove PTR Record</DialogTitle>
            <DialogDescription>
              Are you sure you want to remove the PTR record for {selectedIP?.address}?
              The hostname &quot;{selectedIP?.rdns_hostname}&quot; will no longer resolve to this IP.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteDialogOpen(false)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={confirmDelete} disabled={isDeleting}>
              {isDeleting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Remove
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
