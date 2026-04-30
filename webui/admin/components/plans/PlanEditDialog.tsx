"use client";

import { useEffect } from "react";
import { z } from "zod";
import { Button } from "@virtuestack/ui";
import { Input } from "@virtuestack/ui";
import { Label } from "@virtuestack/ui";
import { Switch } from "@virtuestack/ui";
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
import { Cpu, MemoryStick, HardDrive, Network, DollarSign, Loader2, Camera, Archive, Disc, Server, Hash, Activity, Zap } from "lucide-react";
import { useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useToast } from "@virtuestack/ui";

export const editPlanSchema = z.object({
  name: z.string().min(1, "Name is required").max(100, "Name must be 100 characters or less").optional(),
  slug: z.string()
    .min(1, "Slug is required")
    .max(100, "Slug must be 100 characters or less")
    .regex(/^[a-z0-9]+(?:-[a-z0-9]+)*$/, "Slug must be lowercase alphanumeric with hyphens")
    .optional(),
  vcpu: z.number().int().min(1, "Must be at least 1 vCPU").optional(),
  memory_mb: z.number().int().min(512, "Must be at least 512 MB").optional(),
  disk_gb: z.number().int().min(10, "Must be at least 10 GB").optional(),
  bandwidth_limit_gb: z.number().int().min(0, "Must be 0 or greater").optional(),
  port_speed_mbps: z.number().int().min(1, "Must be at least 1 Mbps").optional(),
  price_monthly: z.number().int().min(0, "Must be 0 or greater").optional(),
  price_hourly: z.number().int().min(0, "Must be 0 or greater").optional(),
  storage_backend: z.enum(["ceph", "qcow", "lvm"]).optional(),
  is_active: z.boolean().optional(),
  sort_order: z.number().int().min(0, "Must be 0 or greater").optional(),
  snapshot_limit: z.number().int().min(0, "Must be 0 or greater").optional(),
  backup_limit: z.number().int().min(0, "Must be 0 or greater").optional(),
  iso_upload_limit: z.number().int().min(0, "Must be 0 or greater").optional(),
});

export type EditPlanFormData = z.infer<typeof editPlanSchema>;

interface PlanEditDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  plan: {
    id: string;
    name: string;
    slug: string;
    vcpu: number;
    memory_mb: number;
    disk_gb: number;
    bandwidth_limit_gb: number;
    port_speed_mbps: number;
    price_monthly: number;
    price_hourly: number;
    storage_backend: string;
    is_active: boolean;
    sort_order: number;
    snapshot_limit: number;
    backup_limit: number;
    iso_upload_limit: number;
  } | null;
  onSave: (data: EditPlanFormData) => Promise<void>;
  isSaving: boolean;
}

