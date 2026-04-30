"use client";

import { useEffect, useMemo, useState } from "react";
import { z } from "zod";
import { useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { CalendarClock, Loader2, Users } from "lucide-react";
import { Badge } from "@virtuestack/ui";
import { Button } from "@virtuestack/ui";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@virtuestack/ui";
import { Input } from "@virtuestack/ui";
import { Label } from "@virtuestack/ui";
import { ScrollArea } from "@virtuestack/ui";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@virtuestack/ui";
import { Switch } from "@virtuestack/ui";
import { Textarea } from "@/components/ui/textarea";
import { useToast } from "@virtuestack/ui";
import {
  adminBackupSchedulesApi,
  adminCustomersApi,
  adminNodesApi,
  adminPlansApi,
  type AdminBackupSchedule,
  type Customer,
  type Node,
  type Plan,
} from "@/lib/api-client";

const adminScheduleSchema = z
  .object({
    name: z.string().min(1, "Name is required").max(100, "Name must be 100 characters or less"),
    description: z.string().max(500, "Description must be 500 characters or less").optional(),
    frequency: z.enum(["daily", "weekly", "monthly"]),
    retention_count: z.coerce.number().int().min(1, "Retention must be at least 1").max(52, "Retention cannot exceed 52"),
    target_all: z.boolean(),
    target_plan_ids: z.array(z.string()).default([]),
    target_node_ids: z.array(z.string()).default([]),
    target_customer_ids: z.array(z.string()).default([]),
    active: z.boolean(),
  })
  .superRefine((value, ctx) => {
    if (
      !value.target_all &&
      value.target_plan_ids.length === 0 &&
      value.target_node_ids.length === 0 &&
      value.target_customer_ids.length === 0
    ) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "Select at least one target or enable “Target all VMs”.",
        path: ["target_all"],
      });
    }
  });

type AdminScheduleFormData = z.infer<typeof adminScheduleSchema>;

interface CreateScheduleModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  editSchedule: AdminBackupSchedule | null;
  onSaved: () => void;
}

const defaultValues: AdminScheduleFormData = {
  name: "",
  description: "",
  frequency: "daily",
  retention_count: 7,
  target_all: false,
  target_plan_ids: [],
  target_node_ids: [],
  target_customer_ids: [],
  active: true,
};

function buildFormValues(schedule: AdminBackupSchedule | null): AdminScheduleFormData {
  if (!schedule) {
    return defaultValues;
  }

  return {
    name: schedule.name,
    description: schedule.description || "",
    frequency: schedule.frequency as "daily" | "weekly" | "monthly",
    retention_count: schedule.retention_count,
    target_all: schedule.target_all,
    target_plan_ids: schedule.target_plan_ids || [],
    target_node_ids: schedule.target_node_ids || [],
    target_customer_ids: schedule.target_customer_ids || [],
    active: schedule.active,
  };
}

interface TargetPickerProps {
  title: string;
  description: string;
  items: Array<{ id: string; label: string; meta?: string }>;
  selectedIds: string[];
  disabled?: boolean;
  onToggle: (id: string, checked: boolean) => void;
}

function TargetPicker({
  title,
  description,
  items,
  selectedIds,
  disabled,
  onToggle,
}: TargetPickerProps) {
  return (
    <div className="space-y-3 rounded-lg border p-4">
      <div>
        <div className="flex items-center gap-2">
          <Users className="h-4 w-4 text-muted-foreground" />
          <h4 className="font-medium">{title}</h4>
          <Badge variant="outline">{selectedIds.length} selected</Badge>
        </div>
        <p className="mt-1 text-xs text-muted-foreground">{description}</p>
      </div>

      <ScrollArea className="h-48 rounded-md border">
        <div className="space-y-2 p-3">
          {items.length === 0 ? (
            <p className="text-sm text-muted-foreground">No available items.</p>
          ) : (
            items.map((item) => {
              const checked = selectedIds.includes(item.id);

              return (
                <label
                  key={item.id}
                  className={`flex cursor-pointer items-start gap-3 rounded-md p-2 transition-colors ${
                    disabled ? "cursor-not-allowed opacity-60" : "hover:bg-muted/60"
                  }`}
                >
                  <Checkbox
                    checked={checked}
                    disabled={disabled}
                    onCheckedChange={(value) => onToggle(item.id, Boolean(value))}
                  />
                  <div className="space-y-1">
                    <p className="text-sm font-medium">{item.label}</p>
                    {item.meta && <p className="text-xs text-muted-foreground">{item.meta}</p>}
                  </div>
                </label>
              );
            })
          )}
        </div>
      </ScrollArea>
    </div>
  );
}

