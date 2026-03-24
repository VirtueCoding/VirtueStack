"use client";

import { useEffect } from "react";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
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
import { Cpu, MemoryStick, HardDrive, Network, DollarSign, Loader2, Camera, Archive, Disc, Server, Hash, Activity, Zap } from "lucide-react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useToast } from "@/components/ui/use-toast";

export const createPlanSchema = z.object({
  name: z.string().min(1, "Name is required").max(100, "Name must be 100 characters or less"),
  slug: z.string()
    .min(1, "Slug is required")
    .max(100, "Slug must be 100 characters or less")
    .regex(/^[a-z0-9]+(?:-[a-z0-9]+)*$/, "Slug must be lowercase alphanumeric with hyphens"),
  vcpu: z.number().int().min(1, "Must be at least 1 vCPU"),
  memory_mb: z.number().int().min(512, "Must be at least 512 MB"),
  disk_gb: z.number().int().min(10, "Must be at least 10 GB"),
  bandwidth_limit_gb: z.number().int().min(0, "Must be 0 or greater").optional(),
  port_speed_mbps: z.number().int().min(1, "Must be at least 1 Mbps"),
  price_monthly: z.number().int().min(0, "Must be 0 or greater").optional(),
  price_hourly: z.number().int().min(0, "Must be 0 or greater").optional(),
  storage_backend: z.enum(["ceph", "qcow", "lvm"]).optional(),
  is_active: z.boolean().optional(),
  sort_order: z.number().int().min(0, "Must be 0 or greater").optional(),
  snapshot_limit: z.number().int().min(0, "Must be 0 or greater").optional(),
  backup_limit: z.number().int().min(0, "Must be 0 or greater").optional(),
  iso_upload_limit: z.number().int().min(0, "Must be 0 or greater").optional(),
});

export type CreatePlanFormData = z.infer<typeof createPlanSchema>;

interface PlanCreateDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreate: (data: CreatePlanFormData) => Promise<void>;
  isCreating: boolean;
}

const defaultValues: CreatePlanFormData = {
  name: "",
  slug: "",
  vcpu: 1,
  memory_mb: 1024,
  disk_gb: 20,
  port_speed_mbps: 1000,
  bandwidth_limit_gb: 0,
  price_monthly: 0,
  price_hourly: 0,
  storage_backend: "ceph",
  is_active: true,
  sort_order: 0,
  snapshot_limit: 2,
  backup_limit: 2,
  iso_upload_limit: 2,
};

