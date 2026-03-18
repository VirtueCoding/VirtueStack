"use client";

import { useState, useEffect } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Network, Database, HardDrive, FileSpreadsheet, Loader2 } from "lucide-react";
import { useToast } from "@/components/ui/use-toast";
import { apiClient } from "@/lib/api-client";
import { IPSetCreateDialog } from "@/components/ip-sets/IPSetCreateDialog";
import { IPSetImportDialog } from "@/components/ip-sets/IPSetImportDialog";
import { IPSetList, IPSetDisplay } from "@/components/ip-sets/IPSetList";
import { CreateIPSetFormData } from "@/components/ip-sets/validation";

interface IPSet {
  id: string;
  name: string;
  type: "ipv4" | "ipv6";
  location: string;
  total_ips: number;
  available_ips: number;
  cidr: string;
}

function mapApiIPSet(raw: Record<string, unknown>): IPSetDisplay {
  return {
    id: typeof raw.id === 'string' ? raw.id : '',
    name: typeof raw.name === 'string' ? raw.name : '',
    type: raw.ip_version === 6 ? "ipv6" : "ipv4",
    location: typeof raw.location === 'string' ? raw.location : "Unassigned",
    total_ips: typeof raw.total_ips === 'number' ? raw.total_ips : 0,
    available_ips: typeof raw.available_ips === 'number' ? raw.available_ips : 0,
    cidr: typeof raw.network === 'string' ? raw.network : "",
  };
}

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
  const { toast } = useToast();

  const fetchIPSets = async () => {
    try {
      const data = await apiClient.get<Record<string, unknown>[]>("/admin/ip-sets");
      setIPSets((data || []).map(mapApiIPSet));
    } catch (err) {
      toast({
        title: "Error",
        description: "Failed to load IP sets.",
        variant: "destructive",
      });
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchIPSets();
  }, []);

  const handleCreate = async (data: CreateIPSetFormData) => {
    setIsCreating(true);
    try {
      const response = await apiClient.post<IPSet>("/admin/ip-sets", {
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
            <IPSetList ipSets={filteredIPSets} />
          </CardContent>
        </Card>
      </div>

      <IPSetCreateDialog
        open={createDialogOpen}
        onOpenChange={setCreateDialogOpen}
        onCreate={handleCreate}
        isCreating={isCreating}
      />
    </div>
  );
}