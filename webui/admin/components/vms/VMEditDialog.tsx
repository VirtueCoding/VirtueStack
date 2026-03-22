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
import { Server, Cpu, MemoryStick, HardDrive, Network, Activity, Loader2 } from "lucide-react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useToast } from "@/components/ui/use-toast";
import type { VM } from "@/lib/api-client";

// RFC 1123 hostname validation: lowercase alphanumeric with hyphens, max 63 chars
const hostnameRegex = /^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/;

export const editVMSchema = z.object({
  hostname: z.string()
    .min(1, "Hostname is required")
    .max(63, "Hostname must be 63 characters or less")
    .regex(hostnameRegex, "Hostname must be lowercase alphanumeric with hyphens")
    .optional(),
  vcpu: z.number().int().min(1, "Must be at least 1 vCPU").optional(),
  memory_mb: z.number().int().min(512, "Must be at least 512 MB").optional(),
  disk_gb: z.number().int().min(10, "Must be at least 10 GB").optional(),
  port_speed_mbps: z.number().int().min(1, "Must be at least 1 Mbps").optional(),
  bandwidth_limit_gb: z.number().int().min(0, "Must be 0 or greater").optional(),
});

export type EditVMFormData = z.infer<typeof editVMSchema>;

interface VMEditDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  vm: VM | null;
  onSave: (data: EditVMFormData) => Promise<void>;
  isSaving: boolean;
}

export function VMEditDialog({ open, onOpenChange, vm, onSave, isSaving }: VMEditDialogProps) {
  const { toast } = useToast();

  const form = useForm<EditVMFormData>({
    resolver: zodResolver(editVMSchema),
    defaultValues: {
      hostname: "",
      vcpu: 1,
      memory_mb: 512,
      disk_gb: 10,
      port_speed_mbps: 1000,
      bandwidth_limit_gb: 0,
    },
  });

  // Reset form when VM changes
  useEffect(() => {
    if (vm && open) {
      form.reset({
        hostname: vm.hostname || vm.name?.toLowerCase().replace(/[^a-z0-9-]/g, "-") || "",
        vcpu: vm.vcpu || 1,
        memory_mb: vm.memory_mb || 512,
        disk_gb: vm.disk_gb || 10,
        port_speed_mbps: vm.port_speed_mbps || 1000,
        bandwidth_limit_gb: vm.bandwidth_limit_gb || 0,
      });
    }
  }, [vm, open, form]);

  const handleSubmit = async (data: EditVMFormData) => {
    try {
      // Only send changed fields
      const changes: EditVMFormData = {};
      if (data.hostname && data.hostname !== vm?.hostname) {
        changes.hostname = data.hostname;
      }
      if (data.vcpu && data.vcpu !== vm?.vcpu) {
        changes.vcpu = data.vcpu;
      }
      if (data.memory_mb && data.memory_mb !== vm?.memory_mb) {
        changes.memory_mb = data.memory_mb;
      }
      if (data.disk_gb && data.disk_gb !== vm?.disk_gb) {
        changes.disk_gb = data.disk_gb;
      }
      if (data.port_speed_mbps && data.port_speed_mbps !== vm?.port_speed_mbps) {
        changes.port_speed_mbps = data.port_speed_mbps;
      }
      if (data.bandwidth_limit_gb !== undefined && data.bandwidth_limit_gb !== vm?.bandwidth_limit_gb) {
        changes.bandwidth_limit_gb = data.bandwidth_limit_gb;
      }

      if (Object.keys(changes).length === 0) {
        toast({
          title: "No Changes",
          description: "No changes were made to the VM.",
        });
        onOpenChange(false);
        return;
      }

      await onSave(changes);
      onOpenChange(false);
    } catch (error) {
      toast({
        title: "Update Failed",
        description: error instanceof Error ? error.message : "Failed to update VM",
        variant: "destructive",
      });
    }
  };

  if (!vm) return null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Server className="h-5 w-5" />
            Edit VM: {vm.name || vm.id.substring(0, 8)}
          </DialogTitle>
          <DialogDescription>
            Modify VM properties. Resource changes may trigger a resize operation.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-6 py-4">
          {/* Hostname */}
          <div className="space-y-2">
            <Label htmlFor="edit-hostname" className="flex items-center gap-2">
              <Server className="h-4 w-4 text-muted-foreground" />
              Hostname
            </Label>
            <Input
              id="edit-hostname"
              placeholder="e.g., my-server-01"
              {...form.register("hostname")}
              onChange={(e) => {
                // Convert to lowercase automatically
                form.setValue("hostname", e.target.value.toLowerCase());
              }}
            />
            {form.formState.errors.hostname && (
              <p className="text-xs text-destructive">{form.formState.errors.hostname.message}</p>
            )}
            <p className="text-xs text-muted-foreground">Lowercase alphanumeric with hyphens</p>
          </div>

          {/* Resources */}
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

          {/* Network */}
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