export function PlanCreateDialog({ open, onOpenChange, onCreate, isCreating }: PlanCreateDialogProps) {
  const { toast } = useToast();

  const form = useForm<CreatePlanFormData>({
    resolver: zodResolver(createPlanSchema),
    defaultValues,
  });

  // Reset form when dialog opens
  useEffect(() => {
    if (open) {
      form.reset(defaultValues);
    }
  }, [open, form]);

  const handleSubmit = async (data: CreatePlanFormData) => {
    try {
      await onCreate(data);
      onOpenChange(false);
    } catch (error) {
      toast({
        title: "Creation Failed",
        description: error instanceof Error ? error.message : "Failed to create plan",
        variant: "destructive",
      });
    }
  };

  // Auto-generate slug from name
  const handleNameChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const name = e.target.value;
    form.setValue("name", name);
    // Auto-generate slug: lowercase, replace spaces and non-alphanumeric with hyphens
    const slug = name
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-|-$/g, "")
      .substring(0, 100);
    form.setValue("slug", slug);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Server className="h-5 w-5" />
            Create New Plan
          </DialogTitle>
          <DialogDescription>
            Define a new VM service plan with resource allocations and pricing.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-6 py-4">
          {/* Basic Info Section */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">Basic Information</h4>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="name" className="flex items-center gap-2">
                  <Server className="h-4 w-4 text-muted-foreground" />
                  Plan Name <span className="text-destructive">*</span>
                </Label>
                <Input
                  id="name"
                  placeholder="e.g., Starter, Pro, Enterprise"
                  {...form.register("name", { onChange: handleNameChange })}
                />
                {form.formState.errors.name && (
                  <p className="text-xs text-destructive">{form.formState.errors.name.message}</p>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="slug" className="flex items-center gap-2">
                  <Hash className="h-4 w-4 text-muted-foreground" />
                  Slug <span className="text-destructive">*</span>
                </Label>
                <Input
                  id="slug"
                  placeholder="e.g., starter, pro, enterprise"
                  {...form.register("slug")}
                />
                {form.formState.errors.slug && (
                  <p className="text-xs text-destructive">{form.formState.errors.slug.message}</p>
                )}
                <p className="text-xs text-muted-foreground">Lowercase alphanumeric with hyphens, auto-generated from name</p>
              </div>
            </div>
          </div>

          {/* Resources Section */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">Resources</h4>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div className="space-y-2">
                <Label htmlFor="vcpu" className="flex items-center gap-2">
                  <Cpu className="h-4 w-4 text-muted-foreground" />
                  vCPU <span className="text-destructive">*</span>
                </Label>
                <Input
                  id="vcpu"
                  type="number"
                  min={1}
                  {...form.register("vcpu", { valueAsNumber: true })}
                />
                {form.formState.errors.vcpu && (
                  <p className="text-xs text-destructive">{form.formState.errors.vcpu.message}</p>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="memory_mb" className="flex items-center gap-2">
                  <MemoryStick className="h-4 w-4 text-muted-foreground" />
                  Memory (MB) <span className="text-destructive">*</span>
                </Label>
                <Input
                  id="memory_mb"
                  type="number"
                  min={512}
                  step={512}
                  {...form.register("memory_mb", { valueAsNumber: true })}
                />
                {form.formState.errors.memory_mb && (
                  <p className="text-xs text-destructive">{form.formState.errors.memory_mb.message}</p>
                )}
                <p className="text-xs text-muted-foreground">1024 = 1 GB</p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="disk_gb" className="flex items-center gap-2">
                  <HardDrive className="h-4 w-4 text-muted-foreground" />
                  Disk (GB) <span className="text-destructive">*</span>
                </Label>
                <Input
                  id="disk_gb"
                  type="number"
                  min={10}
                  {...form.register("disk_gb", { valueAsNumber: true })}
                />
                {form.formState.errors.disk_gb && (
                  <p className="text-xs text-destructive">{form.formState.errors.disk_gb.message}</p>
                )}
              </div>
            </div>
          </div>

          {/* Network Section */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">Network</h4>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="port_speed_mbps" className="flex items-center gap-2">
                  <Activity className="h-4 w-4 text-muted-foreground" />
                  Port Speed (Mbps) <span className="text-destructive">*</span>
                </Label>
                <Input
                  id="port_speed_mbps"
                  type="number"
                  min={1}
                  {...form.register("port_speed_mbps", { valueAsNumber: true })}
                />
                {form.formState.errors.port_speed_mbps && (
                  <p className="text-xs text-destructive">{form.formState.errors.port_speed_mbps.message}</p>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="bandwidth_limit_gb" className="flex items-center gap-2">
                  <Network className="h-4 w-4 text-muted-foreground" />
                  Bandwidth Limit (GB)
                </Label>
                <Input
                  id="bandwidth_limit_gb"
                  type="number"
                  min={0}
                  {...form.register("bandwidth_limit_gb", { valueAsNumber: true })}
                />
                {form.formState.errors.bandwidth_limit_gb && (
                  <p className="text-xs text-destructive">{form.formState.errors.bandwidth_limit_gb.message}</p>
                )}
                <p className="text-xs text-muted-foreground">0 = unlimited</p>
              </div>
            </div>
          </div>

          {/* Pricing Section */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">Pricing (in cents)</h4>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="price_monthly" className="flex items-center gap-2">
                  <DollarSign className="h-4 w-4 text-muted-foreground" />
                  Monthly Price (cents)
                </Label>
                <Input
                  id="price_monthly"
                  type="number"
                  min={0}
                  {...form.register("price_monthly", { valueAsNumber: true })}
                />
                {form.formState.errors.price_monthly && (
                  <p className="text-xs text-destructive">{form.formState.errors.price_monthly.message}</p>
                )}
                <p className="text-xs text-muted-foreground">
                  {form.watch("price_monthly") ? `$${((form.watch("price_monthly") || 0) / 100).toFixed(2)}` : "Enter cents, e.g., 999 = $9.99"}
                </p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="price_hourly" className="flex items-center gap-2">
                  <Zap className="h-4 w-4 text-muted-foreground" />
                  Hourly Price (cents)
                </Label>
                <Input
                  id="price_hourly"
                  type="number"
                  min={0}
                  {...form.register("price_hourly", { valueAsNumber: true })}
                />
                {form.formState.errors.price_hourly && (
                  <p className="text-xs text-destructive">{form.formState.errors.price_hourly.message}</p>
                )}
                <p className="text-xs text-muted-foreground">Enter cents, e.g., 1 = $0.01</p>
              </div>
            </div>
          </div>

          {/* Limits Section */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">Resource Limits per VM</h4>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div className="space-y-2">
                <Label htmlFor="snapshot_limit" className="flex items-center gap-2">
                  <Camera className="h-4 w-4 text-muted-foreground" />
                  Snapshot Limit
                </Label>
                <Input
                  id="snapshot_limit"
                  type="number"
                  min={0}
                  {...form.register("snapshot_limit", { valueAsNumber: true })}
                />
                {form.formState.errors.snapshot_limit && (
                  <p className="text-xs text-destructive">{form.formState.errors.snapshot_limit.message}</p>
                )}
                <p className="text-xs text-muted-foreground">0 = unlimited</p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="backup_limit" className="flex items-center gap-2">
                  <Archive className="h-4 w-4 text-muted-foreground" />
                  Backup Limit
                </Label>
                <Input
                  id="backup_limit"
                  type="number"
                  min={0}
                  {...form.register("backup_limit", { valueAsNumber: true })}
                />
                {form.formState.errors.backup_limit && (
                  <p className="text-xs text-destructive">{form.formState.errors.backup_limit.message}</p>
                )}
                <p className="text-xs text-muted-foreground">0 = unlimited</p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="iso_upload_limit" className="flex items-center gap-2">
                  <Disc className="h-4 w-4 text-muted-foreground" />
                  ISO Upload Limit
                </Label>
                <Input
                  id="iso_upload_limit"
                  type="number"
                  min={0}
                  {...form.register("iso_upload_limit", { valueAsNumber: true })}
                />
                {form.formState.errors.iso_upload_limit && (
                  <p className="text-xs text-destructive">{form.formState.errors.iso_upload_limit.message}</p>
                )}
                <p className="text-xs text-muted-foreground">0 = unlimited</p>
              </div>
            </div>
          </div>

          {/* Settings Section */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">Settings</h4>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div className="space-y-2">
                <Label htmlFor="storage_backend" className="flex items-center gap-2">
                  <HardDrive className="h-4 w-4 text-muted-foreground" />
                  Storage Backend
                </Label>
                <Select
                  value={form.watch("storage_backend")}
                  onValueChange={(value: "ceph" | "qcow" | "lvm") => form.setValue("storage_backend", value)}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="Select backend" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="ceph">Ceph (RBD)</SelectItem>
                    <SelectItem value="qcow">Local QCOW2</SelectItem>
                    <SelectItem value="lvm">LVM Thin</SelectItem>
                  </SelectContent>
                </Select>
                {form.formState.errors.storage_backend && (
                  <p className="text-xs text-destructive">{form.formState.errors.storage_backend.message}</p>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="sort_order" className="flex items-center gap-2">
                  <Hash className="h-4 w-4 text-muted-foreground" />
                  Sort Order
                </Label>
                <Input
                  id="sort_order"
                  type="number"
                  min={0}
                  {...form.register("sort_order", { valueAsNumber: true })}
                />
                {form.formState.errors.sort_order && (
                  <p className="text-xs text-destructive">{form.formState.errors.sort_order.message}</p>
                )}
                <p className="text-xs text-muted-foreground">Lower values appear first</p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="is_active" className="flex items-center gap-2">
                  <Activity className="h-4 w-4 text-muted-foreground" />
                  Active Status
                </Label>
                <div className="flex items-center gap-3 pt-2">
                  <Switch
                    id="is_active"
                    checked={form.watch("is_active")}
                    onCheckedChange={(checked) => form.setValue("is_active", checked)}
                  />
                  <span className="text-sm text-muted-foreground">
                    {form.watch("is_active") ? "Active" : "Inactive"}
                  </span>
                </div>
              </div>
            </div>
          </div>

          <DialogFooter className="pt-4">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={isCreating}>
              Cancel
            </Button>
            <Button type="submit" disabled={isCreating}>
              {isCreating && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Create Plan
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}