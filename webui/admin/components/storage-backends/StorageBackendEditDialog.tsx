"use client";

import { useCallback, useEffect, useState } from "react";
import { z } from "zod";
import { Button } from "@virtuestack/ui";
import { Input } from "@virtuestack/ui";
import { Label } from "@virtuestack/ui";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@virtuestack/ui";
import { Badge } from "@virtuestack/ui";
import { HardDrive, Loader2, Trash2 } from "lucide-react";
import { useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useToast } from "@virtuestack/ui";
import { adminNodesApi, type StorageBackend, type Node } from "@/lib/api-client";

const editStorageBackendSchema = z.object({
  name: z.string()
    .min(1, "Name is required")
    .max(100, "Name must be 100 characters or less"),
  // Ceph fields
  ceph_pool: z.string().max(100).optional(),
  ceph_user: z.string().max(100).optional(),
  ceph_monitors: z.string().max(500).optional(),
  ceph_keyring_path: z.string().max(500).optional(),
  // QCOW fields
  storage_path: z.string().max(500).optional(),
  // LVM fields
  lvm_volume_group: z.string().max(100).optional(),
  lvm_thin_pool: z.string().max(100).optional(),
  lvm_data_percent_threshold: z.coerce.number().min(1).max(100).optional(),
  lvm_metadata_percent_threshold: z.coerce.number().min(1).max(100).optional(),
  // Node assignments
  node_ids: z.array(z.string()).optional(),
});

export type EditStorageBackendFormData = z.infer<typeof editStorageBackendSchema>;

interface StorageBackendEditDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  backend: StorageBackend | null;
  onSave: (data: EditStorageBackendFormData) => Promise<void>;
  isSaving: boolean;
  onDelete: (backend: StorageBackend) => void;
}

