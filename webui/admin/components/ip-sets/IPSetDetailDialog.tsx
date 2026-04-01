"use client";

import { useRef, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@virtuestack/ui";
import { Badge } from "@virtuestack/ui";
import { Button } from "@virtuestack/ui";
import {
  Network,
  Router,
  Hash,
  MapPin,
  Calendar,
  Loader2,
  Database,
  CheckCircle,
  Clock,
  Ban,
} from "lucide-react";
import { adminIPSetsApi, IPSetDetail } from "@/lib/api-client";

interface IPSetDetailDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  ipSetId: string | null;
  onEdit: (ipSet: IPSetDetail) => void;
}

function formatNumber(num: number): string {
  if (num >= 1000000) return (num / 1000000).toFixed(1) + "M";
  if (num >= 1000) return (num / 1000).toFixed(1) + "K";
  return num.toString();
}

function getUsagePercentage(available: number, total: number) {
  if (total === 0) return 0;
  return ((total - available) / total) * 100;
}

function getUsageColor(percentage: number) {
  if (percentage > 90) return "text-red-500";
  if (percentage > 70) return "text-yellow-500";
  return "text-green-500";
}

export function IPSetDetailDialog({ open, onOpenChange, ipSetId, onEdit }: IPSetDetailDialogProps) {
  const [ipSet, setIPSet] = useState<IPSetDetail | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const fetchingRef = useRef(false);

  // Handle dialog open state changes
  const handleOpenChange = (isOpen: boolean) => {
    if (!isOpen) {
      setIPSet(null);
      setError(null);
      fetchingRef.current = false;
      onOpenChange(false);
    }
  };

  // Fetch data when opened - triggered by Dialog's onOpenAutoFocus
  const handleOpenAutoFocus = async (event: Event) => {
    event.preventDefault();

    if (!ipSetId || fetchingRef.current) return;

    fetchingRef.current = true;
    setLoading(true);
    setError(null);

    try {
      const data = await adminIPSetsApi.getIPSet(ipSetId);
      setIPSet(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load IP set details");
    } finally {
      setLoading(false);
      fetchingRef.current = false;
    }
  };

  const handleEdit = () => {
    if (ipSet) {
      onEdit(ipSet);
      handleOpenChange(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        className="sm:max-w-[650px] max-h-[85vh] overflow-y-auto"
        onOpenAutoFocus={handleOpenAutoFocus}
      >
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Network className="h-5 w-5" />
            IP Set Details
          </DialogTitle>
          <DialogDescription>
            View detailed information about this IP address pool.
          </DialogDescription>
        </DialogHeader>

        {loading && (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
          </div>
        )}

        {error && (
          <div className="rounded-md border border-destructive/50 bg-destructive/10 p-4">
            <p className="text-sm text-destructive">{error}</p>
          </div>
        )}

        {ipSet && !loading && (
          <div className="space-y-6">
            {/* Basic Info */}
            <div className="space-y-3">
              <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
                Basic Information
              </h4>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-1">
                  <div className="text-xs text-muted-foreground">Name</div>
                  <div className="font-medium">{ipSet.name}</div>
                </div>
                <div className="space-y-1">
                  <div className="text-xs text-muted-foreground">IP Version</div>
                  <Badge variant={ipSet.ip_version === 4 ? "default" : "secondary"} className="font-mono">
                    IPv{ipSet.ip_version}
                  </Badge>
                </div>
                <div className="space-y-1">
                  <div className="text-xs text-muted-foreground flex items-center gap-1">
                    <Network className="h-3 w-3" /> Network CIDR
                  </div>
                  <div className="font-mono text-sm">{ipSet.network}</div>
                </div>
                <div className="space-y-1">
                  <div className="text-xs text-muted-foreground flex items-center gap-1">
                    <Router className="h-3 w-3" /> Gateway
                  </div>
                  <div className="font-mono text-sm">{ipSet.gateway}</div>
                </div>
                {ipSet.vlan_id && (
                  <div className="space-y-1">
                    <div className="text-xs text-muted-foreground flex items-center gap-1">
                      <Hash className="h-3 w-3" /> VLAN ID
                    </div>
                    <div className="font-mono text-sm">{ipSet.vlan_id}</div>
                  </div>
                )}
                {ipSet.location_id && (
                  <div className="space-y-1">
                    <div className="text-xs text-muted-foreground flex items-center gap-1">
                      <MapPin className="h-3 w-3" /> Location
                    </div>
                    <div className="text-sm">{ipSet.location_id}</div>
                  </div>
                )}
                <div className="space-y-1 col-span-2">
                  <div className="text-xs text-muted-foreground flex items-center gap-1">
                    <Calendar className="h-3 w-3" /> Created
                  </div>
                  <div className="text-sm">
                    {new Date(ipSet.created_at).toLocaleDateString("en-US", {
                      year: "numeric",
                      month: "short",
                      day: "numeric",
                      hour: "2-digit",
                      minute: "2-digit",
                    })}
                  </div>
                </div>
              </div>
            </div>

            {/* IP Statistics */}
            <div className="space-y-3">
              <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
                IP Address Statistics
              </h4>
              <div className="grid grid-cols-2 sm:grid-cols-5 gap-3">
                <div className="rounded-md border p-3 text-center">
                  <div className="flex items-center justify-center gap-1 text-xs text-muted-foreground mb-1">
                    <Database className="h-3 w-3" /> Total
                  </div>
                  <div className="text-xl font-bold">{formatNumber(ipSet.total_ips)}</div>
                </div>
                <div className="rounded-md border p-3 text-center">
                  <div className="flex items-center justify-center gap-1 text-xs text-muted-foreground mb-1">
                    <CheckCircle className="h-3 w-3" /> Available
                  </div>
                  <div className="text-xl font-bold text-green-500">{formatNumber(ipSet.available_ips)}</div>
                </div>
                <div className="rounded-md border p-3 text-center">
                  <div className="flex items-center justify-center gap-1 text-xs text-muted-foreground mb-1">
                    <Network className="h-3 w-3" /> Assigned
                  </div>
                  <div className="text-xl font-bold text-blue-500">{formatNumber(ipSet.assigned_ips)}</div>
                </div>
                <div className="rounded-md border p-3 text-center">
                  <div className="flex items-center justify-center gap-1 text-xs text-muted-foreground mb-1">
                    <Ban className="h-3 w-3" /> Reserved
                  </div>
                  <div className="text-xl font-bold text-orange-500">{formatNumber(ipSet.reserved_ips)}</div>
                </div>
                <div className="rounded-md border p-3 text-center">
                  <div className="flex items-center justify-center gap-1 text-xs text-muted-foreground mb-1">
                    <Clock className="h-3 w-3" /> Cooldown
                  </div>
                  <div className="text-xl font-bold text-purple-500">{formatNumber(ipSet.cooldown_ips)}</div>
                </div>
              </div>

              {/* Usage Progress Bar */}
              <div className="space-y-2">
                <div className="flex justify-between text-sm">
                  <span className="text-muted-foreground">Usage</span>
                  <span className={getUsageColor(getUsagePercentage(ipSet.available_ips, ipSet.total_ips))}>
                    {getUsagePercentage(ipSet.available_ips, ipSet.total_ips).toFixed(1)}% used
                  </span>
                </div>
                <div className="h-3 w-full overflow-hidden rounded-full bg-secondary">
                  <div
                    className={`h-full transition-all ${
                      getUsagePercentage(ipSet.available_ips, ipSet.total_ips) > 90
                        ? "bg-red-500"
                        : getUsagePercentage(ipSet.available_ips, ipSet.total_ips) > 70
                        ? "bg-yellow-500"
                        : "bg-green-500"
                    }`}
                    style={{ width: `${getUsagePercentage(ipSet.available_ips, ipSet.total_ips)}%` }}
                  />
                </div>
              </div>
            </div>

            {/* Node Assignment */}
            {ipSet.node_ids && ipSet.node_ids.length > 0 && (
              <div className="space-y-3">
                <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
                  Assigned Nodes ({ipSet.node_ids.length})
                </h4>
                <div className="flex flex-wrap gap-2">
                  {ipSet.node_ids.map((nodeId) => (
                    <Badge key={nodeId} variant="outline" className="font-mono text-xs">
                      {nodeId.substring(0, 8)}...
                    </Badge>
                  ))}
                </div>
              </div>
            )}

            {/* Actions */}
            <div className="flex justify-end gap-2 pt-4 border-t">
              <Button variant="outline" onClick={() => handleOpenChange(false)}>
                Close
              </Button>
              <Button onClick={handleEdit}>
                Edit IP Set
              </Button>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}