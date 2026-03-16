"use client";

import { useState, useEffect } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
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
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  Network,
  MapPin,
  Plus,
  Upload,
  Search,
  HardDrive,
  Database,
  FileSpreadsheet,
  Loader2,
} from "lucide-react";
import { useToast } from "@/components/ui/use-toast";
import { apiClient } from "@/lib/api-client";

interface IPSet {
  id: string;
  name: string;
  type: "ipv4" | "ipv6";
  location: string;
  total_ips: number;
  available_ips: number;
  cidr: string;
}

interface CreateIPSetRequest {
  name: string;
  network: string;
  gateway: string;
  ip_version: 4 | 6;
  location_id?: string;
  vlan_id?: number;
  node_ids?: string[];
}

function mapApiIPSet(raw: Record<string, unknown>): IPSet {
  return {
    id: raw.id as string,
    name: raw.name as string,
    type: (raw.ip_version === 6 ? "ipv6" : "ipv4") as "ipv4" | "ipv6",
    location: (raw.location as string) || "Unassigned",
    total_ips: (raw.total_ips as number) || 0,
    available_ips: (raw.available_ips as number) || 0,
    cidr: (raw.network as string) || "",
  };
}

function getTypeBadge(type: IPSet["type"]) {
  const variants = {
    ipv4: "default" as const,
    ipv6: "secondary" as const,
  };

  return (
    <Badge variant={variants[type]} className="font-mono">
      {type.toUpperCase()}
    </Badge>
  );
}

function getUsagePercentage(available: number, total: number) {
  if (total === 0) return 0;
  return ((total - available) / total) * 100;
}

function getUsageColor(percentage: number) {
  if (percentage > 90) return "bg-red-500";
  if (percentage > 70) return "bg-yellow-500";
  return "bg-green-500";
}

function formatNumber(num: number): string {
  if (num >= 1000000) return (num / 1000000).toFixed(1) + "M";
  if (num >= 1000) return (num / 1000).toFixed(1) + "K";
  return num.toString();
}

