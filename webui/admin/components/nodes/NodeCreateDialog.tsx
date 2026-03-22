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

export const createNodeSchema = z.object({
  hostname: z.string()
    .min(1, "Hostname is required")
    .max(255, "Hostname must be 255 characters or less"),
  grpc_address: z.string()
    .min(1, "gRPC address is required")
    .max(255, "gRPC address must be 255 characters or less"),
  management_ip: z.string()
    .min(1, "Management IP is required")
    .refine((val) => {
      // Basic IP address validation
      const ipv4Regex = /^(\d{1,3}\.){3}\d{1,3}$/;
      const ipv6Regex = /^([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$/;
      return ipv4Regex.test(val) || ipv6Regex.test(val);
    }, "Must be a valid IP address"),
  total_vcpu: z.number().int().min(1, "Must be at least 1 vCPU"),
  total_memory_mb: z.number().int().min(1024, "Must be at least 1024 MB"),
  storage_backend: z.enum(["ceph", "qcow"], {
    required_error: "Storage backend is required",
  }),
  storage_path: z.string().max(500, "Storage path must be 500 characters or less").optional(),
  ceph_pool: z.string().max(100, "Ceph pool must be 100 characters or less").optional(),
  ipmi_address: z.string().optional(),
  ipmi_username: z.string().optional(),
  ipmi_password: z.string().optional(),
}).refine((data) => {
  // If storage_backend is qcow, storage_path is required
  if (data.storage_backend === "qcow" && !data.storage_path) {
    return false;
  }
  return true;
}, {
  message: "Storage path is required when storage backend is 'qcow'",
  path: ["storage_path"],
});

export type CreateNodeFormData = z.infer<typeof createNodeSchema>;

interface NodeCreateDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreate: (data: CreateNodeFormData) => Promise<void>;
  isCreating: boolean;
}

const defaultValues: CreateNodeFormData = {
  hostname: "",
  grpc_address: "",
  management_ip: "",
  total_vcpu: 32,
  total_memory_mb: 65536,
  storage_backend: "ceph",
  storage_path: "",
  ceph_pool: "",
  ipmi_address: "",
  ipmi_username: "",
  ipmi_password: "",
};

export function NodeCreateDialog({ open, onOpenChange, onCreate, isCreating }: NodeCreateDialogProps) {
  const { toast } = useToast();

  const form = useForm<CreateNodeFormData>({
    resolver: zodResolver(createNodeSchema),
    defaultValues,
  });

  // Reset form when dialog opens
  useEffect(() => {
    if (open) {
      form.reset(defaultValues);
    }
  }, [open, form]);

  const handleSubmit = async (data: CreateNodeFormData) => {
    try {
      await onCreate(data);
      onOpenChange(false);
    } catch (error) {
      toast({
        title: "Creation Failed",
        description: error instanceof Error ? error.message : "Failed to register node",
        variant: "destructive",
      });
    }
  };

  const storageBackend = form.watch("storage_backend");

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Server className="h-5 w-5" />
            Register New Node
          </DialogTitle>
          <DialogDescription>
            Register a new hypervisor node to the cluster. The node agent must be running on the target machine.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-6 py-4">
          {/* Basic Info Section */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">Basic Information</h4>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="hostname" className="flex items-center gap-2">
                  <Server className="h-4 w-4 text-muted-foreground" />
                  Hostname <span className="text-destructive">*</span>
                </Label>
                <Input
                  id="hostname"
                  placeholder="e.g., node-01.example.com"
                  {...form.register("hostname")}
                />
                {form.formState.errors.hostname && (
                  <p className="text-xs text-destructive">{form.formState.errors.hostname.message}</p>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="grpc_address" className="flex items-center gap-2">
                  <Network className="h-4 w-4 text-muted-foreground" />
                  gRPC Address <span className="text-destructive">*</span>
                </Label>
                <Input
                  id="grpc_address"
                  placeholder="e.g., node-01.example.com:50051"
                  {...form.register("grpc_address")}
                />
                {form.formState.errors.grpc_address && (
                  <p className="text-xs text-destructive">{form.formState.errors.grpc_address.message}</p>
                )}
                <p className="text-xs text-muted-foreground">Address where the node agent listens</p>
              </div>
            </div>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="management_ip" className="flex items-center gap-2">
                  <Network className="h-4 w-4 text-muted-foreground" />
                  Management IP <span className="text-destructive">*</span>
                </Label>
                <Input
                  id="management_ip"
                  placeholder="e.g., 192.168.1.10"
                  {...form.register("management_ip")}
                />
                {form.formState.errors.management_ip && (
                  <p className="text-xs text-destructive">{form.formState.errors.management_ip.message}</p>
                )}
                <p className="text-xs text-muted-foreground">IP address for management and migrations</p>
              </div>
            </div>
          </div>

          {/* Resources Section */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">Resources</h4>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="total_vcpu" className="flex items-center gap-2">
                  <Cpu className="h-4 w-4 text-muted-foreground" />
                  Total vCPU <span className="text-destructive">*</span>
                </Label>
                <Input
                  id="total_vcpu"
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
                <Label htmlFor="total_memory_mb" className="flex items-center gap-2">
                  <MemoryStick className="h-4 w-4 text-muted-foreground" />
                  Total Memory (MB) <span className="text-destructive">*</span>
                </Label>
                <Input
                  id="total_memory_mb"
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
                <Label htmlFor="storage_backend" className="flex items-center gap-2">
                  <HardDrive className="h-4 w-4 text-muted-foreground" />
                  Storage Backend <span className="text-destructive">*</span>
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
                <Label htmlFor="ceph_pool" className="flex items-center gap-2">
                  <HardDrive className="h-4 w-4 text-muted-foreground" />
                  Ceph Pool
                </Label>
                <Input
                  id="ceph_pool"
                  placeholder="e.g., vms"
                  disabled={storageBackend === "qcow"}
                  {...form.register("ceph_pool")}
                />
                {form.formState.errors.ceph_pool && (
                  <p className="text-xs text-destructive">{form.formState.errors.ceph_pool.message}</p>
                )}
                <p className="text-xs text-muted-foreground">
                  {storageBackend === "qcow" ? "Not applicable for QCOW backend" : "Ceph pool name for VM disks"}
                </p>
              </div>
            </div>
            <div className="grid grid-cols-1 gap-4">
              <div className="space-y-2">
                <Label htmlFor="storage_path" className="flex items-center gap-2">
                  <HardDrive className="h-4 w-4 text-muted-foreground" />
                  Storage Path {storageBackend === "qcow" && <span className="text-destructive">*</span>}
                </Label>
                <Input
                  id="storage_path"
                  placeholder="e.g., /var/lib/libvirt/images"
                  disabled={storageBackend === "ceph"}
                  {...form.register("storage_path")}
                />
                {form.formState.errors.storage_path && (
                  <p className="text-xs text-destructive">{form.formState.errors.storage_path.message}</p>
                )}
                <p className="text-xs text-muted-foreground">
                  {storageBackend === "ceph"
                    ? "Not applicable for Ceph backend (uses cluster-configured path)"
                    : "Base directory for QCOW2 disk files"}
                </p>
              </div>
            </div>
          </div>

          {/* IPMI Section */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">IPMI Configuration (Optional)</h4>
            <p className="text-xs text-muted-foreground">
              IPMI credentials enable out-of-band management for power control and hardware monitoring.
            </p>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div className="space-y-2">
                <Label htmlFor="ipmi_address" className="flex items-center gap-2">
                  <Shield className="h-4 w-4 text-muted-foreground" />
                  IPMI Address
                </Label>
                <Input
                  id="ipmi_address"
                  placeholder="e.g., 192.168.1.11"
                  {...form.register("ipmi_address")}
                />
                {form.formState.errors.ipmi_address && (
                  <p className="text-xs text-destructive">{form.formState.errors.ipmi_address.message}</p>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="ipmi_username" className="flex items-center gap-2">
                  <Shield className="h-4 w-4 text-muted-foreground" />
                  IPMI Username
                </Label>
                <Input
                  id="ipmi_username"
                  placeholder="Username"
                  autoComplete="off"
                  {...form.register("ipmi_username")}
                />
                {form.formState.errors.ipmi_username && (
                  <p className="text-xs text-destructive">{form.formState.errors.ipmi_username.message}</p>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="ipmi_password" className="flex items-center gap-2">
                  <Shield className="h-4 w-4 text-muted-foreground" />
                  IPMI Password
                </Label>
                <Input
                  id="ipmi_password"
                  type="password"
                  placeholder="Password"
                  autoComplete="new-password"
                  {...form.register("ipmi_password")}
                />
                {form.formState.errors.ipmi_password && (
                  <p className="text-xs text-destructive">{form.formState.errors.ipmi_password.message}</p>
                )}
              </div>
            </div>
          </div>

          <DialogFooter className="pt-4">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={isCreating}>
              Cancel
            </Button>
            <Button type="submit" disabled={isCreating}>
              {isCreating && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Register Node
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}