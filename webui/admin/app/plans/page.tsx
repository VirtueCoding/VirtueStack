"use client";

import { useState, useEffect } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Server,
  Plus,
  Search,
  Edit,
  Trash2,
  Cpu,
  MemoryStick,
  HardDrive,
  Network,
  DollarSign,
  Loader2,
  Camera,
  Archive,
  Disc,
} from "lucide-react";
import { adminPlansApi, type Plan, type UpdatePlanRequest } from "@/lib/api-client";
import { useToast } from "@/components/ui/use-toast";
import { getStatusBadgeVariant } from "@/lib/status-badge";

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

type DialogAction = "edit" | "delete" | null;

export default function PlansPage() {
  const [searchTerm, setSearchTerm] = useState("");
  const [plans, setPlans] = useState<Plan[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [dialogAction, setDialogAction] = useState<DialogAction>(null);
  const [selectedPlan, setSelectedPlan] = useState<Plan | null>(null);
  const [saving, setSaving] = useState(false);
  const { toast } = useToast();

  const [editForm, setEditForm] = useState<UpdatePlanRequest>({});

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
    setEditForm({
      snapshot_limit: plan.snapshot_limit,
      backup_limit: plan.backup_limit,
      iso_upload_limit: plan.iso_upload_limit,
    });
    setDialogAction("edit");
    setDialogOpen(true);
  };

  const openConfirmDialog = (plan: Plan, action: DialogAction) => {
    setSelectedPlan(plan);
    setDialogAction(action);
    setDialogOpen(true);
  };

  const handleConfirmAction = async () => {
    if (!dialogAction) return;

    if (dialogAction === "edit" && selectedPlan) {
      setSaving(true);
      try {
        const updated = await adminPlansApi.updatePlan(selectedPlan.id, editForm);
        toast({
          title: "Plan Updated",
          description: `Plan "${selectedPlan.name}" limits updated successfully.`,
        });
        setPlans((prev) => prev.map((p) => (p.id === updated.id ? { ...p, ...updated } : p)));
        setDialogOpen(false);
      } catch (err) {
        toast({
          title: "Update Failed",
          description: err instanceof Error ? err.message : "Failed to update plan",
          variant: "destructive",
        });
      } finally {
        setSaving(false);
      }
    } else if (dialogAction === "delete" && selectedPlan) {
      setDialogOpen(false);
      try {
        await adminPlansApi.deletePlan(selectedPlan.id);
        toast({
          title: "Plan Deleted",
          description: `Plan "${selectedPlan.name}" has been permanently deleted.`,
        });
        setPlans((prev) => prev.filter((p) => p.id !== selectedPlan.id));
      } catch (error) {
        toast({
          title: "Action Failed",
          description: error instanceof Error ? error.message : "Failed to delete plan",
          variant: "destructive",
        });
      }
    }

    setSelectedPlan(null);
    setDialogAction(null);
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
          <Button size="default" disabled>
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
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>vCPU</TableHead>
                    <TableHead>Memory</TableHead>
                    <TableHead>Disk</TableHead>
                    <TableHead>Bandwidth</TableHead>
                    <TableHead>Price/Month</TableHead>
                    <TableHead className="text-center">Snapshots</TableHead>
                    <TableHead className="text-center">Backups</TableHead>
                    <TableHead className="text-center">ISOs</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredPlans.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={11} className="h-24 text-center">
                        No plans found
                      </TableCell>
                    </TableRow>
                  ) : (
                    filteredPlans.map((plan) => (
                      <TableRow key={plan.id}>
                        <TableCell>
                          <div className="font-medium">{plan.name}</div>
                        </TableCell>
                        <TableCell>{getStatusBadge(plan.status)}</TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            <Cpu className="h-4 w-4 text-muted-foreground" />
                            <span>{plan.vcpu}</span>
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            <MemoryStick className="h-4 w-4 text-muted-foreground" />
                            <span>{formatMemory(plan.memory_mb)}</span>
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            <HardDrive className="h-4 w-4 text-muted-foreground" />
                            <span>{plan.disk_gb} GB</span>
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            <Network className="h-4 w-4 text-muted-foreground" />
                            <span>{plan.bandwidth_mbps} Mbps</span>
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2 font-semibold text-foreground">
                            <DollarSign className="h-4 w-4 text-muted-foreground" />
                            <span>{formatPrice(plan.price_monthly)}</span>
                          </div>
                        </TableCell>
                        <TableCell className="text-center">
                          <div className="flex items-center justify-center gap-1">
                            <Camera className="h-3.5 w-3.5 text-muted-foreground" />
                            <span>{plan.snapshot_limit ?? 2}</span>
                          </div>
                        </TableCell>
                        <TableCell className="text-center">
                          <div className="flex items-center justify-center gap-1">
                            <Archive className="h-3.5 w-3.5 text-muted-foreground" />
                            <span>{plan.backup_limit ?? 2}</span>
                          </div>
                        </TableCell>
                        <TableCell className="text-center">
                          <div className="flex items-center justify-center gap-1">
                            <Disc className="h-3.5 w-3.5 text-muted-foreground" />
                            <span>{plan.iso_upload_limit ?? 2}</span>
                          </div>
                        </TableCell>
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-2">
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => handleEdit(plan)}
                            >
                              <Edit className="mr-1 h-3 w-3" />
                              Edit
                            </Button>
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => openConfirmDialog(plan, "delete")}
                              className="text-destructive hover:bg-destructive/10"
                            >
                              <Trash2 className="mr-1 h-3 w-3" />
                              Delete
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>
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

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className={dialogAction === "edit" ? "sm:max-w-lg" : ""}>
          <DialogHeader>
            <DialogTitle>
              {dialogAction === "edit" && selectedPlan
                ? `Edit Plan: ${selectedPlan.name}`
                : dialogAction === "delete" && selectedPlan
                  ? "Delete Plan"
                  : ""}
            </DialogTitle>
            <DialogDescription>
              {dialogAction === "edit" && selectedPlan
                ? "Modify resource limits per VM for this plan."
                : dialogAction === "delete" && selectedPlan
                  ? `Are you sure you want to permanently delete plan "${selectedPlan.name}"? This action cannot be undone.`
                  : ""}
            </DialogDescription>
          </DialogHeader>

          {dialogAction === "edit" && selectedPlan && (
            <div className="grid gap-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="snapshot_limit" className="flex items-center gap-2">
                  <Camera className="h-4 w-4 text-muted-foreground" />
                  Snapshot Limit
                </Label>
                <Input
                  id="snapshot_limit"
                  type="number"
                  min={0}
                  value={editForm.snapshot_limit ?? selectedPlan.snapshot_limit ?? 2}
                  onChange={(e) =>
                    setEditForm((prev) => ({
                      ...prev,
                      snapshot_limit: parseInt(e.target.value) || 0,
                    }))
                  }
                />
                <p className="text-xs text-muted-foreground">
                  Maximum snapshots per VM (0 = unlimited)
                </p>
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
                  value={editForm.backup_limit ?? selectedPlan.backup_limit ?? 2}
                  onChange={(e) =>
                    setEditForm((prev) => ({
                      ...prev,
                      backup_limit: parseInt(e.target.value) || 0,
                    }))
                  }
                />
                <p className="text-xs text-muted-foreground">
                  Maximum backups per VM (0 = unlimited)
                </p>
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
                  value={editForm.iso_upload_limit ?? selectedPlan.iso_upload_limit ?? 2}
                  onChange={(e) =>
                    setEditForm((prev) => ({
                      ...prev,
                      iso_upload_limit: parseInt(e.target.value) || 0,
                    }))
                  }
                />
                <p className="text-xs text-muted-foreground">
                  Maximum ISO uploads per VM (0 = unlimited)
                </p>
              </div>
            </div>
          )}

          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)} disabled={saving}>
              Cancel
            </Button>
            <Button
              variant={dialogAction === "delete" ? "destructive" : "default"}
              onClick={handleConfirmAction}
              disabled={saving}
            >
              {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {dialogAction === "delete" ? "Delete" : "Save Changes"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
