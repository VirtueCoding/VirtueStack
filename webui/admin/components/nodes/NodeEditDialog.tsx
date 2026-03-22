"use client";

import { useEffect } from "react";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Server, Network, Cpu, MemoryStick, HardDrive, Loader2, Shield } from "lucide-react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useToast } from "@/components/ui/use-toast";

export const editNodeSchema = z.object({
  grpc_address: z.string()
    .max(255, "gRPC address must be 255 characters or less")
    .optional(),
  total_vcpu: z.number().int().min(1, "Must be at least 1 vCPU").optional(),
  total_memory_mb: z.number().int().min(1024, "Must be at least 1024 MB").optional(),
  storage_backend: z.enum(["ceph", "qcow"]).optional(),
  storage_path: z.string().max(500, "Storage path must be 500 characters or less").optional(),
  ipmi_address: z.string().optional(),
});

export type EditNodeFormData = z.infer<typeof editNodeSchema>;

interface NodeEditDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  node: {
    id: string;
    hostname: string;
    grpc_address: string;
    total_vcpu: number;
    total_memory_mb: number;
    storage_backend: string;
    storage_path?: string;
  } | null;
  onSave: (data: EditNodeFormData) => Promise<void>;
  isSaving: boolean;
}

export function NodeEditDialog({ open, onOpenChange, node, onSave, isSaving }: NodeEditDialogProps) {
  const { toast } = useToast();

  const form = useForm<EditNodeFormData>({
    resolver: zodResolver(editNodeSchema),
    defaultValues: {
      grpc_address: "",
      total_vcpu: 32,
      total_memory_mb: 65536,
      storage_backend: "ceph",
      storage_path: "",
      ipmi_address: "",
    },
  });

  // Reset form when node changes
  useEffect(() => {
    if (node && open) {
      form.reset({
        grpc_address: node.grpc_address,
        total_vcpu: node.total_vcpu,
        total_memory_mb: node.total_memory_mb,
        storage_backend: node.storage_backend as "ceph" | "qcow",
        storage_path: node.storage_path || "",
        ipmi_address: "",
      });
    }
  }, [node, open, form]);

  const handleSubmit = async (data: EditNodeFormData) => {
    try {
      await onSave(data);
      onOpenChange(false);
    } catch (error) {
      toast({
        title: "Update Failed",
        description: error instanceof Error ? error.message : "Failed to update node",
        variant: "destructive",
      });
    }
  };

  if (!node) return null;

  const storageBackend = form.watch("storage_backend");

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Server className="h-5 w-5" />
            Edit Node: {node.hostname}
          </DialogTitle>
          <DialogDescription>
            Update node configuration. All fields are optional for partial updates.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-6 py-4">
          {/* Basic Info Section */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">Basic Information</h4>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label className="flex items-center gap-2">
                  <Server className="h-4 w-4 text-muted-foreground" />
                  Hostname
                </Label>
                <Input
                  value={node.hostname}
                  disabled
                  className="bg-muted"
                />
                <p className="text-xs text-muted-foreground">Hostname cannot be changed</p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="edit-grpc_address" className="flex items-center gap-2">
                  <Network className="h-4 w-4 text-muted-foreground" />
                  gRPC Address
                </Label>
                <Input
                  id="edit-grpc_address"
                  placeholder="e.g., node-01.example.com:50051"
                  {...form.register("grpc_address")}
                />
                {form.formState.errors.grpc_address && (
                  <p className="text-xs text-destructive">{form.formState.errors.grpc_address.message}</p>
                )}
                <p className="text-xs text-muted-foreground">Address where the node agent listens</p>
              </div>
            </div>
          </div>

          {/* Resources Section */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">Resources</h4>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="edit-total_vcpu" className="flex items-center gap-2">
                  <Cpu className="h-4 w-4 text-muted-foreground" />
                  Total vCPU
                </Label>
                <Input
                  id="edit-total_vcpu"
                  type="number"
                  min={1}
                  {...form.register("total_vcpu", { valueAsNumber: true })}
                />
                {form.formState.errors.total_vcpu && (
                  <p className="text-xs text-destructive">{form.formState.errors.total_vcpu.message}</p>
                )}
                <p className="text-xs text-muted-foreground">Total CPU cores available for VMs</p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="edit-total_memory_mb" className="flex items-center gap-2">
                  <MemoryStick className="h-4 w-4 text-muted-foreground" />
                  Total Memory (MB)
                </Label>
                <Input
                  id="edit-total_memory_mb"
                  type="number"
                  min={1024}
                  step={1024}
                  {...form.register("total_memory_mb", { valueAsNumber: true })}
                />
                {form.formState.errors.total_memory_mb && (
                  <p className="text-xs text-destructive">{form.formState.errors.total_memory_mb.message}</p>
                )}
                <p className="text-xs text-muted-foreground">1024 = 1 GB (e.g., 65536 = 64 GB)</p>
              </div>
            </div>
          </div>

          {/* Storage Section */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">Storage Configuration</h4>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="edit-storage_backend" className="flex items-center gap-2">
                  <HardDrive className="h-4 w-4 text-muted-foreground" />
                  Storage Backend
                </Label>
                <Select
                  value={form.watch("storage_backend")}
                  onValueChange={(value: "ceph" | "qcow") => form.setValue("storage_backend", value)}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="Select backend" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="ceph">Ceph (RBD)</SelectItem>
                    <SelectItem value="qcow">Local QCOW2</SelectItem>
                  </SelectContent>
                </Select>
                {form.formState.errors.storage_backend && (
                  <p className="text-xs text-destructive">{form.formState.errors.storage_backend.message}</p>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="edit-storage_path" className="flex items-center gap-2">
                  <HardDrive className="h-4 w-4 text-muted-foreground" />
                  Storage Path {storageBackend === "qcow" && <span className="text-destructive">*</span>}
                </Label>
                <Input
                  id="edit-storage_path"
                  placeholder="e.g., /var/lib/libvirt/images"
                  disabled={storageBackend === "ceph"}
                  {...form.register("storage_path")}
                />
                {form.formState.errors.storage_path && (
                  <p className="text-xs text-destructive">{form.formState.errors.storage_path.message}</p>
                )}
                <p className="text-xs text-muted-foreground">
                  {storageBackend === "ceph"
                    ? "Not applicable for Ceph backend"
                    : "Required for QCOW backend"}
                </p>
              </div>
            </div>
          </div>

          {/* IPMI Section */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">IPMI Configuration</h4>
            <p className="text-xs text-muted-foreground">
              Update IPMI address. To change credentials, use the dedicated IPMI management page.
            </p>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="edit-ipmi_address" className="flex items-center gap-2">
                  <Shield className="h-4 w-4 text-muted-foreground" />
                  IPMI Address
                </Label>
                <Input
                  id="edit-ipmi_address"
                  placeholder="e.g., 192.168.1.11"
                  {...form.register("ipmi_address")}
                />
                {form.formState.errors.ipmi_address && (
                  <p className="text-xs text-destructive">{form.formState.errors.ipmi_address.message}</p>
                )}
                <p className="text-xs text-muted-foreground">Out-of-band management IP address</p>
              </div>
            </div>
          </div>

          <DialogFooter className="pt-4">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={isSaving}>
              Cancel
            </Button>
            <Button type="submit" disabled={isSaving}>
              {isSaving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Save Changes
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}