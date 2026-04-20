"use client";

import { useState, useEffect, useCallback } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@virtuestack/ui";
import { Button } from "@virtuestack/ui";
import { Input } from "@virtuestack/ui";
import { Network, Database, HardDrive, FileSpreadsheet, Loader2 } from "lucide-react";
import { useToast } from "@virtuestack/ui";
import { adminIPSetsApi, IPSetDetail } from "@/lib/api-client";
import { IPSetCreateDialog } from "@/components/ip-sets/IPSetCreateDialog";
import { IPSetEditDialog } from "@/components/ip-sets/IPSetEditDialog";
import { IPSetDetailDialog } from "@/components/ip-sets/IPSetDetailDialog";
import { IPSetImportDialog } from "@/components/ip-sets/IPSetImportDialog";
import { IPSetList, IPSetDisplay } from "@/components/ip-sets/IPSetList";
import { CreateIPSetFormData, EditIPSetFormData } from "@/components/ip-sets/validation";

function formatNumber(num: number): string {
  if (num >= 1000000) return (num / 1000000).toFixed(1) + "M";
  if (num >= 1000) return (num / 1000).toFixed(1) + "K";
  return num.toString();
}

export default function IPSetsPage() {
  const [searchTerm, setSearchTerm] = useState("");
  const [ipSets, setIPSets] = useState<IPSetDisplay[]>([]);
  const [loading, setLoading] = useState(true);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [isCreating, setIsCreating] = useState(false);
  const [detailDialogOpen, setDetailDialogOpen] = useState(false);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [selectedIPSetId, setSelectedIPSetId] = useState<string | null>(null);
  const [selectedIPSetForEdit, setSelectedIPSetForEdit] = useState<{
    id: string;
    name: string;
    gateway: string;
    vlan_id?: number | null;
    location_id?: string | null;
    node_ids?: string[];
    ip_version: number;
    network: string;
  } | null>(null);
  const [isSaving, setIsSaving] = useState(false);
  const { toast } = useToast();

  const fetchIPSets = useCallback(async () => {
    try {
      const data = await adminIPSetsApi.getIPSets();
      setIPSets((data || []).map((ipSet) => ({
        id: ipSet.id,
        name: ipSet.name,
        type: ipSet.ip_version === 6 ? "ipv6" as const : "ipv4" as const,
        location: ipSet.location || "Unassigned",
        total_ips: ipSet.total_ips || 0,
        available_ips: ipSet.available_ips || 0,
        cidr: ipSet.network,
      })));
    } catch (err) {
      toast({
        title: "Error",
        description: "Failed to load IP sets.",
        variant: "destructive",
      });
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    void fetchIPSets();
  }, [fetchIPSets]);

  const handleCreate = async (data: CreateIPSetFormData) => {
    setIsCreating(true);
    try {
      const response = await adminIPSetsApi.createIPSet({
        name: data.name,
        network: data.network,
        gateway: data.gateway,
        ip_version: data.ip_version,
        location_id: data.location_id || undefined,
        vlan_id: data.vlan_id || undefined,
        node_ids: data.node_ids || [],
      });

      const newIPSet: IPSetDisplay = {
        id: response.id,
        name: response.name,
        type: data.ip_version === 4 ? "ipv4" : "ipv6",
        location: response.location || "Unassigned",
        total_ips: response.total_ips || 0,
        available_ips: response.available_ips || 0,
        cidr: data.network,
      };

      setIPSets((prev) => [...prev, newIPSet]);

      toast({
        title: "IP Set Created",
        description: `"${data.name}" has been created successfully.`,
      });
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : "Failed to create IP set";
      toast({
        title: "Failed to Create IP Set",
        description: errorMessage,
        variant: "destructive",
      });
    } finally {
      setIsCreating(false);
    }
  };

  const handleView = (ipSet: IPSetDisplay) => {
    setSelectedIPSetId(ipSet.id);
    setDetailDialogOpen(true);
  };

  const handleEditFromDetail = (ipSet: IPSetDetail) => {
    setSelectedIPSetForEdit({
      id: ipSet.id,
      name: ipSet.name,
      gateway: ipSet.gateway,
      vlan_id: ipSet.vlan_id ?? null,
      location_id: ipSet.location_id ?? null,
      node_ids: ipSet.node_ids ?? [],
      ip_version: ipSet.ip_version,
      network: ipSet.network,
    });
    setEditDialogOpen(true);
  };

  const handleEdit = async (data: EditIPSetFormData) => {
    if (!selectedIPSetForEdit) return;

    setIsSaving(true);
    try {
      await adminIPSetsApi.updateIPSet(selectedIPSetForEdit.id, {
        name: data.name,
        gateway: data.gateway,
        vlan_id: data.vlan_id ?? undefined,
        location_id: data.location_id ?? undefined,
        node_ids: data.node_ids,
      });

      // Refresh the list
      await fetchIPSets();

      toast({
        title: "IP Set Updated",
        description: `IP set has been updated successfully.`,
      });
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : "Failed to update IP set";
      toast({
        title: "Failed to Update IP Set",
        description: errorMessage,
        variant: "destructive",
      });
    } finally {
      setIsSaving(false);
    }
  };

  const handleEditClick = async (ipSet: IPSetDisplay) => {
    // Fetch full details for editing
    try {
      const details = await adminIPSetsApi.getIPSet(ipSet.id);
      setSelectedIPSetForEdit({
        id: details.id,
        name: details.name,
        gateway: details.gateway,
        vlan_id: details.vlan_id ?? null,
        location_id: details.location_id ?? null,
        node_ids: details.node_ids ?? [],
        ip_version: details.ip_version,
        network: details.network,
      });
      setEditDialogOpen(true);
    } catch {
      toast({
        title: "Error",
        description: "Failed to load IP set details for editing.",
        variant: "destructive",
      });
    }
  };

  const filteredIPSets = ipSets.filter(
    (ipSet) =>
      ipSet.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      ipSet.location.toLowerCase().includes(searchTerm.toLowerCase())
  );

  const totalSets = ipSets.length;
  const totalIPs = ipSets.reduce((acc, set) => acc + set.total_ips, 0);
  const availableIPs = ipSets.reduce((acc, set) => acc + set.available_ips, 0);
  const ipv4Sets = ipSets.filter((set) => set.type === "ipv4").length;
  const ipv6Sets = ipSets.filter((set) => set.type === "ipv6").length;

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-background p-6 md:p-8">
      <div className="mx-auto max-w-7xl space-y-6">
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">IP Address Pools</h1>
            <p className="text-muted-foreground">
              Manage IP address sets and allocations
            </p>
          </div>
          <div className="flex gap-2">
            <IPSetImportDialog
              ipSets={ipSets}
              onImportComplete={fetchIPSets}
            />
            <Button size="default" onClick={() => setCreateDialogOpen(true)}>
              <Network className="mr-2 h-4 w-4" />
              Create IP Set
            </Button>
          </div>
        </div>

        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <Card>
            <CardContent className="pt-6">
              <div className="flex items-center gap-4">
                <div className="flex h-12 w-12 items-center justify-center rounded-full bg-blue-500/10">
                  <Database className="h-5 w-5 text-blue-500" />
                </div>
                <div>
                  <div className="text-2xl font-bold">{totalSets}</div>
                  <p className="text-xs text-muted-foreground">Total Sets</p>
                </div>
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="pt-6">
              <div className="flex items-center gap-4">
                <div className="flex h-12 w-12 items-center justify-center rounded-full bg-green-500/10">
                  <Network className="h-5 w-5 text-green-500" />
                </div>
                <div>
                  <div className="text-2xl font-bold">{formatNumber(totalIPs)}</div>
                  <p className="text-xs text-muted-foreground">Total IPs</p>
                </div>
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="pt-6">
              <div className="flex items-center gap-4">
                <div className="flex h-12 w-12 items-center justify-center rounded-full bg-purple-500/10">
                  <HardDrive className="h-5 w-5 text-purple-500" />
                </div>
                <div>
                  <div className="text-2xl font-bold">{formatNumber(availableIPs)}</div>
                  <p className="text-xs text-muted-foreground">Available IPs</p>
                </div>
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="pt-6">
              <div className="flex items-center gap-4">
                <div className="flex h-12 w-12 items-center justify-center rounded-full bg-orange-500/10">
                  <FileSpreadsheet className="h-5 w-5 text-orange-500" />
                </div>
                <div>
                  <div className="text-2xl font-bold">{ipv4Sets}/{ipv6Sets}</div>
                  <p className="text-xs text-muted-foreground">IPv4 / IPv6 Sets</p>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>

        <Card>
          <CardContent className="pt-6">
            <div className="relative">
              <Network className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Search by name or location..."
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
                className="pl-10"
              />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Network className="h-5 w-5" />
              All IP Sets
            </CardTitle>
            <CardDescription>
              {filteredIPSets.length} of {ipSets.length} IP sets displayed
            </CardDescription>
          </CardHeader>
          <CardContent>
            <IPSetList
              ipSets={filteredIPSets}
              onView={handleView}
              onEdit={handleEditClick}
            />
          </CardContent>
        </Card>
      </div>

      <IPSetCreateDialog
        open={createDialogOpen}
        onOpenChange={setCreateDialogOpen}
        onCreate={handleCreate}
        isCreating={isCreating}
      />

      <IPSetDetailDialog
        open={detailDialogOpen}
        onOpenChange={setDetailDialogOpen}
        ipSetId={selectedIPSetId}
        onEdit={handleEditFromDetail}
      />

      <IPSetEditDialog
        open={editDialogOpen}
        onOpenChange={setEditDialogOpen}
        ipSet={selectedIPSetForEdit}
        onSave={handleEdit}
        isSaving={isSaving}
      />
    </div>
  );
}