export function CreateScheduleModal({
  open,
  onOpenChange,
  editSchedule,
  onSaved,
}: CreateScheduleModalProps) {
  const { toast } = useToast();
  const [plans, setPlans] = useState<Plan[]>([]);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [customers, setCustomers] = useState<Customer[]>([]);
  const [loadingTargets, setLoadingTargets] = useState(false);

  const form = useForm<AdminScheduleFormData>({
    resolver: zodResolver(adminScheduleSchema),
    defaultValues,
  });

  useEffect(() => {
    if (!open) {
      return;
    }

    form.reset(buildFormValues(editSchedule));
  }, [editSchedule, form, open]);

  useEffect(() => {
    if (!open) {
      return;
    }

    let isMounted = true;

    async function loadTargets() {
      setLoadingTargets(true);

      try {
        const [planData, nodeData, customerData] = await Promise.all([
          adminPlansApi.getPlans(),
          adminNodesApi.getNodes(),
          adminCustomersApi.getCustomers(),
        ]);

        if (!isMounted) return;

        setPlans(planData || []);
        setNodes(nodeData.data || []);
        setCustomers(customerData.data || []);
      } catch (error) {
        toast({
          title: "Failed to load schedule targets",
          description: error instanceof Error ? error.message : "Unable to load plans, nodes, or customers.",
          variant: "destructive",
        });
      } finally {
        if (isMounted) {
          setLoadingTargets(false);
        }
      }
    }

    void loadTargets();

    return () => {
      isMounted = false;
    };
  }, [open, toast]);

  const targetAll = useWatch({ control: form.control, name: "target_all" });
  const frequency = useWatch({ control: form.control, name: "frequency" });
  const active = useWatch({ control: form.control, name: "active" });
  const selectedPlanIds = useWatch({ control: form.control, name: "target_plan_ids" }) || [];
  const selectedNodeIds = useWatch({ control: form.control, name: "target_node_ids" }) || [];
  const selectedCustomerIds = useWatch({ control: form.control, name: "target_customer_ids" }) || [];

  const targetSummary = useMemo(() => {
    if (targetAll) {
      return "This schedule will apply to all eligible virtual machines.";
    }

    const parts = [
      selectedPlanIds.length ? `${selectedPlanIds.length} plan target${selectedPlanIds.length === 1 ? "" : "s"}` : null,
      selectedNodeIds.length ? `${selectedNodeIds.length} node target${selectedNodeIds.length === 1 ? "" : "s"}` : null,
      selectedCustomerIds.length ? `${selectedCustomerIds.length} customer target${selectedCustomerIds.length === 1 ? "" : "s"}` : null,
    ].filter(Boolean);

    return parts.length > 0 ? parts.join(" • ") : "No targets selected yet.";
  }, [selectedCustomerIds.length, selectedNodeIds.length, selectedPlanIds.length, targetAll]);

  const toggleSelection = (
    field: "target_plan_ids" | "target_node_ids" | "target_customer_ids",
    id: string,
    checked: boolean
  ) => {
    const current = form.getValues(field) || [];
    form.setValue(
      field,
      checked ? [...current, id] : current.filter((value) => value !== id),
      { shouldValidate: true }
    );
  };

  const handleSubmit = async (values: AdminScheduleFormData) => {
    try {
      if (editSchedule) {
        await adminBackupSchedulesApi.updateSchedule(editSchedule.id, values);
      } else {
        await adminBackupSchedulesApi.createSchedule(values);
      }

      toast({
        title: editSchedule ? "Schedule updated" : "Schedule created",
        description: `${values.name} has been saved successfully.`,
      });
      onOpenChange(false);
      onSaved();
    } catch (error) {
      toast({
        title: editSchedule ? "Update failed" : "Creation failed",
        description: error instanceof Error ? error.message : "Unable to save the backup schedule.",
        variant: "destructive",
      });
    }
  };

  const planOptions = plans.map((plan) => ({
    id: plan.id,
    label: plan.name,
    meta: `${plan.vcpu} vCPU • ${plan.memory_mb} MB • ${plan.storage_backend}`,
  }));
  const nodeOptions = nodes.map((node) => ({
    id: node.id,
    label: node.hostname || node.name,
    meta: `${node.location} • ${node.status} • ${node.vm_count} VM${node.vm_count === 1 ? "" : "s"}`,
  }));
  const customerOptions = customers.map((customer) => ({
    id: customer.id,
    label: customer.name,
    meta: `${customer.email} • ${customer.vm_count} VM${customer.vm_count === 1 ? "" : "s"}`,
  }));

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-5xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <CalendarClock className="h-5 w-5" />
            {editSchedule ? "Edit Backup Schedule" : "Create Backup Schedule"}
          </DialogTitle>
          <DialogDescription>
            Configure a recurring admin-managed backup campaign and target the plans, nodes, or customers it should cover.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-6 py-4">
          <div className="grid gap-6 lg:grid-cols-[1.2fr_0.8fr]">
            <div className="space-y-6">
              <div className="space-y-4 rounded-lg border p-4">
                <h3 className="text-sm font-medium uppercase tracking-wide text-muted-foreground">
                  Basic Information
                </h3>
                <div className="space-y-2">
                  <Label htmlFor="schedule-name">Schedule name</Label>
                  <Input id="schedule-name" placeholder="e.g. Weekly production backups" {...form.register("name")} />
                  {form.formState.errors.name && (
                    <p className="text-xs text-destructive">{form.formState.errors.name.message}</p>
                  )}
                </div>
                <div className="space-y-2">
                  <Label htmlFor="schedule-description">Description</Label>
                  <Textarea
                    id="schedule-description"
                    placeholder="Describe what this campaign protects and when it should run."
                    {...form.register("description")}
                  />
                  {form.formState.errors.description && (
                    <p className="text-xs text-destructive">{form.formState.errors.description.message}</p>
                  )}
                </div>
              </div>

              <div className="space-y-4 rounded-lg border p-4">
                <h3 className="text-sm font-medium uppercase tracking-wide text-muted-foreground">
                  Schedule Settings
                </h3>
                <div className="grid gap-4 md:grid-cols-2">
                  <div className="space-y-2">
                    <Label>Frequency</Label>
                    <Select
                      value={frequency}
                      onValueChange={(value: "daily" | "weekly" | "monthly") =>
                        form.setValue("frequency", value, { shouldValidate: true })
                      }
                    >
                      <SelectTrigger>
                        <SelectValue placeholder="Select frequency" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="daily">Daily</SelectItem>
                        <SelectItem value="weekly">Weekly</SelectItem>
                        <SelectItem value="monthly">Monthly</SelectItem>
                      </SelectContent>
                    </Select>
                    {form.formState.errors.frequency && (
                      <p className="text-xs text-destructive">{form.formState.errors.frequency.message}</p>
                    )}
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="retention-count">Retention count</Label>
                    <Input
                      id="retention-count"
                      type="number"
                      min={1}
                      max={52}
                      {...form.register("retention_count", { valueAsNumber: true })}
                    />
                    {form.formState.errors.retention_count && (
                      <p className="text-xs text-destructive">{form.formState.errors.retention_count.message}</p>
                    )}
                  </div>
                </div>

                <div className="grid gap-4 md:grid-cols-2">
                  <div className="flex items-center justify-between rounded-md border p-3">
                    <div>
                      <Label htmlFor="schedule-active">Schedule active</Label>
                      <p className="text-xs text-muted-foreground">
                        Disabled schedules stay saved but won’t execute automatically.
                      </p>
                    </div>
                    <Switch
                      id="schedule-active"
                      checked={active}
                      onCheckedChange={(checked) => form.setValue("active", checked)}
                    />
                  </div>

                  <div className="flex items-center justify-between rounded-md border p-3">
                    <div>
                      <Label htmlFor="schedule-target-all">Target all VMs</Label>
                      <p className="text-xs text-muted-foreground">
                        Apply the schedule globally instead of selecting specific scopes.
                      </p>
                    </div>
                    <Switch
                      id="schedule-target-all"
                      checked={targetAll}
                      onCheckedChange={(checked) => form.setValue("target_all", checked, { shouldValidate: true })}
                    />
                  </div>
                </div>

                <div className="rounded-md bg-muted/50 p-3 text-sm text-muted-foreground">
                  {targetSummary}
                </div>
                {form.formState.errors.target_all && (
                  <p className="text-xs text-destructive">{form.formState.errors.target_all.message}</p>
                )}
              </div>
            </div>

            <div className="space-y-4">
              {loadingTargets ? (
                <div className="flex min-h-64 items-center justify-center rounded-lg border">
                  <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
                </div>
              ) : (
                <>
                  <TargetPicker
                    title="Plans"
                    description="Target every VM provisioned on selected plans."
                    items={planOptions}
                    selectedIds={selectedPlanIds}
                    disabled={targetAll}
                    onToggle={(id, checked) => toggleSelection("target_plan_ids", id, checked)}
                  />
                  <TargetPicker
                    title="Nodes"
                    description="Target VMs currently assigned to selected compute nodes."
                    items={nodeOptions}
                    selectedIds={selectedNodeIds}
                    disabled={targetAll}
                    onToggle={(id, checked) => toggleSelection("target_node_ids", id, checked)}
                  />
                  <TargetPicker
                    title="Customers"
                    description="Target all VMs owned by selected customers."
                    items={customerOptions}
                    selectedIds={selectedCustomerIds}
                    disabled={targetAll}
                    onToggle={(id, checked) => toggleSelection("target_customer_ids", id, checked)}
                  />
                </>
              )}
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" type="button" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit">
              {editSchedule ? "Save Changes" : "Create Schedule"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
