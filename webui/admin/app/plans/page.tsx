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
} from "lucide-react";
import { adminPlansApi, type Plan } from "@/lib/api-client";
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

  const openConfirmDialog = (plan: Plan, action: DialogAction) => {
    setSelectedPlan(plan);
    setDialogAction(action);
    setDialogOpen(true);
  };

  const handleConfirmAction = async () => {
    if (!dialogAction) return;

    setDialogOpen(false);

    if (dialogAction === "delete" && selectedPlan) {
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
          description: error instanceof Error ? error.message : `Failed to ${dialogAction} plan`,
          variant: "destructive",
        });
      }
    } else if (dialogAction === "edit") {
      toast({
        title: "Coming Soon",
        description: "Plan editing form is not yet available.",
      });
    }

    setSelectedPlan(null);
    setDialogAction(null);
  };

  const getDialogContent = () => {
    if (!dialogAction) return null;

    if (dialogAction === "edit") {
      return {
        title: selectedPlan ? `Edit Plan: ${selectedPlan.name}` : "Create Plan",
        description: selectedPlan
          ? `Modify specifications for "${selectedPlan.name}". ${selectedPlan.vcpu} vCPU, ${formatMemory(selectedPlan.memory_mb)} RAM, ${selectedPlan.disk_gb} GB disk.`
          : "Define specifications and pricing for the new plan.",
        confirmText: selectedPlan ? "Save Changes" : "Create Plan",
        variant: "default" as const,
      };
    }

    if (dialogAction === "delete" && selectedPlan) {
      return {
        title: "Delete Plan",
        description: `Are you sure you want to permanently delete plan "${selectedPlan.name}"? This action cannot be undone.`,
        confirmText: "Delete",
        variant: "destructive" as const,
      };
    }

    return null;
  };

  const dialogContent = getDialogContent();

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
              Manage pricing tiers and VM specifications
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
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredPlans.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={8} className="h-24 text-center">
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
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-2">
                            <Button
                              variant="outline"
                              size="sm"
                              disabled
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
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{dialogContent?.title}</DialogTitle>
            <DialogDescription>{dialogContent?.description}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>
              Cancel
            </Button>
            <Button 
              variant={dialogContent?.variant || "default"}
              onClick={handleConfirmAction}
            >
              {dialogContent?.confirmText}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
