"use client";

import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Button } from "@virtuestack/ui";
import { Input } from "@virtuestack/ui";
import { Label } from "@virtuestack/ui";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@virtuestack/ui";
import { Loader2, Network, Hash, Router } from "lucide-react";
import { editIPSetSchema, EditIPSetFormData } from "./validation";

interface IPSetEditDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  ipSet: {
    id: string;
    name: string;
    gateway: string;
    vlan_id?: number | null;
    location_id?: string | null;
    node_ids?: string[];
    ip_version: number;
    network: string;
  } | null;
  onSave: (data: EditIPSetFormData) => Promise<void>;
  isSaving: boolean;
}

export function IPSetEditDialog({ open, onOpenChange, ipSet, onSave, isSaving }: IPSetEditDialogProps) {
  const form = useForm<EditIPSetFormData>({
    resolver: zodResolver(editIPSetSchema),
    defaultValues: {
      name: "",
      gateway: "",
      vlan_id: null,
      location_id: null,
      node_ids: [],
    },
  });

  // Reset form when ipSet changes
  useEffect(() => {
    if (ipSet && open) {
      form.reset({
        name: ipSet.name,
        gateway: ipSet.gateway,
        vlan_id: ipSet.vlan_id ?? null,
        location_id: ipSet.location_id ?? null,
        node_ids: ipSet.node_ids ?? [],
      });
    }
  }, [ipSet, open, form]);

  const handleSubmit = async (data: EditIPSetFormData) => {
    await onSave(data);
    onOpenChange(false);
  };

  const handleClose = () => {
    form.reset();
    onOpenChange(false);
  };

  if (!ipSet) return null;

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-[525px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Network className="h-5 w-5" />
            Edit IP Set: {ipSet.name}
          </DialogTitle>
          <DialogDescription>
            Modify IP set properties. Network CIDR and IP version cannot be changed after creation.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={form.handleSubmit(handleSubmit)} className="grid gap-4 py-4">
          {/* Read-only network info */}
          <div className="rounded-md border bg-muted/50 p-3 space-y-2">
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">Network CIDR:</span>
              <span className="font-mono">{ipSet.network}</span>
            </div>
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">IP Version:</span>
              <span className="font-mono">IPv{ipSet.ip_version}</span>
            </div>
          </div>

          <div className="grid gap-2">
            <Label htmlFor="edit-name" className="flex items-center gap-2">
              <Network className="h-4 w-4 text-muted-foreground" />
              Name
            </Label>
            <Input
              id="edit-name"
              {...form.register("name")}
              placeholder="e.g., production-pool-01"
              className={form.formState.errors.name ? "border-destructive" : ""}
            />
            {form.formState.errors.name && (
              <p className="text-xs text-destructive">{form.formState.errors.name.message}</p>
            )}
          </div>

          <div className="grid gap-2">
            <Label htmlFor="edit-gateway" className="flex items-center gap-2">
              <Router className="h-4 w-4 text-muted-foreground" />
              Gateway
            </Label>
            <Input
              id="edit-gateway"
              {...form.register("gateway")}
              placeholder={ipSet.ip_version === 4 ? "10.0.0.1" : "2001:db8::1"}
              className={`${form.formState.errors.gateway ? "border-destructive" : ""} font-mono`}
            />
            {form.formState.errors.gateway && (
              <p className="text-xs text-destructive">{form.formState.errors.gateway.message}</p>
            )}
          </div>

          <div className="grid gap-2">
            <Label htmlFor="edit-vlan-id" className="flex items-center gap-2">
              <Hash className="h-4 w-4 text-muted-foreground" />
              VLAN ID (Optional)
            </Label>
            <Input
              id="edit-vlan-id"
              type="number"
              min={1}
              max={4094}
              {...form.register("vlan_id", {
                setValueAs: (value) => {
                  if (value === "" || value == null) {
                    return null;
                  }
                  const parsed = Number(value);
                  return Number.isNaN(parsed) ? null : parsed;
                },
              })}
              placeholder="e.g., 100"
              className={form.formState.errors.vlan_id ? "border-destructive" : ""}
            />
            {form.formState.errors.vlan_id && (
              <p className="text-xs text-destructive">{form.formState.errors.vlan_id.message}</p>
            )}
            <p className="text-xs text-muted-foreground">
              Leave empty to remove VLAN assignment
            </p>
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={handleClose} disabled={isSaving}>
              Cancel
            </Button>
            <Button type="submit" disabled={isSaving}>
              {isSaving ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Saving...
                </>
              ) : (
                "Save Changes"
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
