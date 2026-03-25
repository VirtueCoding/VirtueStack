"use client";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { getStatusBadgeVariant } from "@/lib/status-badge";
import { Server, Network, Cpu, MemoryStick, MapPin, Activity, Clock, HardDrive } from "lucide-react";

interface NodeDetailDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  node: {
    id: string;
    hostname: string;
    grpc_address: string;
    management_ip?: string;
    location: string;
    status: string;
    total_vcpu: number;
    total_memory_mb: number;
    cpu_allocated: number;
    memory_allocated_gb: number;
    vm_count: number;
    created_at?: string;
    last_heartbeat_at?: string;
    storage_backend?: string;
    storage_backend_name?: string;
  } | null;
}

export function NodeDetailDialog({ open, onOpenChange, node }: NodeDetailDialogProps) {
  if (!node) return null;

  const cpuPercent = node.total_vcpu > 0 ? Math.round((node.cpu_allocated / node.total_vcpu) * 100) : 0;
  const memoryGb = node.total_memory_mb / 1024;
  const memoryPercent = memoryGb > 0 ? Math.round((node.memory_allocated_gb / memoryGb) * 100) : 0;

  const formatDate = (dateStr?: string) => {
    if (!dateStr) return "Never";
    try {
      return new Intl.DateTimeFormat("en-US", {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(new Date(dateStr));
    } catch {
      return dateStr;
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Server className="h-5 w-5" />
            Node Details: {node.hostname}
          </DialogTitle>
          <DialogDescription>
            Read-only view of node configuration and status. Use Edit to modify settings.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-6 py-4">
          {/* Status Section */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide flex items-center gap-2">
              <Activity className="h-4 w-4" />
              Status
            </h4>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-1">
                <p className="text-xs text-muted-foreground">Current Status</p>
                <Badge variant={getStatusBadgeVariant(node.status) as "default" | "secondary" | "destructive" | "outline"} className="capitalize">
                  {node.status}
                </Badge>
              </div>
              <div className="space-y-1">
                <p className="text-xs text-muted-foreground flex items-center gap-1">
                  <Clock className="h-3 w-3" />
                  Last Heartbeat
                </p>
                <p className="text-sm font-medium">{formatDate(node.last_heartbeat_at)}</p>
              </div>
            </div>
          </div>

          {/* Network Section */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide flex items-center gap-2">
              <Network className="h-4 w-4" />
              Network
            </h4>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-1">
                <p className="text-xs text-muted-foreground">gRPC Address</p>
                <p className="text-sm font-medium font-mono">{node.grpc_address}</p>
              </div>
              <div className="space-y-1">
                <p className="text-xs text-muted-foreground">Management IP</p>
                <p className="text-sm font-medium font-mono">{node.management_ip || "Not configured"}</p>
              </div>
              <div className="space-y-1">
                <p className="text-xs text-muted-foreground flex items-center gap-1">
                  <MapPin className="h-3 w-3" />
                  Location
                </p>
                <p className="text-sm font-medium">{node.location}</p>
              </div>
            </div>
          </div>

          {/* Resources Section */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide flex items-center gap-2">
              <Cpu className="h-4 w-4" />
              Resources
            </h4>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <p className="text-xs text-muted-foreground">CPU Allocation</p>
                <div className="flex items-center justify-between text-sm">
                  <span>{node.cpu_allocated} / {node.total_vcpu} Cores</span>
                  <span className="text-muted-foreground">{cpuPercent}%</span>
                </div>
                <div className="h-2 bg-secondary rounded-full overflow-hidden">
                  <div
                    className="h-full bg-primary transition-all duration-300"
                    style={{ width: `${Math.min(cpuPercent, 100)}%` }}
                  />
                </div>
              </div>
              <div className="space-y-2">
                <p className="text-xs text-muted-foreground">Memory Allocation</p>
                <div className="flex items-center justify-between text-sm">
                  <span>{node.memory_allocated_gb} / {memoryGb.toFixed(1)} GB</span>
                  <span className="text-muted-foreground">{memoryPercent}%</span>
                </div>
                <div className="h-2 bg-secondary rounded-full overflow-hidden">
                  <div
                    className="h-full bg-primary transition-all duration-300"
                    style={{ width: `${Math.min(memoryPercent, 100)}%` }}
                  />
                </div>
              </div>
            </div>
          </div>

          {/* Storage Section */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide flex items-center gap-2">
              <HardDrive className="h-4 w-4" />
              Storage
            </h4>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-1">
                <p className="text-xs text-muted-foreground">Storage Backend</p>
                <p className="text-sm font-medium capitalize">{node.storage_backend || "Not configured"}</p>
              </div>
              {node.storage_backend_name && (
                <div className="space-y-1">
                  <p className="text-xs text-muted-foreground">Backend Name</p>
                  <p className="text-sm font-medium">{node.storage_backend_name}</p>
                </div>
              )}
            </div>
          </div>

          {/* VMs Section */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide flex items-center gap-2">
              <Server className="h-4 w-4" />
              Virtual Machines
            </h4>
            <div className="grid grid-cols-1 gap-4">
              <div className="space-y-1">
                <p className="text-xs text-muted-foreground">Running VMs</p>
                <p className="text-2xl font-bold">{node.vm_count}</p>
              </div>
            </div>
          </div>

          {/* Metadata Section */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
              Metadata
            </h4>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4 text-sm">
              <div className="space-y-1">
                <p className="text-xs text-muted-foreground">Node ID</p>
                <p className="font-mono text-xs truncate">{node.id}</p>
              </div>
              <div className="space-y-1">
                <p className="text-xs text-muted-foreground">Created At</p>
                <p className="text-sm font-medium">{formatDate(node.created_at)}</p>
              </div>
            </div>
          </div>
        </div>

        <div className="flex justify-end pt-4 border-t">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Close
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
