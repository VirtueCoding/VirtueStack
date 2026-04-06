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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@virtuestack/ui";
import { HardDrive, Loader2 } from "lucide-react";
import { useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useToast } from "@virtuestack/ui";
import { adminNodesApi, type Node } from "@/lib/api-client";

const createStorageBackendSchema = z.object({
  name: z.string()
    .min(1, "Name is required")
    .max(100, "Name must be 100 characters or less"),
  type: z.enum(["ceph", "qcow", "lvm"], {
    required_error: "Type is required",
  }),
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
  node_ids: z.array(z.string()).default([]),
}).refine((data) => {
  // Type-specific validation
  if (data.type === "ceph" && !data.ceph_pool) {
    return false;
  }
  if (data.type === "qcow" && !data.storage_path) {
    return false;
  }
  if (data.type === "lvm" && (!data.lvm_volume_group || !data.lvm_thin_pool)) {
    return false;
  }
  return true;
}, {
  message: "Required fields are missing for the selected storage type",
  path: ["type"],
});

export type CreateStorageBackendFormData = z.infer<typeof createStorageBackendSchema>;

interface StorageBackendCreateDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreate: (data: CreateStorageBackendFormData) => Promise<void>;
  isCreating: boolean;
}

const defaultValues: CreateStorageBackendFormData = {
  name: "",
  type: "ceph",
  ceph_pool: "",
  ceph_user: "",
  ceph_monitors: "",
  ceph_keyring_path: "",
  storage_path: "",
  lvm_volume_group: "",
  lvm_thin_pool: "",
  lvm_data_percent_threshold: 95,
  lvm_metadata_percent_threshold: 70,
  node_ids: [],
};

