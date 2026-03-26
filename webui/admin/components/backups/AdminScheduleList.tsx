"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { CalendarClock, Loader2, Pencil, Play, Plus, RefreshCw, Search, Trash2, Users } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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
import { useToast } from "@/components/ui/use-toast";
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

type ScheduleStatusFilter = "all" | "active" | "inactive";
type ScheduleFrequencyFilter = "all" | "daily" | "weekly" | "monthly";

export interface AdminScheduleSummary {
  total: number;
  active: number;
  frequencyCounts: Record<"daily" | "weekly" | "monthly", number>;
  targetedPlans: number;
  targetedNodes: number;
  targetedCustomers: number;
}

interface AdminScheduleListProps {
  onEditSchedule: (schedule: AdminBackupSchedule) => void;
  onCreateSchedule: () => void;
  refreshKey?: number;
  onSummaryChange?: (summary: AdminScheduleSummary) => void;
}

const emptySummary: AdminScheduleSummary = {
  total: 0,
  active: 0,
  frequencyCounts: {
    daily: 0,
    weekly: 0,
    monthly: 0,
  },
  targetedPlans: 0,
  targetedNodes: 0,
  targetedCustomers: 0,
};

function formatDateTime(value?: string) {
  if (!value) return "Never";

  try {
    return new Intl.DateTimeFormat("en-US", {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(new Date(value));
  } catch {
    return value;
  }
}

function getTargetCountLabel(label: string, count: number) {
  return `${count} ${label}${count === 1 ? "" : "s"}`;
}

export function AdminScheduleList({
  onEditSchedule,
  onCreateSchedule,
  refreshKey = 0,
  onSummaryChange,
}: AdminScheduleListProps) {
  const { toast } = useToast();
  const [schedules, setSchedules] = useState<AdminBackupSchedule[]>([]);
  const [plans, setPlans] = useState<Plan[]>([]);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [customers, setCustomers] = useState<Customer[]>([]);
  const [searchTerm, setSearchTerm] = useState("");
  const [statusFilter, setStatusFilter] = useState<ScheduleStatusFilter>("all");
  const [frequencyFilter, setFrequencyFilter] = useState<ScheduleFrequencyFilter>("all");
  const [loading, setLoading] = useState(true);
  const [busyActionId, setBusyActionId] = useState<string | null>(null);
  const [deleteCandidate, setDeleteCandidate] = useState<AdminBackupSchedule | null>(null);

  const planMap = useMemo(
    () => new Map(plans.map((plan) => [plan.id, plan.name])),
    [plans]
  );
  const nodeMap = useMemo(
    () => new Map(nodes.map((node) => [node.id, node.hostname || node.name])),
    [nodes]
  );
  const customerMap = useMemo(
    () => new Map(customers.map((customer) => [customer.id, customer.name])),
    [customers]
  );

  const loadSchedules = useCallback(async () => {
    setLoading(true);
    try {
      const [scheduleResponse, plansResponse, nodesResponse, customersResponse] = await Promise.all([
        adminBackupSchedulesApi.getSchedules({ page: 1, per_page: 100 }),
        adminPlansApi.getPlans(),
        adminNodesApi.getNodes(),
        adminCustomersApi.getCustomers(),
      ]);

      setSchedules(scheduleResponse.data || []);
      setPlans(plansResponse || []);
      setNodes(nodesResponse.data || []);
      setCustomers(customersResponse.data || []);
    } catch (error) {
      toast({
        title: "Failed to load backup schedules",
        description: error instanceof Error ? error.message : "Unable to load backup schedule data.",
        variant: "destructive",
      });
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    void loadSchedules();
  }, [loadSchedules, refreshKey]);

  const summary = useMemo<AdminScheduleSummary>(() => {
    const uniquePlans = new Set<string>();
    const uniqueNodes = new Set<string>();
    const uniqueCustomers = new Set<string>();

    const nextSummary = schedules.reduce<AdminScheduleSummary>(
      (acc, schedule) => {
        acc.total += 1;
        if (schedule.active) {
          acc.active += 1;
        }
        acc.frequencyCounts[schedule.frequency as "daily" | "weekly" | "monthly"] += 1;

        schedule.target_plan_ids?.forEach((id) => uniquePlans.add(id));
        schedule.target_node_ids?.forEach((id) => uniqueNodes.add(id));
        schedule.target_customer_ids?.forEach((id) => uniqueCustomers.add(id));

        return acc;
      },
      {
        total: 0,
        active: 0,
        frequencyCounts: { daily: 0, weekly: 0, monthly: 0 },
        targetedPlans: 0,
        targetedNodes: 0,
        targetedCustomers: 0,
      }
    );

    nextSummary.targetedPlans = uniquePlans.size;
    nextSummary.targetedNodes = uniqueNodes.size;
    nextSummary.targetedCustomers = uniqueCustomers.size;

    return nextSummary;
  }, [schedules]);

  useEffect(() => {
    onSummaryChange?.(summary);
  }, [onSummaryChange, summary]);

  const filteredSchedules = useMemo(() => {
    const needle = searchTerm.trim().toLowerCase();

    return schedules.filter((schedule) => {
      if (statusFilter === "active" && !schedule.active) return false;
      if (statusFilter === "inactive" && schedule.active) return false;
      if (frequencyFilter !== "all" && schedule.frequency !== frequencyFilter) return false;

      if (!needle) return true;

      const targets = [
        ...(schedule.target_plan_ids || []).map((id) => planMap.get(id) || id),
        ...(schedule.target_node_ids || []).map((id) => nodeMap.get(id) || id),
        ...(schedule.target_customer_ids || []).map((id) => customerMap.get(id) || id),
      ]
        .join(" ")
        .toLowerCase();

      return (
        schedule.name.toLowerCase().includes(needle) ||
        (schedule.description || "").toLowerCase().includes(needle) ||
        targets.includes(needle)
      );
    });
  }, [customerMap, frequencyFilter, nodeMap, planMap, schedules, searchTerm, statusFilter]);

  const describeTargets = useCallback(
    (schedule: AdminBackupSchedule) => {
      if (schedule.target_all) {
        return {
          primary: "All eligible VMs",
          details: ["Applies to every VM covered by the scheduler"],
        };
      }

      const details: string[] = [];

      if (schedule.target_plan_ids?.length) {
        const names = schedule.target_plan_ids.map((id) => planMap.get(id) || id);
        details.push(`Plans: ${names.join(", ")}`);
      }
      if (schedule.target_node_ids?.length) {
        const names = schedule.target_node_ids.map((id) => nodeMap.get(id) || id);
        details.push(`Nodes: ${names.join(", ")}`);
      }
      if (schedule.target_customer_ids?.length) {
        const names = schedule.target_customer_ids.map((id) => customerMap.get(id) || id);
        details.push(`Customers: ${names.join(", ")}`);
      }

      return {
        primary: [
          schedule.target_plan_ids?.length ? getTargetCountLabel("plan", schedule.target_plan_ids.length) : null,
          schedule.target_node_ids?.length ? getTargetCountLabel("node", schedule.target_node_ids.length) : null,
          schedule.target_customer_ids?.length ? getTargetCountLabel("customer", schedule.target_customer_ids.length) : null,
        ]
          .filter(Boolean)
          .join(" • "),
        details,
      };
    },
    [customerMap, nodeMap, planMap]
  );

  const handleRunNow = async (schedule: AdminBackupSchedule) => {
    setBusyActionId(schedule.id);
    try {
      await adminBackupSchedulesApi.runSchedule(schedule.id);
      toast({
        title: "Schedule triggered",
        description: `${schedule.name} has been queued for immediate execution.`,
      });
      await loadSchedules();
    } catch (error) {
      toast({
        title: "Run failed",
        description: error instanceof Error ? error.message : `Unable to trigger ${schedule.name}.`,
        variant: "destructive",
      });
    } finally {
      setBusyActionId(null);
    }
  };

  const handleDelete = async () => {
    if (!deleteCandidate) return;

    setBusyActionId(deleteCandidate.id);
    try {
      await adminBackupSchedulesApi.deleteSchedule(deleteCandidate.id);
      toast({
        title: "Schedule deleted",
        description: `${deleteCandidate.name} has been removed.`,
      });
      setDeleteCandidate(null);
      await loadSchedules();
    } catch (error) {
      toast({
        title: "Delete failed",
        description: error instanceof Error ? error.message : `Unable to delete ${deleteCandidate.name}.`,
        variant: "destructive",
      });
    } finally {
      setBusyActionId(null);
    }
  };

  return (
    <>
      <Card>
        <CardHeader className="space-y-4">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
            <div>
              <CardTitle>Backup Schedule Directory</CardTitle>
              <CardDescription>
                Create, review, and trigger recurring backup campaigns across plans, nodes, and customers.
              </CardDescription>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button variant="outline" onClick={() => void loadSchedules()} disabled={loading}>
                {loading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RefreshCw className="mr-2 h-4 w-4" />}
                Refresh
              </Button>
              <Button onClick={onCreateSchedule}>
                <Plus className="mr-2 h-4 w-4" />
                New Schedule
              </Button>
            </div>
          </div>

          <div className="grid gap-3 md:grid-cols-3">
            <div className="relative">
              <Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
              <Input
                className="pl-9"
                placeholder="Search schedules or targets"
                value={searchTerm}
                onChange={(event) => setSearchTerm(event.target.value)}
              />
            </div>
            <Select value={statusFilter} onValueChange={(value: ScheduleStatusFilter) => setStatusFilter(value)}>
              <SelectTrigger>
                <SelectValue placeholder="Filter by status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All statuses</SelectItem>
                <SelectItem value="active">Active only</SelectItem>
                <SelectItem value="inactive">Inactive only</SelectItem>
              </SelectContent>
            </Select>
            <Select
              value={frequencyFilter}
              onValueChange={(value: ScheduleFrequencyFilter) => setFrequencyFilter(value)}
            >
              <SelectTrigger>
                <SelectValue placeholder="Filter by frequency" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All frequencies</SelectItem>
                <SelectItem value="daily">Daily</SelectItem>
                <SelectItem value="weekly">Weekly</SelectItem>
                <SelectItem value="monthly">Monthly</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </CardHeader>

        <CardContent>
          {loading ? (
            <div className="flex min-h-48 items-center justify-center">
              <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
            </div>
          ) : filteredSchedules.length === 0 ? (
            <div className="rounded-lg border border-dashed p-8 text-center">
              <CalendarClock className="mx-auto h-10 w-10 text-muted-foreground" />
              <h3 className="mt-4 text-lg font-semibold">No backup schedules found</h3>
              <p className="mt-2 text-sm text-muted-foreground">
                Adjust the current filters or create a new backup campaign to get started.
              </p>
              <Button className="mt-4" onClick={onCreateSchedule}>
                <Plus className="mr-2 h-4 w-4" />
                Create Schedule
              </Button>
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Schedule</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Targets</TableHead>
                  <TableHead>Run Window</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredSchedules.map((schedule) => {
                  const targets = describeTargets(schedule);
                  const isBusy = busyActionId === schedule.id;

                  return (
                    <TableRow key={schedule.id}>
                      <TableCell className="align-top">
                        <div className="space-y-2">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="font-medium">{schedule.name}</span>
                            <Badge variant={schedule.active ? "default" : "secondary"}>
                              {schedule.active ? "Active" : "Inactive"}
                            </Badge>
                            <Badge variant="outline" className="capitalize">
                              {schedule.frequency}
                            </Badge>
                          </div>
                          <p className="text-sm text-muted-foreground">
                            {schedule.description || "No description provided."}
                          </p>
                          <div className="flex flex-wrap gap-3 text-xs text-muted-foreground">
                            <span>Retention: {schedule.retention_count} backups</span>
                            <span>Created: {formatDateTime(schedule.created_at)}</span>
                          </div>
                        </div>
                      </TableCell>
                      <TableCell className="align-top">
                        <div className="space-y-2 text-sm">
                          <div className="flex items-center gap-2">
                            <Users className="h-4 w-4 text-muted-foreground" />
                            <span>{schedule.active ? "Enabled" : "Paused"}</span>
                          </div>
                          {schedule.last_run_at ? (
                            <p className="text-xs text-muted-foreground">
                              Last run: {formatDateTime(schedule.last_run_at)}
                            </p>
                          ) : (
                            <p className="text-xs text-muted-foreground">Last run: not yet executed</p>
                          )}
                        </div>
                      </TableCell>
                      <TableCell className="align-top">
                        <div className="space-y-2">
                          <p className="text-sm font-medium">{targets.primary}</p>
                          <div className="space-y-1 text-xs text-muted-foreground">
                            {targets.details.map((detail) => (
                              <p key={detail}>{detail}</p>
                            ))}
                          </div>
                        </div>
                      </TableCell>
                      <TableCell className="align-top">
                        <div className="space-y-1 text-sm">
                          <p>Next run: {formatDateTime(schedule.next_run_at)}</p>
                          <p className="text-xs text-muted-foreground">
                            Frequency: {schedule.frequency}, retaining {schedule.retention_count}
                          </p>
                        </div>
                      </TableCell>
                      <TableCell className="align-top">
                        <div className="flex justify-end gap-2">
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => void handleRunNow(schedule)}
                            disabled={isBusy}
                          >
                            {isBusy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
                          </Button>
                          <Button variant="outline" size="sm" onClick={() => onEditSchedule(schedule)} disabled={isBusy}>
                            <Pencil className="h-4 w-4" />
                          </Button>
                          <Button
                            variant="destructive"
                            size="sm"
                            onClick={() => setDeleteCandidate(schedule)}
                            disabled={isBusy}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Dialog open={deleteCandidate !== null} onOpenChange={(open) => !open && setDeleteCandidate(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Backup Schedule</DialogTitle>
            <DialogDescription>
              {deleteCandidate
                ? `Delete "${deleteCandidate.name}"? This will stop future automated executions for this campaign.`
                : "Delete the selected backup schedule."}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteCandidate(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => void handleDelete()}
              disabled={deleteCandidate ? busyActionId === deleteCandidate.id : false}
            >
              {deleteCandidate && busyActionId === deleteCandidate.id ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <Trash2 className="mr-2 h-4 w-4" />
              )}
              Delete Schedule
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