export function StorageBackendEditDialog({
  open,
  onOpenChange,
  backend,
  onSave,
  isSaving,
  onDelete,
}: StorageBackendEditDialogProps) {
  const { toast } = useToast();
  const [nodes, setNodes] = useState<Node[]>([]);
  const [loadingNodes, setLoadingNodes] = useState(true);

  const form = useForm<EditStorageBackendFormData>({
    resolver: zodResolver(editStorageBackendSchema),
    defaultValues: {
      name: "",
      ceph_pool: "",
      ceph_user: "",
      ceph_monitors: "",
      ceph_keyring_path: "",
      storage_path: "",
      lvm_volume_group: "",
      lvm_thin_pool: "",
      node_ids: [],
    },
  });
  const selectedNodeIds = useWatch({ control: form.control, name: "node_ids" }) || [];
  const loadNodes = useCallback(async () => {
    setLoadingNodes(true);
    try {
      const response = await adminNodesApi.getNodes();
      setNodes(response.data || []);
    } catch {
      toast({
        title: "Error",
        description: "Failed to load nodes.",
        variant: "destructive",
      });
    } finally {
      setLoadingNodes(false);
    }
  }, [toast]);

  // Reset form when backend changes
  useEffect(() => {
    if (backend && open) {
      form.reset({
        name: backend.name,
        ceph_pool: backend.ceph_pool || "",
        ceph_user: backend.ceph_user || "",
        ceph_monitors: backend.ceph_monitors || "",
        ceph_keyring_path: backend.ceph_keyring_path || "",
        storage_path: backend.storage_path || "",
        lvm_volume_group: backend.lvm_volume_group || "",
        lvm_thin_pool: backend.lvm_thin_pool || "",
        lvm_data_percent_threshold: backend.lvm_data_percent_threshold || 95,
        lvm_metadata_percent_threshold: backend.lvm_metadata_percent_threshold || 70,
        node_ids: backend.nodes?.map((n) => n.node_id) || [],
      });
    }
  }, [backend, open, form]);

  // Fetch nodes when dialog opens
  useEffect(() => {
    if (open) {
      void loadNodes();
    }
  }, [loadNodes, open]);

  const handleSubmit = async (data: EditStorageBackendFormData) => {
    try {
      await onSave(data);
      onOpenChange(false);
    } catch (error) {
      toast({
        title: "Update Failed",
        description: error instanceof Error ? error.message : "Failed to update storage backend",
        variant: "destructive",
      });
    }
  };

  const handleDelete = () => {
    if (backend) {
      onDelete(backend);
    }
  };

  const toggleNode = (nodeId: string, checked: boolean) => {
    form.setValue(
      "node_ids",
      checked ? [...selectedNodeIds, nodeId] : selectedNodeIds.filter((id) => id !== nodeId)
    );
  };

  if (!backend) return null;

  const getTypeBadgeVariant = (type: string): "default" | "secondary" => {
    return type === "ceph" ? "default" : "secondary";
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <HardDrive className="h-5 w-5" />
            Edit Storage Backend
          </DialogTitle>
          <DialogDescription>
            Modify storage backend configuration. Type cannot be changed after creation.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-6 py-4">
          {/* Basic Info */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
              Basic Information
            </h4>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="edit-name" className="flex items-center gap-2">
                  <HardDrive className="h-4 w-4 text-muted-foreground" />
                  Name <span className="text-destructive">*</span>
                </Label>
                <Input
                  id="edit-name"
                  placeholder="e.g., ceph-prod, local-qcow-ssd"
                  {...form.register("name")}
                />
                {form.formState.errors.name && (
                  <p className="text-xs text-destructive">{form.formState.errors.name.message}</p>
                )}
              </div>
              <div className="space-y-2">
                <Label className="flex items-center gap-2">
                  <HardDrive className="h-4 w-4 text-muted-foreground" />
                  Type (Read-only)
                </Label>
                <div className="pt-2">
                  <Badge variant={getTypeBadgeVariant(backend.type)} className="uppercase">
                    {backend.type}
                  </Badge>
                </div>
                <p className="text-xs text-muted-foreground">
                  Storage type cannot be changed after creation.
                </p>
              </div>
            </div>
          </div>

          {/* Health Status */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
              Health Status
            </h4>
            <div className="flex items-center gap-4">
              <Badge
                variant={
                  backend.health_status === "healthy"
                    ? "success"
                    : backend.health_status === "warning"
                    ? "warning"
                    : backend.health_status === "critical"
                    ? "destructive"
                    : "secondary"
                }
                className="capitalize"
              >
                {backend.health_status}
              </Badge>
              {backend.health_message && (
                <span className="text-sm text-muted-foreground">
                  {backend.health_message}
                </span>
              )}
            </div>
            {backend.type === "lvm" && backend.lvm_data_percent !== undefined && (
              <div className="text-sm text-muted-foreground">
                Data: {backend.lvm_data_percent.toFixed(1)}%
                {backend.lvm_metadata_percent !== undefined && (
                  <span className="ml-4">Metadata: {backend.lvm_metadata_percent.toFixed(1)}%</span>
                )}
              </div>
            )}
          </div>

          {/* Ceph Configuration */}
          {backend.type === "ceph" && (
            <div className="space-y-4">
              <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
                Ceph Configuration
              </h4>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="edit-ceph_pool">Ceph Pool</Label>
                  <Input
                    id="edit-ceph_pool"
                    placeholder="e.g., vms"
                    {...form.register("ceph_pool")}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="edit-ceph_user">Ceph User</Label>
                  <Input
                    id="edit-ceph_user"
                    placeholder="e.g., admin"
                    {...form.register("ceph_user")}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="edit-ceph_monitors">Ceph Monitors</Label>
                  <Input
                    id="edit-ceph_monitors"
                    placeholder="e.g., 10.0.0.10:6789,10.0.0.11:6789"
                    {...form.register("ceph_monitors")}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="edit-ceph_keyring_path">Keyring Path</Label>
                  <Input
                    id="edit-ceph_keyring_path"
                    placeholder="e.g., /etc/ceph/ceph.keyring"
                    {...form.register("ceph_keyring_path")}
                  />
                </div>
              </div>
            </div>
          )}

          {/* QCOW Configuration */}
          {backend.type === "qcow" && (
            <div className="space-y-4">
              <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
                QCOW Configuration
              </h4>
              <div className="space-y-2">
                <Label htmlFor="edit-storage_path">Storage Path</Label>
                <Input
                  id="edit-storage_path"
                  placeholder="e.g., /var/lib/virtuestack/vms"
                  {...form.register("storage_path")}
                />
              </div>
            </div>
          )}

          {/* LVM Configuration */}
          {backend.type === "lvm" && (
            <div className="space-y-4">
              <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
                LVM Thin Pool Configuration
              </h4>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="edit-lvm_volume_group">Volume Group</Label>
                  <Input
                    id="edit-lvm_volume_group"
                    placeholder="e.g., vgvs"
                    {...form.register("lvm_volume_group")}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="edit-lvm_thin_pool">Thin Pool</Label>
                  <Input
                    id="edit-lvm_thin_pool"
                    placeholder="e.g., thinpool"
                    {...form.register("lvm_thin_pool")}
                  />
                </div>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4 pt-2">
                <div className="space-y-2">
                  <Label htmlFor="edit-lvm_data_percent_threshold">
                    Data Usage Alert Threshold (%)
                  </Label>
                  <Input
                    id="edit-lvm_data_percent_threshold"
                    type="number"
                    min={1}
                    max={100}
                    placeholder="95"
                    {...form.register("lvm_data_percent_threshold")}
                  />
                  <p className="text-xs text-muted-foreground">
                    Alert when thin pool data usage exceeds this percentage
                  </p>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="edit-lvm_metadata_percent_threshold">
                    Metadata Usage Alert Threshold (%)
                  </Label>
                  <Input
                    id="edit-lvm_metadata_percent_threshold"
                    type="number"
                    min={1}
                    max={100}
                    placeholder="70"
                    {...form.register("lvm_metadata_percent_threshold")}
                  />
                  <p className="text-xs text-muted-foreground">
                    Alert when thin pool metadata usage exceeds this percentage
                  </p>
                </div>
              </div>
            </div>
          )}

          {/* Node Assignment */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
              Node Assignment
            </h4>
            {loadingNodes ? (
              <div className="flex justify-center py-4">
                <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
              </div>
            ) : nodes.length === 0 ? (
              <div className="rounded-md border border-dashed p-4 text-center text-sm text-muted-foreground">
                No nodes available.
              </div>
            ) : (
              <div className="border rounded-md p-2 max-h-48 overflow-auto">
                {nodes.map((node) => (
                  <label
                    key={node.id}
                    className="flex items-center gap-3 p-2 hover:bg-muted/50 rounded cursor-pointer"
                  >
                    <Checkbox
                      checked={selectedNodeIds.includes(node.id)}
                      onCheckedChange={(checked) => toggleNode(node.id, checked === true)}
                    />
                    <div className="flex-1">
                      <span className="font-medium">{node.hostname}</span>
                      <Badge
                        variant={node.status === "online" ? "success" : "destructive"}
                        className="ml-2 text-xs"
                      >
                        {node.status}
                      </Badge>
                      <span className="ml-2 text-xs text-muted-foreground">
                        {node.location}
                      </span>
                    </div>
                  </label>
                ))}
              </div>
            )}
          </div>

          <DialogFooter className="pt-4 flex justify-between">
            <Button
              type="button"
              variant="destructive"
              onClick={handleDelete}
              disabled={isSaving}
              className="mr-auto"
            >
              <Trash2 className="mr-2 h-4 w-4" />
              Delete
            </Button>
            <div className="flex gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
                disabled={isSaving}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={isSaving}>
                {isSaving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Save Changes
              </Button>
            </div>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
