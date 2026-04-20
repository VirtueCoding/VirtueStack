"use client";

import { useForm, useWatch } from "react-hook-form";
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
import { Loader2, Plus } from "lucide-react";
import { createIPSetSchema, CreateIPSetFormData } from "./validation";

interface IPSetCreateDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreate: (data: CreateIPSetFormData) => Promise<void>;
  isCreating: boolean;
}

export function IPSetCreateDialog({ open, onOpenChange, onCreate, isCreating }: IPSetCreateDialogProps) {
  const createForm = useForm<CreateIPSetFormData>({
    resolver: zodResolver(createIPSetSchema),
    defaultValues: {
      name: "",
      network: "",
      gateway: "",
      ip_version: 4,
    },
  });

  const ipVersion = useWatch({ control: createForm.control, name: "ip_version" }) || 4;

  const handleSubmit = async (data: CreateIPSetFormData) => {
    await onCreate(data);
    createForm.reset();
    onOpenChange(false);
  };

  const handleClose = () => {
    createForm.reset();
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-[525px]">
        <DialogHeader>
          <DialogTitle>Create IP Set</DialogTitle>
          <DialogDescription>
            Create a new IP address pool for VM assignments.
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={createForm.handleSubmit(handleSubmit)} className="grid gap-4 py-4">
          <div className="grid gap-2">
            <Label htmlFor="name">Name</Label>
            <Input
              id="name"
              {...createForm.register("name")}
              placeholder="e.g., production-pool-01"
              className={createForm.formState.errors.name ? "border-destructive" : ""}
            />
            {createForm.formState.errors.name && (
              <p className="text-xs text-destructive">{createForm.formState.errors.name.message}</p>
            )}
          </div>

          <div className="grid gap-2">
            <Label htmlFor="ip-version">IP Version</Label>
            <select
              id="ip-version"
              {...createForm.register("ip_version", { valueAsNumber: true })}
              className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
            >
              <option value={4}>IPv4</option>
              <option value={6}>IPv6</option>
            </select>
          </div>

          <div className="grid gap-2">
            <Label htmlFor="network">Network CIDR</Label>
            <Input
              id="network"
              {...createForm.register("network")}
              placeholder={ipVersion === 4 ? "10.0.0.0/24" : "2001:db8::/32"}
              className={createForm.formState.errors.network ? "border-destructive font-mono" : "font-mono"}
            />
            {createForm.formState.errors.network ? (
              <p className="text-xs text-destructive">{createForm.formState.errors.network.message}</p>
            ) : (
              <p className="text-xs text-muted-foreground">
                {ipVersion === 4
                  ? "Format: xxx.xxx.xxx.xxx/xx (e.g., 10.0.0.0/24)"
                  : "Format: xxxx:xxxx::/xx (e.g., 2001:db8::/32)"}
              </p>
            )}
          </div>

          <div className="grid gap-2">
            <Label htmlFor="gateway">Gateway</Label>
            <Input
              id="gateway"
              {...createForm.register("gateway")}
              placeholder={ipVersion === 4 ? "10.0.0.1" : "2001:db8::1"}
              className={createForm.formState.errors.gateway ? "border-destructive font-mono" : "font-mono"}
            />
            {createForm.formState.errors.gateway && (
              <p className="text-xs text-destructive">{createForm.formState.errors.gateway.message}</p>
            )}
          </div>

          <div className="grid gap-2">
            <Label htmlFor="vlan-id">VLAN ID (Optional)</Label>
            <Input
              id="vlan-id"
              type="number"
              min={1}
              max={4094}
              {...createForm.register("vlan_id", {
                setValueAs: (value) => {
                  if (value === "" || value == null) {
                    return undefined;
                  }
                  const parsed = Number(value);
                  return Number.isNaN(parsed) ? undefined : parsed;
                },
              })}
              placeholder="e.g., 100"
              className={createForm.formState.errors.vlan_id ? "border-destructive" : ""}
            />
            {createForm.formState.errors.vlan_id && (
              <p className="text-xs text-destructive">{createForm.formState.errors.vlan_id.message}</p>
            )}
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={handleClose} disabled={isCreating}>
              Cancel
            </Button>
            <Button type="submit" disabled={isCreating}>
              {isCreating ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Creating...
                </>
              ) : (
                <>
                  <Plus className="mr-2 h-4 w-4" />
                  Create
                </>
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