export function StorageBackendCreateDialog({
  open,
  onOpenChange,
  onCreate,
  isCreating,
}: StorageBackendCreateDialogProps) {
  const { toast } = useToast();
  const [nodes, setNodes] = useState<Node[]>([]);
  const [loadingNodes, setLoadingNodes] = useState(true);

  const form = useForm<CreateStorageBackendFormData>({
    resolver: zodResolver(createStorageBackendSchema),
    defaultValues,
  });
  const storageType = useWatch({ control: form.control, name: "type" });
  const selectedNodeIds = useWatch({ control: form.control, name: "node_ids" });
  const loadNodes = useCallback(async () => {
    setLoadingNodes(true);
    try {
      const response = await adminNodesApi.getNodes();
      setNodes((response.data || []).filter((n) => n.status === "online"));
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

  // Fetch nodes when dialog opens
  useEffect(() => {
    if (open) {
      form.reset(defaultValues);
      void loadNodes();
    }
  }, [form, loadNodes, open]);

  const handleSubmit = async (data: CreateStorageBackendFormData) => {
    try {
      await onCreate(data);
      onOpenChange(false);
    } catch (error) {
      toast({
        title: "Creation Failed",
        description: error instanceof Error ? error.message : "Failed to create storage backend",
        variant: "destructive",
      });
    }
  };

  const toggleNode = (nodeId: string, checked: boolean) => {
    const current = selectedNodeIds || [];
    form.setValue(
      "node_ids",
      checked ? [...current, nodeId] : current.filter((id) => id !== nodeId)
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <HardDrive className="h-5 w-5" />
            Create Storage Backend
          </DialogTitle>
          <DialogDescription>
            Configure a new storage backend for VM disk provisioning.
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
                <Label htmlFor="name" className="flex items-center gap-2">
                  <HardDrive className="h-4 w-4 text-muted-foreground" />
                  Name <span className="text-destructive">*</span>
                </Label>
                <Input
                  id="name"
                  placeholder="e.g., ceph-prod, local-qcow-ssd"
                  {...form.register("name")}
                />
                {form.formState.errors.name && (
                  <p className="text-xs text-destructive">{form.formState.errors.name.message}</p>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="type" className="flex items-center gap-2">
                  <HardDrive className="h-4 w-4 text-muted-foreground" />
                  Type <span className="text-destructive">*</span>
                </Label>
                <Select
                  value={storageType}
                  onValueChange={(value: "ceph" | "qcow" | "lvm") => form.setValue("type", value)}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="Select type" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="ceph">Ceph (Shared Storage)</SelectItem>
                    <SelectItem value="qcow">QCOW2 (Local Files)</SelectItem>
                    <SelectItem value="lvm">LVM Thin (Local Block)</SelectItem>
                  </SelectContent>
                </Select>
                {form.formState.errors.type && (
                  <p className="text-xs text-destructive">{form.formState.errors.type.message}</p>
                )}
              </div>
            </div>
          </div>

          {/* Ceph Configuration */}
          {storageType === "ceph" && (
            <div className="space-y-4">
              <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
                Ceph Configuration
              </h4>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="ceph_pool">
                    Ceph Pool <span className="text-destructive">*</span>
                  </Label>
                  <Input
                    id="ceph_pool"
                    placeholder="e.g., vms"
                    {...form.register("ceph_pool")}
                  />
                  <p className="text-xs text-muted-foreground">RBD pool name for VM disks</p>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="ceph_user">Ceph User</Label>
                  <Input
                    id="ceph_user"
                    placeholder="e.g., admin"
                    {...form.register("ceph_user")}
                  />
                  <p className="text-xs text-muted-foreground">Ceph client user (optional)</p>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="ceph_monitors">Ceph Monitors</Label>
                  <Input
                    id="ceph_monitors"
                    placeholder="e.g., 10.0.0.10:6789,10.0.0.11:6789"
                    {...form.register("ceph_monitors")}
                  />
                  <p className="text-xs text-muted-foreground">Comma-separated monitor addresses</p>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="ceph_keyring_path">Keyring Path</Label>
                  <Input
                    id="ceph_keyring_path"
                    placeholder="e.g., /etc/ceph/ceph.keyring"
                    {...form.register("ceph_keyring_path")}
                  />
                  <p className="text-xs text-muted-foreground">Path to Ceph keyring file</p>
                </div>
              </div>
            </div>
          )}

          {/* QCOW Configuration */}
          {storageType === "qcow" && (
            <div className="space-y-4">
              <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
                QCOW Configuration
              </h4>
              <div className="space-y-2">
                <Label htmlFor="storage_path">
                  Storage Path <span className="text-destructive">*</span>
                </Label>
                <Input
                  id="storage_path"
                  placeholder="e.g., /var/lib/virtuestack/vms"
                  {...form.register("storage_path")}
                />
                <p className="text-xs text-muted-foreground">
                  Base directory for QCOW2 disk files. Must exist and be writable.
                </p>
              </div>
            </div>
          )}

          {/* LVM Configuration */}
          {storageType === "lvm" && (
            <div className="space-y-4">
              <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
                LVM Thin Pool Configuration
              </h4>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="lvm_volume_group">
                    Volume Group <span className="text-destructive">*</span>
                  </Label>
                  <Input
                    id="lvm_volume_group"
                    placeholder="e.g., vgvs"
                    {...form.register("lvm_volume_group")}
                  />
                  <p className="text-xs text-muted-foreground">LVM volume group name</p>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="lvm_thin_pool">
                    Thin Pool <span className="text-destructive">*</span>
                  </Label>
                  <Input
                    id="lvm_thin_pool"
                    placeholder="e.g., thinpool"
                    {...form.register("lvm_thin_pool")}
                  />
                  <p className="text-xs text-muted-foreground">Thin pool LV name within the VG</p>
                </div>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4 pt-2">
                <div className="space-y-2">
                  <Label htmlFor="lvm_data_percent_threshold">
                    Data Usage Alert Threshold (%)
                  </Label>
                  <Input
                    id="lvm_data_percent_threshold"
                    type="number"
                    min={1}
                    max={100}
                    placeholder="95"
                    {...form.register("lvm_data_percent_threshold")}
                  />
                  <p className="text-xs text-muted-foreground">
                    Alert when thin pool data usage exceeds this percentage (default: 95)
                  </p>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="lvm_metadata_percent_threshold">
                    Metadata Usage Alert Threshold (%)
                  </Label>
                  <Input
                    id="lvm_metadata_percent_threshold"
                    type="number"
                    min={1}
                    max={100}
                    placeholder="70"
                    {...form.register("lvm_metadata_percent_threshold")}
                  />
                  <p className="text-xs text-muted-foreground">
                    Alert when thin pool metadata usage exceeds this percentage (default: 70)
                  </p>
                </div>
              </div>
              <div className="rounded-md bg-muted p-3 text-sm">
                <strong>Note:</strong> The thin pool must be pre-created on all assigned nodes.
                VMs use thin-provisioned logical volumes for efficient storage.
              </div>
            </div>
          )}

          {/* Node Assignment */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
              Node Assignment
            </h4>
            <p className="text-xs text-muted-foreground">
              Select nodes where this storage backend is available.
              Only online nodes are shown.
            </p>
            {loadingNodes ? (
              <div className="flex justify-center py-4">
                <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
              </div>
            ) : nodes.length === 0 ? (
              <div className="rounded-md border border-dashed p-4 text-center text-sm text-muted-foreground">
                No online nodes available. Register nodes first.
              </div>
            ) : (
              <div className="border rounded-md p-2 max-h-48 overflow-auto">
                {nodes.map((node) => (
                  <label
                    key={node.id}
                    className="flex items-center gap-3 p-2 hover:bg-muted/50 rounded cursor-pointer"
                  >
                    <Checkbox
                      checked={selectedNodeIds?.includes(node.id) || false}
                      onCheckedChange={(checked) => toggleNode(node.id, checked === true)}
                    />
                    <div className="flex-1">
                      <span className="font-medium">{node.hostname}</span>
                      <span className="ml-2 text-xs text-muted-foreground">
                        {node.location}
                      </span>
                    </div>
                  </label>
                ))}
              </div>
            )}
          </div>

          <DialogFooter className="pt-4">
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={isCreating}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={isCreating}>
              {isCreating && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Create Storage Backend
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
