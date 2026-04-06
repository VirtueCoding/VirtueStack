"use client";

import { useEffect } from "react";
import { z } from "zod";
import { Button } from "@virtuestack/ui";
import { Input } from "@virtuestack/ui";
import { Label } from "@virtuestack/ui";
import { Switch } from "@virtuestack/ui";
import { Textarea } from "@/components/ui/textarea";
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
import { HardDrive, Hash, Server, Activity, Disc, FileText, Database, Loader2 } from "lucide-react";
import { useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useToast } from "@virtuestack/ui";

export const editTemplateSchema = z.object({
  name: z.string().min(1, "Name is required").max(100, "Name must be 100 characters or less").optional(),
  os_family: z.string().max(50, "OS Family must be 50 characters or less").optional(),
  os_version: z.string().max(50, "OS Version must be 50 characters or less").optional(),
  rbd_image: z.string().max(255, "RBD Image must be 255 characters or less").optional(),
  rbd_snapshot: z.string().max(255, "RBD Snapshot must be 255 characters or less").optional(),
  min_disk_gb: z.number().int().min(1, "Must be at least 1 GB").optional(),
  supports_cloudinit: z.boolean().optional(),
  is_active: z.boolean().optional(),
  sort_order: z.number().int().min(0, "Must be 0 or greater").optional(),
  description: z.string().max(500, "Description must be 500 characters or less").optional(),
  storage_backend: z.enum(["ceph", "qcow", "lvm"]).optional(),
  file_path: z.string().max(500, "File Path must be 500 characters or less").optional(),
});

export type EditTemplateFormData = z.infer<typeof editTemplateSchema>;

interface TemplateEditDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  template: {
    id: string;
    name: string;
    os_family: string;
    os_version: string;
    rbd_image: string;
    rbd_snapshot: string;
    min_disk_gb: number;
    supports_cloudinit: boolean;
    is_active: boolean;
    sort_order: number;
    description?: string;
    storage_backend: string;
    file_path?: string;
  } | null;
  onSave: (data: EditTemplateFormData) => Promise<void>;
  isSaving: boolean;
}

