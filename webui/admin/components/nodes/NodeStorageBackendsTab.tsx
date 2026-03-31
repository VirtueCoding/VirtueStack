"use client";

import { useState } from "react";
import { Button } from "@virtuestack/ui";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@virtuestack/ui";
import { Badge } from "@virtuestack/ui";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@virtuestack/ui";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@virtuestack/ui";
import { useToast } from "@virtuestack/ui";
import { Loader2, Plus, Trash2, AlertCircle, CheckCircle2, XCircle } from "lucide-react";

interface StorageBackend {
  id: string;
  name: string;
  type: "ceph" | "qcow" | "lvm";
  health_status?: "healthy" | "warning" | "critical";
  lvm_data_percent?: number;
  lvm_metadata_percent?: number;
}

interface NodeStorageBackendsTabProps {
  nodeId: string;
  assignedBackends: StorageBackend[];
  availableBackends: StorageBackend[];
  onAssign: (backendId: string) => Promise<void>;
  onUnassign: (backendId: string) => Promise<void>;
}

export function NodeStorageBackendsTab({
  nodeId,
  assignedBackends,
  availableBackends,
  onAssign,
  onUnassign,
}: NodeStorageBackendsTabProps) {
  const { toast } = useToast();
  const [isAssigning, setIsAssigning] = useState(false);
  const [isUnassigning, setIsUnassigning] = useState<string | null>(null);
  const [showAssignDialog, setShowAssignDialog] = useState(false);
  const [selectedBackendId, setSelectedBackendId] = useState<string>("");

  // Filter out already assigned backends from available options
  const assignableBackends = availableBackends.filter(
    (backend) => !assignedBackends.some((assigned) => assigned.id === backend.id)
  );

  const handleAssign = async () => {
    if (!selectedBackendId) {
      toast({
        title: "Error",
        description: "Please select a storage backend",
        variant: "destructive",
      });
      return;
    }

    setIsAssigning(true);
    try {
      await onAssign(selectedBackendId);
      toast({
        title: "Success",
        description: "Storage backend assigned successfully",
      });
      setShowAssignDialog(false);
      setSelectedBackendId("");
    } catch (error) {
      toast({
        title: "Error",
        description: error instanceof Error ? error.message : "Failed to assign storage backend",
        variant: "destructive",
      });
    } finally {
      setIsAssigning(false);
    }
  };

  const handleUnassign = async (backendId: string) => {
    setIsUnassigning(backendId);
    try {
      await onUnassign(backendId);
      toast({
        title: "Success",
        description: "Storage backend unassigned successfully",
      });
    } catch (error) {
      toast({
        title: "Error",
        description: error instanceof Error ? error.message : "Failed to unassign storage backend",
        variant: "destructive",
      });
    } finally {
      setIsUnassigning(null);
    }
  };

  const getHealthIcon = (status?: string) => {
    switch (status) {
      case "healthy":
        return <CheckCircle2 className="h-4 w-4 text-green-500" />;
      case "warning":
        return <AlertCircle className="h-4 w-4 text-yellow-500" />;
      case "critical":
        return <XCircle className="h-4 w-4 text-red-500" />;
      default:
        return null;
    }
  };

  const getBackendTypeBadge = (type: string) => {
    const colors = {
      ceph: "bg-purple-100 text-purple-800 border-purple-200",
      qcow: "bg-blue-100 text-blue-800 border-blue-200",
      lvm: "bg-orange-100 text-orange-800 border-orange-200",
    };
    return (
      <Badge variant="outline" className={colors[type as keyof typeof colors] || ""}>
        {type.toUpperCase()}
      </Badge>
    );
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-lg font-medium">Storage Backends</h3>
          <p className="text-sm text-muted-foreground">
            Manage storage backends assigned to this node
          </p>
        </div>
        {assignableBackends.length > 0 && (
          <Button onClick={() => setShowAssignDialog(true)} size="sm">
            <Plus className="h-4 w-4 mr-2" />
            Assign Backend
          </Button>
        )}
      </div>

      {assignedBackends.length === 0 ? (
        <Card>
          <CardContent className="flex flex-col items-center justify-center py-8">
            <AlertCircle className="h-12 w-12 text-muted-foreground mb-4" />
            <p className="text-muted-foreground text-center">
              No storage backends assigned to this node.
              <br />
              Assign a storage backend to enable VM provisioning.
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4">
          {assignedBackends.map((backend) => (
            <Card key={backend.id}>
              <CardHeader className="pb-2">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <CardTitle className="text-base">{backend.name}</CardTitle>
                    {getBackendTypeBadge(backend.type)}
                  </div>
                  <div className="flex items-center gap-2">
                    {getHealthIcon(backend.health_status)}
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleUnassign(backend.id)}
                      disabled={isUnassigning === backend.id}
                    >
                      {isUnassigning === backend.id ? (
                        <Loader2 className="h-4 w-4 animate-spin" />
                      ) : (
                        <Trash2 className="h-4 w-4 text-destructive" />
                      )}
                    </Button>
                  </div>
                </div>
              </CardHeader>
              <CardContent>
                {backend.type === "lvm" && (
                  <div className="space-y-2 text-sm">
                    <div className="flex items-center justify-between">
                      <span className="text-muted-foreground">Data Usage:</span>
                      <span
                        className={
                          (backend.lvm_data_percent || 0) >= 90
                            ? "text-red-500 font-medium"
                            : (backend.lvm_data_percent || 0) >= 80
                              ? "text-yellow-500"
                              : ""
                        }
                      >
                        {backend.lvm_data_percent?.toFixed(1) || "N/A"}%
                      </span>
                    </div>
                    <div className="flex items-center justify-between">
                      <span className="text-muted-foreground">Metadata Usage:</span>
                      <span
                        className={
                          (backend.lvm_metadata_percent || 0) >= 70
                            ? "text-red-500 font-medium"
                            : (backend.lvm_metadata_percent || 0) >= 50
                              ? "text-yellow-500"
                              : ""
                        }
                      >
                        {backend.lvm_metadata_percent?.toFixed(1) || "N/A"}%
                      </span>
                    </div>
                    {(backend.lvm_data_percent !== undefined || backend.lvm_metadata_percent !== undefined) && (
                      <div className="space-y-1">
                        <div className="h-2 bg-secondary rounded-full overflow-hidden">
                          <div
                            className={`h-full ${(backend.lvm_data_percent || 0) >= 90 ? "bg-red-500" : (backend.lvm_data_percent || 0) >= 80 ? "bg-yellow-500" : "bg-green-500"}`}
                            style={{ width: `${Math.min(backend.lvm_data_percent || 0, 100)}%` }}
                          />
                        </div>
                      </div>
                    )}
                  </div>
                )}
                {backend.type === "ceph" && (
                  <p className="text-sm text-muted-foreground">
                    Ceph distributed storage - health monitored by cluster
                  </p>
                )}
                {backend.type === "qcow" && (
                  <p className="text-sm text-muted-foreground">
                    Local QCOW2 file storage
                  </p>
                )}
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      <Dialog open={showAssignDialog} onOpenChange={setShowAssignDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Assign Storage Backend</DialogTitle>
            <DialogDescription>
              Select a storage backend to assign to this node. This will allow the node
              to provision VMs using this storage backend.
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            <Select value={selectedBackendId} onValueChange={setSelectedBackendId}>
              <SelectTrigger>
                <SelectValue placeholder="Select a storage backend" />
              </SelectTrigger>
              <SelectContent>
                {assignableBackends.map((backend) => (
                  <SelectItem key={backend.id} value={backend.id}>
                    <div className="flex items-center gap-2">
                      <span>{backend.name}</span>
                      {getBackendTypeBadge(backend.type)}
                    </div>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowAssignDialog(false)}>
              Cancel
            </Button>
            <Button onClick={handleAssign} disabled={isAssigning || !selectedBackendId}>
              {isAssigning && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
              Assign
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
