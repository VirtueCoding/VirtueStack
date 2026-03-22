"use client";

import { useState, useEffect } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { AlertTriangle, Server, Plus, Search, Loader2 } from "lucide-react";
import { adminPlansApi, type Plan } from "@/lib/api-client";
import { useToast } from "@/components/ui/use-toast";
import { getStatusBadgeVariant } from "@/lib/status-badge";
import { PlanEditDialog, EditPlanFormData } from "@/components/plans/PlanEditDialog";
import { PlanCreateDialog, CreatePlanFormData } from "@/components/plans/PlanCreateDialog";
import { PlanList } from "@/components/plans/PlanList";
import { usePermissions } from "@/hooks/usePermissions";

type DialogAction = "create" | "edit" | "delete" | null;

function getStatusBadge(status: Plan["status"]) {
  const labels = {
    active: "Active",
    inactive: "Inactive",
  };

  return <Badge variant={getStatusBadgeVariant(status) as React.ComponentProps<typeof Badge>["variant"]}>{labels[status]}</Badge>;
}

function formatPrice(cents: number) {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
  }).format(cents / 100);
}

function formatMemory(mb: number) {
  if (mb >= 1024) {
    return `${(mb / 1024).toFixed(0)} GB`;
  }
  return `${mb} MB`;
}

