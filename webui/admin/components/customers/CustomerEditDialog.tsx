"use client";

import { useEffect } from "react";
import { z } from "zod";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@virtuestack/ui";
import { User, Mail, Loader2, ShieldCheck } from "lucide-react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useToast } from "@virtuestack/ui";

export const editCustomerSchema = z.object({
  name: z.string().min(1, "Name is required").max(255, "Name must be 255 characters or less"),
  status: z.enum(["active", "suspended"]).optional(),
});

export type EditCustomerFormData = z.infer<typeof editCustomerSchema>;

interface CustomerEditDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  customer: {
    id: string;
    name: string;
    email: string;
    status: string;
  } | null;
  onSave: (data: EditCustomerFormData) => Promise<void>;
  isSaving: boolean;
}

export function CustomerEditDialog({ open, onOpenChange, customer, onSave, isSaving }: CustomerEditDialogProps) {
  const { toast } = useToast();

  const form = useForm<EditCustomerFormData>({
    resolver: zodResolver(editCustomerSchema),
    defaultValues: {
      name: "",
      status: "active",
    },
  });

  // Reset form when customer changes
  useEffect(() => {
    if (customer && open) {
      form.reset({
        name: customer.name,
        status: customer.status as "active" | "suspended",
      });
    }
  }, [customer, open, form]);

  const handleSubmit = async (data: EditCustomerFormData) => {
    try {
      await onSave(data);
      onOpenChange(false);
    } catch (error) {
      toast({
        title: "Update Failed",
        description: error instanceof Error ? error.message : "Failed to update customer",
        variant: "destructive",
      });
    }
  };

  if (!customer) return null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <User className="h-5 w-5" />
            Edit Customer
          </DialogTitle>
          <DialogDescription>
            Update customer information for {customer.email}
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="edit-name" className="flex items-center gap-2">
              <User className="h-4 w-4 text-muted-foreground" />
              Full Name
            </Label>
            <Input
              id="edit-name"
              placeholder="e.g., John Doe"
              {...form.register("name")}
            />
            {form.formState.errors.name && (
              <p className="text-xs text-destructive">{form.formState.errors.name.message}</p>
            )}
          </div>

          <div className="space-y-2">
            <Label htmlFor="edit-email" className="flex items-center gap-2">
              <Mail className="h-4 w-4 text-muted-foreground" />
              Email Address
            </Label>
            <Input
              id="edit-email"
              type="email"
              value={customer.email}
              disabled
              className="bg-muted"
            />
            <p className="text-xs text-muted-foreground">Email cannot be changed</p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="edit-status" className="flex items-center gap-2">
              <ShieldCheck className="h-4 w-4 text-muted-foreground" />
              Account Status
            </Label>
            <Select
              value={form.watch("status")}
              onValueChange={(value: "active" | "suspended") => form.setValue("status", value)}
            >
              <SelectTrigger>
                <SelectValue placeholder="Select status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="active">Active</SelectItem>
                <SelectItem value="suspended">Suspended</SelectItem>
              </SelectContent>
            </Select>
            {form.formState.errors.status && (
              <p className="text-xs text-destructive">{form.formState.errors.status.message}</p>
            )}
            <p className="text-xs text-muted-foreground">
              Suspended accounts cannot access their dashboard or VMs
            </p>
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