export default function IPSetsPage() {
  const [searchTerm, setSearchTerm] = useState("");
  const [ipSets, setIPSets] = useState<IPSet[]>([]);
  const [loading, setLoading] = useState(true);
  const [importDialogOpen, setImportDialogOpen] = useState(false);
  const { toast } = useToast();

  useEffect(() => {
    async function fetchIPSets() {
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
    }
    fetchIPSets();
  }, [toast]);
  const [importFile, setImportFile] = useState<File | null>(null);
  const [importTargetPool, setImportTargetPool] = useState("");
  const [isImporting, setIsImporting] = useState(false);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [isCreating, setIsCreating] = useState(false);

  const [formData, setFormData] = useState<CreateIPSetRequest>({
    name: "",
    network: "",
    gateway: "",
    ip_version: 4,
  });
  const [formErrors, setFormErrors] = useState<Record<string, string>>({});

  const filteredIPSets = ipSets.filter(
    (ipSet) =>
      ipSet.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      ipSet.location.toLowerCase().includes(searchTerm.toLowerCase())
  );

  const validateCIDR = (cidr: string, ipVersion: 4 | 6): boolean => {
    if (!cidr) return false;
    
    if (ipVersion === 4) {
      const ipv4CidrRegex = /^(\d{1,3}\.){3}\d{1,3}\/(\d{1,2})$/;
      if (!ipv4CidrRegex.test(cidr)) return false;
      
      const [ip, prefix] = cidr.split("/");
      const prefixNum = parseInt(prefix, 10);
      if (prefixNum < 1 || prefixNum > 32) return false;
      
      const parts = ip.split(".").map(Number);
      return parts.every((part) => part >= 0 && part <= 255);
    } else {
      const ipv6CidrRegex = /^([0-9a-fA-F:]+)\/(\d{1,3})$/;
      if (!ipv6CidrRegex.test(cidr)) return false;
      
      const [, prefix] = cidr.split("/");
      const prefixNum = parseInt(prefix, 10);
      return prefixNum >= 1 && prefixNum <= 128;
    }
  };

  const validateIP = (ip: string, ipVersion: 4 | 6): boolean => {
    if (!ip) return false;
    
    if (ipVersion === 4) {
      const ipv4Regex = /^(\d{1,3}\.){3}\d{1,3}$/;
      if (!ipv4Regex.test(ip)) return false;
      const parts = ip.split(".").map(Number);
      return parts.every((part) => part >= 0 && part <= 255);
    } else {
      const ipv6Regex = /^([0-9a-fA-F]{0,4}:){2,7}[0-9a-fA-F]{0,4}$/;
      return ipv6Regex.test(ip) || ip === "::" || ip.includes("::");
    }
  };

  const validateForm = (): boolean => {
    const errors: Record<string, string> = {};

    if (!formData.name.trim()) {
      errors.name = "Name is required";
    } else if (formData.name.length > 100) {
      errors.name = "Name must be less than 100 characters";
    }

    if (!formData.network.trim()) {
      errors.network = "Network CIDR is required";
    } else if (!validateCIDR(formData.network, formData.ip_version)) {
      errors.network = formData.ip_version === 4 
        ? "Invalid IPv4 CIDR format (e.g., 10.0.0.0/24)"
        : "Invalid IPv6 CIDR format (e.g., 2001:db8::/32)";
    }

    if (!formData.gateway.trim()) {
      errors.gateway = "Gateway is required";
    } else if (!validateIP(formData.gateway, formData.ip_version)) {
      errors.gateway = formData.ip_version === 4
        ? "Invalid IPv4 address"
        : "Invalid IPv6 address";
    }

    if (formData.vlan_id !== undefined && formData.vlan_id !== null) {
      if (formData.vlan_id < 1 || formData.vlan_id > 4094) {
        errors.vlan_id = "VLAN ID must be between 1 and 4094";
      }
    }

    setFormErrors(errors);
    return Object.keys(errors).length === 0;
  };

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    
    if (!validateForm()) {
      return;
    }

    setIsCreating(true);
    
    try {
      const response = await apiClient.post<IPSet>("/admin/ip-sets", {
        name: formData.name,
        network: formData.network,
        gateway: formData.gateway,
        ip_version: formData.ip_version,
        location_id: formData.location_id || undefined,
        vlan_id: formData.vlan_id || undefined,
        node_ids: formData.node_ids || [],
      });

      const newIPSet: IPSet = {
        id: response.id,
        name: response.name,
        type: formData.ip_version === 4 ? "ipv4" : "ipv6",
        location: response.location || "Unassigned",
        total_ips: response.total_ips || 0,
        available_ips: response.available_ips || 0,
        cidr: formData.network,
      };

      setIPSets((prev) => [...prev, newIPSet]);
      
      toast({
        title: "IP Set Created",
        description: `"${formData.name}" has been created successfully.`,
      });

      setFormData({
        name: "",
        network: "",
        gateway: "",
        ip_version: 4,
      });
      setFormErrors({});
      setCreateDialogOpen(false);
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

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0] || null;
    setImportFile(file);
  };

  const handleImport = async (e: React.FormEvent) => {
    e.preventDefault();
    
    if (!importFile) {
      toast({
        title: "No File Selected",
        description: "Please select a CSV or text file to import.",
        variant: "destructive",
      });
      return;
    }

    if (importFile.size > 1 * 1024 * 1024) {
      toast({
        title: "File Too Large",
        description: "Import file must be under 1MB.",
        variant: "destructive",
      });
      return;
    }
    
    if (!importTargetPool) {
      toast({
        title: "No Pool Selected", 
        description: "Please select a target pool for the imported IPs.",
        variant: "destructive",
      });
      return;
    }
    
    setIsImporting(true);
    
    try {
      const text = await importFile.text();
      const lines = text.split(/[\r\n]+/).map((line) => line.trim()).filter(Boolean);
      
      // Parse IPs: support "ip" or "ip,subnet,gateway" CSV format
      const ips: string[] = [];
      for (const line of lines) {
        // Skip header rows
        if (line.toLowerCase().startsWith("ip") || line.startsWith("#")) continue;
        
        // Take first column if CSV
        const ip = line.split(",")[0].trim();
        
        // Basic IP validation (v4 or v6)
        const ipv4Regex = /^(\d{1,3}\.){3}\d{1,3}(\/\d{1,2})?$/;
        const ipv6Regex = /^[0-9a-fA-F:]+(::[0-9a-fA-F]*)*?(\/\d{1,3})?$/;
        
        if (ipv4Regex.test(ip) || ipv6Regex.test(ip)) {
          ips.push(ip);
        }
      }
      
      if (ips.length === 0) {
        toast({
          title: "No Valid IPs Found",
          description: "The file does not contain any valid IP addresses. Ensure one IP per line.",
          variant: "destructive",
        });
        return;
      }
      
      // Call API to import IPs into the target pool
      await apiClient.post(`/admin/ip-sets/${importTargetPool}/import`, { addresses: ips });
      
      toast({
        title: "Import Successful",
        description: `${ips.length} IP address${ips.length !== 1 ? "es" : ""} imported successfully.`,
      });
      
      setImportDialogOpen(false);
      setImportFile(null);
      setImportTargetPool("");
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : "Failed to import IP addresses";
      toast({
        title: "Import Failed",
        description: errorMessage,
        variant: "destructive",
      });
    } finally {
      setIsImporting(false);
    }
  };

  const totalSets = ipSets.length;
  const totalIPs = ipSets.reduce((acc, set) => acc + set.total_ips, 0);
  const availableIPs = ipSets.reduce((acc, set) => acc + set.available_ips, 0);
  const ipv4Sets = ipSets.filter((set) => set.type === "ipv4").length;
  const ipv6Sets = ipSets.filter((set) => set.type === "ipv6").length;

  return (
    <div className="min-h-screen bg-background p-6 md:p-8">
      <div className="mx-auto max-w-7xl space-y-6">
        {/* Header */}
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">IP Address Pools</h1>
            <p className="text-muted-foreground">
              Manage IP address sets and allocations
            </p>
          </div>
          <div className="flex gap-2">
            <Dialog open={importDialogOpen} onOpenChange={setImportDialogOpen}>
              <DialogTrigger asChild>
                <Button variant="outline" size="default">
                  <Upload className="mr-2 h-4 w-4" />
                  Import IPs
                </Button>
              </DialogTrigger>
              <DialogContent className="sm:max-w-[525px]">
                <DialogHeader>
                  <DialogTitle>Import IP Addresses</DialogTitle>
                  <DialogDescription>
                    Upload a CSV or text file containing IP addresses to add to a pool.
                  </DialogDescription>
                </DialogHeader>
                <form onSubmit={handleImport}>
                  <div className="grid gap-4 py-4">
                    <div className="grid gap-2">
                      <label className="text-sm font-medium" htmlFor="file-upload">
                        Select File
                      </label>
                      <div className="flex items-center gap-4">
                        <Input
                          id="file-upload"
                          type="file"
                          accept=".csv,.txt"
                          onChange={handleFileChange}
                          className="flex-1"
                        />
                      </div>
                      <p className="text-xs text-muted-foreground">
                        Supported formats: CSV, TXT (one IP per line)
                      </p>
                    </div>
                    <div className="grid gap-2">
                      <label className="text-sm font-medium" htmlFor="target-pool">
                        Target Pool
                      </label>
                      <select
                        id="target-pool"
                        value={importTargetPool}
                        onChange={(e) => setImportTargetPool(e.target.value)}
                        className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                      >
                        <option value="">Select a pool...</option>
                        {ipSets.map((set) => (
                          <option key={set.id} value={set.id}>
                            {set.name} ({set.cidr})
                          </option>
                        ))}
                      </select>
                    </div>
                  </div>
                  <DialogFooter>
                    <Button type="button" variant="outline" onClick={() => setImportDialogOpen(false)} disabled={isImporting}>
                      Cancel
                    </Button>
                    <Button type="submit" disabled={isImporting}>
                      {isImporting ? (
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      ) : (
                        <Upload className="mr-2 h-4 w-4" />
                      )}
                      {isImporting ? "Importing..." : "Import"}
                    </Button>
                  </DialogFooter>
                </form>
              </DialogContent>
            </Dialog>
            <Button size="default" onClick={() => setCreateDialogOpen(true)}>
              <Plus className="mr-2 h-4 w-4" />
              Create IP Set
            </Button>
          </div>
        </div>

        {/* Summary Stats */}
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

        {/* Search and Filter */}
        <Card>
          <CardContent className="pt-6">
            <div className="relative">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Search by name or location..."
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
                className="pl-10"
              />
            </div>
          </CardContent>
        </Card>

        {/* IP Sets Table */}
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
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Type</TableHead>
                    <TableHead>Location</TableHead>
                    <TableHead>CIDR</TableHead>
                    <TableHead>Total IPs</TableHead>
                    <TableHead>Available IPs</TableHead>
                    <TableHead>Usage</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredIPSets.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={8} className="h-24 text-center">
                        No IP sets found
                      </TableCell>
                    </TableRow>
                  ) : (
                    filteredIPSets.map((ipSet) => {
                      const usagePercentage = getUsagePercentage(ipSet.available_ips, ipSet.total_ips);
                      return (
                        <TableRow key={ipSet.id}>
                          <TableCell>
                            <div className="font-medium">{ipSet.name}</div>
                          </TableCell>
                          <TableCell>{getTypeBadge(ipSet.type)}</TableCell>
                          <TableCell>
                            <div className="flex items-center gap-2 text-muted-foreground">
                              <MapPin className="h-3 w-3" />
                              {ipSet.location}
                            </div>
                          </TableCell>
                          <TableCell className="font-mono text-xs text-muted-foreground">
                            {ipSet.cidr}
                          </TableCell>
                          <TableCell className="text-muted-foreground">
                            {formatNumber(ipSet.total_ips)}
                          </TableCell>
                          <TableCell className="text-muted-foreground">
                            {formatNumber(ipSet.available_ips)}
                          </TableCell>
                          <TableCell className="w-[180px]">
                            <div className="space-y-1">
                              <div className="flex justify-between text-xs">
                                <span className="text-muted-foreground">
                                  {usagePercentage.toFixed(0)}% used
                                </span>
                              </div>
                              <div className="h-2 w-full overflow-hidden rounded-full bg-secondary">
                                <div
                                  className={`h-full transition-all ${getUsageColor(usagePercentage)}`}
                                  style={{ width: `${usagePercentage}%` }}
                                />
                              </div>
                            </div>
                          </TableCell>
                          <TableCell className="text-right">
                            <div className="flex justify-end gap-2">
                              <Button variant="outline" size="sm" disabled>
                                View
                              </Button>
                              <Button variant="outline" size="sm" disabled>
                                Edit
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
      </div>

      <Dialog open={createDialogOpen} onOpenChange={setCreateDialogOpen}>
        <DialogContent className="sm:max-w-[525px]">
          <DialogHeader>
            <DialogTitle>Create IP Set</DialogTitle>
            <DialogDescription>
              Create a new IP address pool for VM assignments.
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleCreate}>
            <div className="grid gap-4 py-4">
              <div className="grid gap-2">
                <Label htmlFor="name">Name</Label>
                <Input
                  id="name"
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                  placeholder="e.g., production-pool-01"
                  className={formErrors.name ? "border-destructive" : ""}
                />
                {formErrors.name && (
                  <p className="text-xs text-destructive">{formErrors.name}</p>
                )}
              </div>

              <div className="grid gap-2">
                <Label htmlFor="ip-version">IP Version</Label>
                <select
                  id="ip-version"
                  value={formData.ip_version}
                  onChange={(e) => setFormData({ ...formData, ip_version: parseInt(e.target.value) as 4 | 6 })}
                  className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                >
                  <option value={4}>IPv4</option>
                  <option value={6}>IPv6</option>
                </select>
              </div>

              <div className="grid gap-2">
                <Label htmlFor="network">Network CIDR</Label>
                <Input
                  id="network"
                  value={formData.network}
                  onChange={(e) => setFormData({ ...formData, network: e.target.value })}
                  placeholder={formData.ip_version === 4 ? "10.0.0.0/24" : "2001:db8::/32"}
                  className={formErrors.network ? "border-destructive font-mono" : "font-mono"}
                />
                {formErrors.network ? (
                  <p className="text-xs text-destructive">{formErrors.network}</p>
                ) : (
                  <p className="text-xs text-muted-foreground">
                    {formData.ip_version === 4 
                      ? "Format: xxx.xxx.xxx.xxx/xx (e.g., 10.0.0.0/24)"
                      : "Format: xxxx:xxxx::/xx (e.g., 2001:db8::/32)"}
                  </p>
                )}
              </div>

              <div className="grid gap-2">
                <Label htmlFor="gateway">Gateway</Label>
                <Input
                  id="gateway"
                  value={formData.gateway}
                  onChange={(e) => setFormData({ ...formData, gateway: e.target.value })}
                  placeholder={formData.ip_version === 4 ? "10.0.0.1" : "2001:db8::1"}
                  className={formErrors.gateway ? "border-destructive font-mono" : "font-mono"}
                />
                {formErrors.gateway && (
                  <p className="text-xs text-destructive">{formErrors.gateway}</p>
                )}
              </div>

              <div className="grid gap-2">
                <Label htmlFor="vlan-id">VLAN ID (Optional)</Label>
                <Input
                  id="vlan-id"
                  type="number"
                  min={1}
                  max={4094}
                  value={formData.vlan_id || ""}
                  onChange={(e) => {
                    const value = e.target.value ? parseInt(e.target.value, 10) : undefined;
                    setFormData({ ...formData, vlan_id: value });
                  }}
                  placeholder="e.g., 100"
                  className={formErrors.vlan_id ? "border-destructive" : ""}
                />
                {formErrors.vlan_id && (
                  <p className="text-xs text-destructive">{formErrors.vlan_id}</p>
                )}
              </div>
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setCreateDialogOpen(false)}
                disabled={isCreating}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={isCreating}>
                {isCreating ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Creating...
                  </>
                ) : (
                  <>
                    <Plus className="mr-2 h-4 w-4" />
                    Create
                  </>
                )}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
