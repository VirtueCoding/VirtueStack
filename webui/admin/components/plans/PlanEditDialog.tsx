"use client";

import { useState } from "react";
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
import { Camera, Archive, Disc, Loader2 } from "lucide-react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useToast } from "@/components/ui/use-toast";

export const editPlanSchema = z.object({
  snapshot_limit: z.number().int().min(0, "Must be 0 or greater"),
  backup_limit: z.number().int().min(0, "Must be 0 or greater"),
  iso_upload_limit: z.number().int().min(0, "Must be 0 or greater"),
});

export type EditPlanFormData = z.infer<typeof editPlanSchema>;

interface PlanEditDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  plan: {
    id: string;
    name: string;
    snapshot_limit?: number;
    backup_limit?: number;
    iso_upload_limit?: number;
  } | null;
  onSave: (data: EditPlanFormData) => Promise<void>;
  isSaving: boolean;
}

export function PlanEditDialog({ open, onOpenChange, plan, onSave, isSaving }: PlanEditDialogProps) {
  const { toast } = useToast();

  const form = useForm<EditPlanFormData>({
    resolver: zodResolver(editPlanSchema),
    defaultValues: {
      snapshot_limit: 2,
      backup_limit: 2,
      iso_upload_limit: 2,
    },
  });

  // Reset form when plan changes
  useState(() => {
    if (plan) {
      form.reset({
        snapshot_limit: plan.snapshot_limit ?? 2,
        backup_limit: plan.backup_limit ?? 2,
        iso_upload_limit: plan.iso_upload_limit ?? 2,
      });
    }
  });

  // Update form when plan prop changes
  if (plan && form.getValues() !== undefined) {
    const currentValues = form.getValues();
    if (currentValues.snapshot_limit !== (plan.snapshot_limit ?? 2) ||
        currentValues.backup_limit !== (plan.backup_limit ?? 2) ||
        currentValues.iso_upload_limit !== (plan.iso_upload_limit ?? 2)) {
      form.reset({
        snapshot_limit: plan.snapshot_limit ?? 2,
        backup_limit: plan.backup_limit ?? 2,
        iso_upload_limit: plan.iso_upload_limit ?? 2,
      });
    }
  }

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

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {plan ? `Edit Plan: ${plan.name}` : "Edit Plan"}
          </DialogTitle>
          <DialogDescription>
            Modify resource limits per VM for this plan.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={form.handleSubmit(handleSubmit)} className="grid gap-4 py-4">
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
            {form.formState.errors.snapshot_limit ? (
              <p className="text-xs text-destructive">{form.formState.errors.snapshot_limit.message}</p>
            ) : (
              <p className="text-xs text-muted-foreground">
                Maximum snapshots per VM (0 = unlimited)
              </p>
            )}
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
            {form.formState.errors.backup_limit ? (
              <p className="text-xs text-destructive">{form.formState.errors.backup_limit.message}</p>
            ) : (
              <p className="text-xs text-muted-foreground">
                Maximum backups per VM (0 = unlimited)
              </p>
            )}
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
            {form.formState.errors.iso_upload_limit ? (
              <p className="text-xs text-destructive">{form.formState.errors.iso_upload_limit.message}</p>
            ) : (
              <p className="text-xs text-muted-foreground">
                Maximum ISO uploads per VM (0 = unlimited)
              </p>
            )}
          </div>

          <DialogFooter>
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