export default function PlansPage() {
  const { hasPermission } = usePermissions();
  const canWrite = hasPermission("plans:write");
  const canDelete = hasPermission("plans:delete");

  const [searchTerm, setSearchTerm] = useState("");
  const [plans, setPlans] = useState<Plan[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [dialogAction, setDialogAction] = useState<DialogAction>(null);
  const [selectedPlan, setSelectedPlan] = useState<Plan | null>(null);
  const [saving, setSaving] = useState(false);
  const [creating, setCreating] = useState(false);
  const [planUsage, setPlanUsage] = useState<number | null>(null);
  const [loadingUsage, setLoadingUsage] = useState(false);
  const { toast } = useToast();

  useEffect(() => {
    async function loadPlans() {
      try {
        const data = await adminPlansApi.getPlans();
        setPlans(data || []);
      } catch (err) {
        setError("Failed to load plans");
        toast({
          title: "Error",
          description: "Failed to load plans.",
          variant: "destructive",
        });
      } finally {
        setLoading(false);
      }
    }
    loadPlans();
  }, [toast]);

  const filteredPlans = plans.filter((plan) =>
    plan.name.toLowerCase().includes(searchTerm.toLowerCase())
  );

  const handleEdit = (plan: Plan) => {
    setSelectedPlan(plan);
    setDialogAction("edit");
    setDialogOpen(true);
  };

  const handleCreate = () => {
    setDialogAction("create");
    setDialogOpen(true);
  };

  const handleCreatePlan = async (data: CreatePlanFormData) => {
    setCreating(true);
    try {
      const newPlan = await adminPlansApi.createPlan({
        name: data.name,
        slug: data.slug,
        vcpu: data.vcpu,
        memory_mb: data.memory_mb,
        disk_gb: data.disk_gb,
        port_speed_mbps: data.port_speed_mbps,
        bandwidth_limit_gb: data.bandwidth_limit_gb ?? 0,
        price_monthly: data.price_monthly ?? 0,
        price_hourly: data.price_hourly ?? 0,
        storage_backend: data.storage_backend,
        is_active: data.is_active,
        sort_order: data.sort_order,
        snapshot_limit: data.snapshot_limit,
        backup_limit: data.backup_limit,
        iso_upload_limit: data.iso_upload_limit,
      });
      toast({
        title: "Plan Created",
        description: `Plan "${newPlan.name}" has been created successfully.`,
      });
      setPlans((prev) => [...prev, newPlan]);
    } finally {
      setCreating(false);
    }
  };

  const openConfirmDialog = async (plan: Plan, action: DialogAction) => {
    setSelectedPlan(plan);
    setDialogAction(action);
    if (action === "delete") {
      // Fetch usage count before opening delete dialog
      setLoadingUsage(true);
      setPlanUsage(null);
      try {
        const usage = await adminPlansApi.getPlanUsage(plan.id);
        setPlanUsage(usage.vm_count);
      } catch {
        // If we can't get usage, we'll show the dialog without the warning
        setPlanUsage(null);
      } finally {
        setLoadingUsage(false);
      }
    }
    setDialogOpen(true);
  };

  const handleSaveEdit = async (data: EditPlanFormData) => {
    if (!selectedPlan) return;
    setSaving(true);
    try {
      const updated = await adminPlansApi.updatePlan(selectedPlan.id, {
        name: data.name,
        slug: data.slug,
        vcpu: data.vcpu,
        memory_mb: data.memory_mb,
        disk_gb: data.disk_gb,
        port_speed_mbps: data.port_speed_mbps,
        bandwidth_limit_gb: data.bandwidth_limit_gb,
        price_monthly: data.price_monthly,
        price_hourly: data.price_hourly,
        storage_backend: data.storage_backend,
        is_active: data.is_active,
        sort_order: data.sort_order,
        snapshot_limit: data.snapshot_limit,
        backup_limit: data.backup_limit,
        iso_upload_limit: data.iso_upload_limit,
      });
      toast({
        title: "Plan Updated",
        description: `Plan "${selectedPlan.name}" has been updated successfully.`,
      });
      setPlans((prev) => prev.map((p) => (p.id === updated.id ? { ...p, ...updated } : p)));
    } finally {
      setSaving(false);
    }
  };

  const handleDeletePlan = async () => {
    if (!selectedPlan) return;
    try {
      await adminPlansApi.deletePlan(selectedPlan.id);
      toast({
        title: "Plan Deleted",
        description: `Plan "${selectedPlan.name}" has been permanently deleted.`,
      });
      setPlans((prev) => prev.filter((p) => p.id !== selectedPlan.id));
      setDialogOpen(false);
    } catch (error) {
      toast({
        title: "Action Failed",
        description: error instanceof Error ? error.message : "Failed to delete plan",
        variant: "destructive",
      });
    }
    setSelectedPlan(null);
    setDialogAction(null);
    setPlanUsage(null);
  };

  const activePlans = plans.filter((p) => p.status === "active").length;
  const totalPlans = plans.length;

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <p className="text-destructive">{error}</p>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-background p-6 md:p-8">
      <div className="mx-auto max-w-7xl space-y-6">
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">VM Plans</h1>
            <p className="text-muted-foreground">
              Manage pricing tiers, VM specifications, and resource limits
            </p>
          </div>
          <Button size="default" onClick={handleCreate} disabled={!canWrite}>
            <Plus className="mr-2 h-4 w-4" />
            Create Plan
          </Button>
        </div>

        <Card>
          <CardContent className="pt-6">
            <div className="relative">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Search plans by name..."
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
                className="pl-10"
              />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Server className="h-5 w-5" />
              All Plans
            </CardTitle>
            <CardDescription>
              {filteredPlans.length} of {totalPlans} plans displayed
            </CardDescription>
          </CardHeader>
          <CardContent>
            <PlanList
              plans={filteredPlans}
              onEdit={handleEdit}
              onDelete={(plan) => openConfirmDialog(plan, "delete")}
              getStatusBadge={getStatusBadge}
              formatMemory={formatMemory}
              formatPrice={formatPrice}
              canWrite={canWrite}
              canDelete={canDelete}
            />
          </CardContent>
        </Card>

        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          <Card>
            <CardContent className="pt-6">
              <div className="flex items-center gap-4">
                <div className="flex h-12 w-12 items-center justify-center rounded-full bg-blue-500/10">
                  <Server className="h-5 w-5 text-blue-500" />
                </div>
                <div>
                  <div className="text-2xl font-bold">{totalPlans}</div>
                  <p className="text-xs text-muted-foreground">Total Plans</p>
                </div>
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="pt-6">
              <div className="flex items-center gap-4">
                <div className="flex h-12 w-12 items-center justify-center rounded-full bg-green-500/10">
                  <div className="h-3 w-3 rounded-full bg-green-500" />
                </div>
                <div>
                  <div className="text-2xl font-bold">{activePlans}</div>
                  <p className="text-xs text-muted-foreground">Active Plans</p>
                </div>
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="pt-6">
              <div className="flex items-center gap-4">
                <div className="flex h-12 w-12 items-center justify-center rounded-full bg-gray-500/10">
                  <div className="h-3 w-3 rounded-full bg-gray-500" />
                </div>
                <div>
                  <div className="text-2xl font-bold">
                    {totalPlans - activePlans}
                  </div>
                  <p className="text-xs text-muted-foreground">Inactive Plans</p>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>

      <PlanEditDialog
        open={dialogOpen && dialogAction === "edit"}
        onOpenChange={(open) => { setDialogOpen(open); if (!open) { setSelectedPlan(null); setDialogAction(null); }}}
        plan={selectedPlan}
        onSave={handleSaveEdit}
        isSaving={saving}
      />

      <PlanCreateDialog
        open={dialogOpen && dialogAction === "create"}
        onOpenChange={(open) => { setDialogOpen(open); if (!open) { setDialogAction(null); }}}
        onCreate={handleCreatePlan}
        isCreating={creating}
      />

      {/* Delete Dialog */}
      <Dialog open={dialogOpen && dialogAction === "delete"} onOpenChange={(open) => { setDialogOpen(open); if (!open) { setSelectedPlan(null); setDialogAction(null); setPlanUsage(null); }}}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Plan</DialogTitle>
            <DialogDescription>
              Are you sure you want to permanently delete plan &quot;{selectedPlan?.name}&quot;? This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          {loadingUsage ? (
            <div className="flex items-center justify-center py-4">
              <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
            </div>
          ) : planUsage !== null && planUsage > 0 ? (
            <div className="flex items-start gap-3 rounded-md border border-yellow-200 bg-yellow-50 p-4 dark:border-yellow-900 dark:bg-yellow-950">
              <AlertTriangle className="h-5 w-5 text-yellow-600 dark:text-yellow-500 shrink-0" />
              <div className="text-sm text-yellow-800 dark:text-yellow-200">
                <span className="font-medium">Warning:</span> This plan has <span className="font-semibold">{planUsage} VM{planUsage !== 1 ? "s" : ""}</span> associated. Deletion will fail due to foreign key constraints. Migrate or delete the VMs first.
              </div>
            </div>
          ) : null}
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDeletePlan}
              disabled={loadingUsage || (planUsage !== null && planUsage > 0)}
            >
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}