export function TemplateEditDialog({ open, onOpenChange, template, onSave, isSaving }: TemplateEditDialogProps) {
  const { toast } = useToast();

  const form = useForm<EditTemplateFormData>({
    resolver: zodResolver(editTemplateSchema),
    defaultValues: {
      name: "",
      os_family: "",
      os_version: "",
      rbd_image: "",
      rbd_snapshot: "",
      min_disk_gb: 10,
      supports_cloudinit: true,
      is_active: true,
      sort_order: 0,
      description: "",
      storage_backend: "ceph",
      file_path: "",
    },
  });
  const storageBackend = useWatch({ control: form.control, name: "storage_backend" });
  const isActive = useWatch({ control: form.control, name: "is_active" });
  const supportsCloudinit = useWatch({ control: form.control, name: "supports_cloudinit" });

  // Reset form when template changes
  useEffect(() => {
    if (template && open) {
      form.reset({
        name: template.name,
        os_family: template.os_family,
        os_version: template.os_version,
        rbd_image: template.rbd_image,
        rbd_snapshot: template.rbd_snapshot,
        min_disk_gb: template.min_disk_gb,
        supports_cloudinit: template.supports_cloudinit,
        is_active: template.is_active,
        sort_order: template.sort_order,
        description: template.description || "",
        storage_backend: template.storage_backend as "ceph" | "qcow",
        file_path: template.file_path || "",
      });
    }
  }, [template, open, form]);

  const handleSubmit = async (data: EditTemplateFormData) => {
    try {
      await onSave(data);
      onOpenChange(false);
    } catch (error) {
      toast({
        title: "Update Failed",
        description: error instanceof Error ? error.message : "Failed to update template",
        variant: "destructive",
      });
    }
  };

  if (!template) return null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Server className="h-5 w-5" />
            Edit Template: {template.name}
          </DialogTitle>
          <DialogDescription>
            Modify template properties. All fields are optional for partial updates.
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
                  Template Name
                </Label>
                <Input
                  id="edit-name"
                  placeholder="e.g., Ubuntu 24.04 LTS"
                  {...form.register("name")}
                />
                {form.formState.errors.name && (
                  <p className="text-xs text-destructive">{form.formState.errors.name.message}</p>
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
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-description" className="flex items-center gap-2">
                <FileText className="h-4 w-4 text-muted-foreground" />
                Description
              </Label>
              <Textarea
                id="edit-description"
                placeholder="Optional description of this template..."
                rows={2}
                {...form.register("description")}
              />
              {form.formState.errors.description && (
                <p className="text-xs text-destructive">{form.formState.errors.description.message}</p>
              )}
            </div>
          </div>

          {/* OS Info Section */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">Operating System</h4>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="edit-os_family" className="flex items-center gap-2">
                  <Disc className="h-4 w-4 text-muted-foreground" />
                  OS Family
                </Label>
                <Input
                  id="edit-os_family"
                  placeholder="e.g., debian, ubuntu, centos"
                  {...form.register("os_family")}
                />
                {form.formState.errors.os_family && (
                  <p className="text-xs text-destructive">{form.formState.errors.os_family.message}</p>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="edit-os_version">OS Version</Label>
                <Input
                  id="edit-os_version"
                  placeholder="e.g., 24.04, 22, 9"
                  {...form.register("os_version")}
                />
                {form.formState.errors.os_version && (
                  <p className="text-xs text-destructive">{form.formState.errors.os_version.message}</p>
                )}
              </div>
            </div>
          </div>

          {/* Storage Section */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">Storage Configuration</h4>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="edit-storage_backend" className="flex items-center gap-2">
                  <Database className="h-4 w-4 text-muted-foreground" />
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
                <Label htmlFor="edit-min_disk_gb" className="flex items-center gap-2">
                  <HardDrive className="h-4 w-4 text-muted-foreground" />
                  Min Disk (GB)
                </Label>
                <Input
                  id="edit-min_disk_gb"
                  type="number"
                  min={1}
                  {...form.register("min_disk_gb", { valueAsNumber: true })}
                />
                {form.formState.errors.min_disk_gb && (
                  <p className="text-xs text-destructive">{form.formState.errors.min_disk_gb.message}</p>
                )}
                <p className="text-xs text-muted-foreground">Minimum disk size required</p>
              </div>
            </div>
            {storageBackend === "ceph" && (
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="edit-rbd_image">RBD Image</Label>
                  <Input
                    id="edit-rbd_image"
                    placeholder="e.g., vs-images/ubuntu-24.04"
                    {...form.register("rbd_image")}
                  />
                  {form.formState.errors.rbd_image && (
                    <p className="text-xs text-destructive">{form.formState.errors.rbd_image.message}</p>
                  )}
                </div>
                <div className="space-y-2">
                  <Label htmlFor="edit-rbd_snapshot">RBD Snapshot</Label>
                  <Input
                    id="edit-rbd_snapshot"
                    placeholder="e.g., v1"
                    {...form.register("rbd_snapshot")}
                  />
                  {form.formState.errors.rbd_snapshot && (
                    <p className="text-xs text-destructive">{form.formState.errors.rbd_snapshot.message}</p>
                  )}
                </div>
              </div>
            )}
            {storageBackend === "qcow" && (
              <div className="space-y-2">
                <Label htmlFor="edit-file_path">File Path</Label>
                <Input
                  id="edit-file_path"
                  placeholder="e.g., /var/lib/libvirt/templates/ubuntu-24.04.qcow2"
                  {...form.register("file_path")}
                />
                {form.formState.errors.file_path && (
                  <p className="text-xs text-destructive">{form.formState.errors.file_path.message}</p>
                )}
                <p className="text-xs text-muted-foreground">Path to the QCOW2 template file on the node</p>
              </div>
            )}
            {storageBackend === "lvm" && (
              <div className="space-y-2">
                <Label htmlFor="edit-file_path">LV Path</Label>
                <Input
                  id="edit-file_path"
                  placeholder="e.g., /dev/vgvs/template-ubuntu-24.04"
                  {...form.register("file_path")}
                />
                {form.formState.errors.file_path && (
                  <p className="text-xs text-destructive">{form.formState.errors.file_path.message}</p>
                )}
                <p className="text-xs text-muted-foreground">Path to the LVM thin logical volume</p>
              </div>
            )}
          </div>

          {/* Status Section */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">Status</h4>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
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
              <div className="space-y-2">
                <Label htmlFor="edit-supports_cloudinit">Cloud-Init Support</Label>
                <div className="flex items-center gap-3 pt-2">
                  <Switch
                    id="edit-supports_cloudinit"
                    checked={supportsCloudinit}
                    onCheckedChange={(checked) => form.setValue("supports_cloudinit", checked)}
                  />
                  <span className="text-sm text-muted-foreground">
                    {supportsCloudinit ? "Supported" : "Not Supported"}
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