export function PlanEditDialog({ open, onOpenChange, plan, onSave, isSaving }: PlanEditDialogProps) {
  const { toast } = useToast();

  const form = useForm<EditPlanFormData>({
    resolver: zodResolver(editPlanSchema),
    defaultValues: {
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
    },
  });

  // Reset form when plan changes
  useEffect(() => {
    if (plan && open) {
      form.reset({
        name: plan.name,
        slug: plan.slug,
        vcpu: plan.vcpu,
        memory_mb: plan.memory_mb,
        disk_gb: plan.disk_gb,
        port_speed_mbps: plan.port_speed_mbps,
        bandwidth_limit_gb: plan.bandwidth_limit_gb,
        price_monthly: plan.price_monthly,
        price_hourly: plan.price_hourly,
        storage_backend: plan.storage_backend as "ceph" | "qcow" | "lvm",
        is_active: plan.is_active,
        sort_order: plan.sort_order,
        snapshot_limit: plan.snapshot_limit,
        backup_limit: plan.backup_limit,
        iso_upload_limit: plan.iso_upload_limit,
      });
    }
  }, [plan, open, form]);

  const handleSubmit = async (data: EditPlanFormData) => {
    try {
      await onSave(data);
      onOpenChange(false);
    } catch (error) {
      toast({
        title: "Update Failed",
        description: error instanceof Error ? error.message : "Failed to update plan",
        variant: "destructive",
      });
    }
  };
  const priceMonthly = useWatch({ control: form.control, name: "price_monthly" });
  const storageBackend = useWatch({ control: form.control, name: "storage_backend" }) || "ceph";
  const isActive = useWatch({ control: form.control, name: "is_active" }) ?? true;

  if (!plan) return null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Server className="h-5 w-5" />
            Edit Plan: {plan.name}
          </DialogTitle>
          <DialogDescription>
            Modify plan properties. All fields are optional for partial updates.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-6 py-4">
          {/* Basic Info Section */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">Basic Information</h4>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="edit-name" className="flex items-center gap-2">
                  <Server className="h-4 w-4 text-muted-foreground" />
                  Plan Name
                </Label>
                <Input
                  id="edit-name"
                  placeholder="e.g., Starter, Pro, Enterprise"
                  {...form.register("name")}
                />
                {form.formState.errors.name && (
                  <p className="text-xs text-destructive">{form.formState.errors.name.message}</p>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="edit-slug" className="flex items-center gap-2">
                  <Hash className="h-4 w-4 text-muted-foreground" />
                  Slug
                </Label>
                <Input
                  id="edit-slug"
                  placeholder="e.g., starter, pro, enterprise"
                  {...form.register("slug")}
                />
                {form.formState.errors.slug && (
                  <p className="text-xs text-destructive">{form.formState.errors.slug.message}</p>
                )}
                <p className="text-xs text-muted-foreground">Lowercase alphanumeric with hyphens</p>
              </div>
            </div>
          </div>

          {/* Resources Section */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">Resources</h4>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div className="space-y-2">
                <Label htmlFor="edit-vcpu" className="flex items-center gap-2">
                  <Cpu className="h-4 w-4 text-muted-foreground" />
                  vCPU
                </Label>
                <Input
                  id="edit-vcpu"
                  type="number"
                  min={1}
                  {...form.register("vcpu", { valueAsNumber: true })}
                />
                {form.formState.errors.vcpu && (
                  <p className="text-xs text-destructive">{form.formState.errors.vcpu.message}</p>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="edit-memory_mb" className="flex items-center gap-2">
                  <MemoryStick className="h-4 w-4 text-muted-foreground" />
                  Memory (MB)
                </Label>
                <Input
                  id="edit-memory_mb"
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
                <Label htmlFor="edit-disk_gb" className="flex items-center gap-2">
                  <HardDrive className="h-4 w-4 text-muted-foreground" />
                  Disk (GB)
                </Label>
                <Input
                  id="edit-disk_gb"
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
                <Label htmlFor="edit-port_speed_mbps" className="flex items-center gap-2">
                  <Activity className="h-4 w-4 text-muted-foreground" />
                  Port Speed (Mbps)
                </Label>
                <Input
                  id="edit-port_speed_mbps"
                  type="number"
                  min={1}
                  {...form.register("port_speed_mbps", { valueAsNumber: true })}
                />
                {form.formState.errors.port_speed_mbps && (
                  <p className="text-xs text-destructive">{form.formState.errors.port_speed_mbps.message}</p>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="edit-bandwidth_limit_gb" className="flex items-center gap-2">
                  <Network className="h-4 w-4 text-muted-foreground" />
                  Bandwidth Limit (GB)
                </Label>
                <Input
                  id="edit-bandwidth_limit_gb"
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
                <Label htmlFor="edit-price_monthly" className="flex items-center gap-2">
                  <DollarSign className="h-4 w-4 text-muted-foreground" />
                  Monthly Price (cents)
                </Label>
                <Input
                  id="edit-price_monthly"
                  type="number"
                  min={0}
                  {...form.register("price_monthly", { valueAsNumber: true })}
                />
                {form.formState.errors.price_monthly && (
                  <p className="text-xs text-destructive">{form.formState.errors.price_monthly.message}</p>
                )}
                <p className="text-xs text-muted-foreground">
                  {priceMonthly !== undefined ? `$${((priceMonthly || 0) / 100).toFixed(2)}` : "Enter cents, e.g., 999 = $9.99"}
                </p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="edit-price_hourly" className="flex items-center gap-2">
                  <Zap className="h-4 w-4 text-muted-foreground" />
                  Hourly Price (cents)
                </Label>
                <Input
                  id="edit-price_hourly"
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
                <Label htmlFor="edit-snapshot_limit" className="flex items-center gap-2">
                  <Camera className="h-4 w-4 text-muted-foreground" />
                  Snapshot Limit
                </Label>
                <Input
                  id="edit-snapshot_limit"
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
                <Label htmlFor="edit-backup_limit" className="flex items-center gap-2">
                  <Archive className="h-4 w-4 text-muted-foreground" />
                  Backup Limit
                </Label>
                <Input
                  id="edit-backup_limit"
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
                <Label htmlFor="edit-iso_upload_limit" className="flex items-center gap-2">
                  <Disc className="h-4 w-4 text-muted-foreground" />
                  ISO Upload Limit
                </Label>
                <Input
                  id="edit-iso_upload_limit"
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
                <Label htmlFor="edit-storage_backend" className="flex items-center gap-2">
                  <HardDrive className="h-4 w-4 text-muted-foreground" />
                  Storage Backend
                </Label>
                <Select
                  value={storageBackend}
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
                <Label htmlFor="edit-sort_order" className="flex items-center gap-2">
                  <Hash className="h-4 w-4 text-muted-foreground" />
                  Sort Order
                </Label>
                <Input
                  id="edit-sort_order"
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
                <Label htmlFor="edit-is_active" className="flex items-center gap-2">
                  <Activity className="h-4 w-4 text-muted-foreground" />
                  Active Status
                </Label>
                <div className="flex items-center gap-3 pt-2">
                  <Switch
                    id="edit-is_active"
                    checked={isActive}
                    onCheckedChange={(checked) => form.setValue("is_active", checked)}
                  />
                  <span className="text-sm text-muted-foreground">
                    {isActive ? "Active" : "Inactive"}
                  </span>
                </div>
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
