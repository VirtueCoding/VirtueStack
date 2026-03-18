"use client";

import { useState, useEffect, useCallback } from "react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import {
  Server,
  Users,
  Monitor,
  Plus,
  FileSpreadsheet,
  HardDrive,
  Activity,
  Loader2,
  AlertTriangle,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { adminVMsApi, adminNodesApi, adminCustomersApi, adminAuditLogsApi } from "@/lib/api-client";

interface DashboardStats {
  totalVMs: number;
  totalNodes: number;
  totalCustomers: number;
}

interface ActivityItem {
  id: string;
  action: string;
  resource: string;
  timestamp: string;
  type: "info" | "warning" | "success" | "error";
}

export default function DashboardPage() {
  const router = useRouter();
  const [stats, setStats] = useState<DashboardStats>({
    totalVMs: 0,
    totalNodes: 0,
    totalCustomers: 0,
  });
  const [activities, setActivities] = useState<ActivityItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadData = useCallback(async function loadData() {
    try {
      const results = await Promise.allSettled([
        adminVMsApi.getVMs(),
        adminNodesApi.getNodes(),
        adminCustomersApi.getCustomers(),
        adminAuditLogsApi.getAuditLogs(),
      ]);

      const vms = results[0].status === 'fulfilled' ? results[0].value : [];
      const nodes = results[1].status === 'fulfilled' ? results[1].value : [];
      const customers = results[2].status === 'fulfilled' ? results[2].value : [];
      const logsResult = results[3].status === 'fulfilled' ? results[3].value : { logs: [], total: 0 };
      const logs = logsResult.logs || [];

      setStats({
        totalVMs: vms.length,
        totalNodes: nodes.length,
        totalCustomers: customers.length,
      });

      const mappedActivities: ActivityItem[] = logs.slice(0, 6).map((log) => {
        let type: "info" | "warning" | "success" | "error" = "info";
        if (!log.success) type = "error";
        else if (log.action.includes("create") || log.action.includes("start")) type = "success";
        else if (log.action.includes("delete") || log.action.includes("stop")) type = "warning";

        return {
          id: log.id,
          action: log.action,
          resource: log.resource_id || log.resource_type,
          timestamp: new Date(log.timestamp).toLocaleString(),
          type,
        };
      });
      setActivities(mappedActivities);

      const failedCount = results.filter(r => r.status === 'rejected').length;
      if (failedCount > 0) {
        setError(`Failed to load ${failedCount} dashboard data source(s)`);
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadData();
  }, [loadData]);

  useEffect(() => {
    const interval = setInterval(() => {
      loadData();
    }, 60000);
    return () => clearInterval(interval);
  }, [loadData]);

  const statCards = [
    {
      title: "Total VMs",
      value: stats.totalVMs.toString(),
      icon: Server,
      description: "Virtual machines running",
    },
    {
      title: "Total Nodes",
      value: stats.totalNodes.toString(),
      icon: HardDrive,
      description: "Hypervisor nodes",
    },
    {
      title: "Total Customers",
      value: stats.totalCustomers.toString(),
      icon: Users,
      description: "Active accounts",
    },
  ];

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-background p-6 md:p-8">
      {error && (
        <div className="mx-auto max-w-7xl mb-6 flex items-center gap-2 rounded-md border border-yellow-500/50 bg-yellow-500/10 p-3 text-sm text-yellow-700 dark:text-yellow-400">
          <AlertTriangle className="h-4 w-4 shrink-0" />
          {error}
        </div>
      )}
      <div className="mx-auto max-w-7xl space-y-8">
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">Dashboard</h1>
            <p className="text-muted-foreground">
              System overview and recent activity
            </p>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" size="default" onClick={() => router.push("/audit-logs")}>
              <Activity className="mr-2 h-4 w-4" />
              View Logs
            </Button>
          </div>
        </div>

        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {statCards.map((stat) => (
            <Card key={stat.title} className="relative overflow-hidden">
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm font-medium">
                  {stat.title}
                </CardTitle>
                <stat.icon className="h-4 w-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">{stat.value}</div>
                <p className="text-xs text-muted-foreground">
                  {stat.description}
                </p>
              </CardContent>
            </Card>
          ))}
        </div>

        <div className="grid gap-6 lg:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Activity className="h-5 w-5" />
                Recent Activity
              </CardTitle>
              <CardDescription>
                Latest events from across the system
              </CardDescription>
            </CardHeader>
            <CardContent>
              {activities.length === 0 ? (
                <div className="text-sm text-muted-foreground py-4">No recent activity found.</div>
              ) : (
                <div className="space-y-4">
                  {activities.map((activity) => (
                    <div
                      key={activity.id}
                    className="flex items-start justify-between gap-4 border-b border-border last:border-0 pb-4 last:pb-0"
                  >
                    <div className="flex items-start gap-3">
                      <div
                        className={`mt-0.5 h-2 w-2 rounded-full ${
                          activity.type === "success"
                            ? "bg-green-500"
                            : activity.type === "error"
                              ? "bg-red-500"
                              : activity.type === "warning"
                                ? "bg-yellow-500"
                                : "bg-blue-500"
                        }`}
                      />
                      <div>
                        <p className="text-sm font-medium">
                          {activity.action}
                        </p>
                        <p className="text-xs text-muted-foreground">
                          {activity.resource}
                        </p>
                      </div>
                    </div>
                    <span className="text-xs text-muted-foreground whitespace-nowrap">
                      {activity.timestamp}
                    </span>
                  </div>
                ))}
              </div>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Monitor className="h-5 w-5" />
                Quick Actions
              </CardTitle>
              <CardDescription>
                Common administrative tasks
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="grid gap-3">
                <Button variant="outline" className="justify-start cursor-pointer" onClick={() => router.push("/nodes")}>
                  <Plus className="mr-2 h-4 w-4" />
                  Add New Node
                </Button>
                <Button variant="outline" className="justify-start cursor-pointer" onClick={() => router.push("/plans")}>
                  <FileSpreadsheet className="mr-2 h-4 w-4" />
                  Create VM Plan
                </Button>
                <Button variant="outline" className="justify-start cursor-pointer" onClick={() => router.push("/vms")}>
                  <Server className="mr-2 h-4 w-4" />
                  Provision VM
                </Button>
                <Button variant="outline" className="justify-start cursor-pointer" onClick={() => router.push("/customers")}>
                  <Users className="mr-2 h-4 w-4" />
                  Add Customer
                </Button>
                <Button variant="outline" className="justify-start cursor-pointer" onClick={() => router.push("/ip-sets")}>
                  <HardDrive className="mr-2 h-4 w-4" />
                  Manage IP Pools
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
