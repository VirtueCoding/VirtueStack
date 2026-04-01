"use client";

import { useState, useEffect, useCallback } from "react";
import { motion } from "motion/react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Button,
} from "@virtuestack/ui";
import {
  Server,
  Users,
  Monitor,
  Plus,
  FileSpreadsheet,
  HardDrive,
  Activity,
  AlertTriangle,
  Network,
} from "lucide-react";
import { useRouter } from "next/navigation";
import {
  adminVMsApi,
  adminNodesApi,
  adminCustomersApi,
  adminAuditLogsApi,
  type AuditLog,
} from "@/lib/api-client";
import { PageTransition } from "@/components/page-transition";
import { AnimatedCard } from "@/components/animated-card";
import { Skeleton } from "@/components/skeleton";
import {
  fadeUp,
  staggerContainer,
} from "@/lib/animations";

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

function formatRelativeTime(dateStr: string): string {
  const now = Date.now();
  const then = new Date(dateStr).getTime();
  const diff = Math.floor((now - then) / 1000);
  if (diff < 60) return "just now";
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

const statCardConfig = [
  {
    key: "totalVMs",
    title: "Virtual Machines",
    icon: Monitor,
    description: "Total VMs",
    color: "text-blue-500",
    bg: "bg-blue-500/10",
  },
  {
    key: "totalNodes",
    title: "Nodes",
    icon: HardDrive,
    description: "Hypervisor nodes",
    color: "text-emerald-500",
    bg: "bg-emerald-500/10",
  },
  {
    key: "totalCustomers",
    title: "Customers",
    icon: Users,
    description: "Active accounts",
    color: "text-violet-500",
    bg: "bg-violet-500/10",
  },
] as const;

function DashboardSkeleton() {
  return (
    <div className="mx-auto max-w-7xl space-y-8">
      <div>
        <Skeleton className="h-9 w-48" />
        <Skeleton className="mt-2 h-5 w-72" />
      </div>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {[1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-32 rounded-xl" />
        ))}
      </div>
      <div className="grid gap-6 lg:grid-cols-2">
        <Skeleton className="h-80 rounded-xl" />
        <Skeleton className="h-80 rounded-xl" />
      </div>
    </div>
  );
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

      const vmsResult =
        results[0].status === "fulfilled" ? results[0].value : { data: [] };
      const nodesResult =
        results[1].status === "fulfilled" ? results[1].value : { data: [] };
      const customersResult =
        results[2].status === "fulfilled" ? results[2].value : { data: [] };
      const logsResult =
        results[3].status === "fulfilled" ? results[3].value : { data: [] };
      const logs = logsResult.data || [];

      setStats({
        totalVMs: (vmsResult.data || []).length,
        totalNodes: (nodesResult.data || []).length,
        totalCustomers: (customersResult.data || []).length,
      });

      const mappedActivities: ActivityItem[] = (logs as AuditLog[])
        .slice(0, 8)
        .map((log) => {
          let type: "info" | "warning" | "success" | "error" = "info";
          if (!log.success) type = "error";
          else if (log.action.includes("create") || log.action.includes("start"))
            type = "success";
          else if (log.action.includes("delete") || log.action.includes("stop"))
            type = "warning";

          return {
            id: log.id,
            action: log.action,
            resource: log.resource_id || log.resource_type,
            timestamp: log.timestamp,
            type,
          };
        });
      setActivities(mappedActivities);

      const failedCount = results.filter((r) => r.status === "rejected").length;
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

  if (loading) {
    return (
      <div className="min-h-screen p-6 md:p-8">
        <DashboardSkeleton />
      </div>
    );
  }

  return (
    <PageTransition>
      <div className="min-h-screen p-6 md:p-8">
        {error && (
          <div className="mx-auto max-w-7xl mb-6 flex items-center gap-2 rounded-lg border border-yellow-500/30 bg-yellow-500/10 p-3 text-sm text-yellow-700 dark:text-yellow-400">
            <AlertTriangle className="h-4 w-4 shrink-0" />
            {error}
          </div>
        )}

        <div className="mx-auto max-w-7xl space-y-8">
          {/* Header */}
          <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
            <div>
              <h1 className="text-3xl font-bold tracking-tight">Dashboard</h1>
              <p className="text-muted-foreground">
                System overview and recent activity
              </p>
            </div>
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="default"
                onClick={() => router.push("/audit-logs")}
              >
                <Activity className="mr-2 h-4 w-4" />
                View Logs
              </Button>
            </div>
          </div>

          {/* Stat Cards */}
          <motion.div
            variants={staggerContainer}
            initial="hidden"
            animate="visible"
            className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3"
          >
            {statCardConfig.map((stat, i) => {
              const Icon = stat.icon;
              const value = stats[stat.key as keyof DashboardStats];
              return (
                <motion.div key={stat.key} variants={fadeUp}>
                  <AnimatedCard delay={i * 0.05}>
                    <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                      <CardTitle className="text-sm font-medium">
                        {stat.title}
                      </CardTitle>
                      <div className={`rounded-lg p-2 ${stat.bg}`}>
                        <Icon className={`h-4 w-4 ${stat.color}`} />
                      </div>
                    </CardHeader>
                    <CardContent>
                      <div className="text-3xl font-bold tabular-nums">
                        {value}
                      </div>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {stat.description}
                      </p>
                    </CardContent>
                  </AnimatedCard>
                </motion.div>
              );
            })}
          </motion.div>

          {/* Activity + Quick Actions */}
          <div className="grid gap-6 lg:grid-cols-2">
            {/* Activity Feed */}
            <AnimatedCard hoverLift={false} delay={0.2}>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Activity className="h-5 w-5" />
                  Recent Activity
                </CardTitle>
                <CardDescription>Latest events across the system</CardDescription>
              </CardHeader>
              <CardContent>
                {activities.length === 0 ? (
                  <div className="py-8 text-center text-sm text-muted-foreground">
                    No recent activity found.
                  </div>
                ) : (
                  <motion.div
                    variants={staggerContainer}
                    initial="hidden"
                    animate="visible"
                    className="space-y-1"
                  >
                    {activities.map((activity) => (
                      <motion.div
                        key={activity.id}
                        variants={fadeUp}
                        className="flex items-start justify-between gap-4 rounded-lg px-2 py-2.5 transition-colors hover:bg-muted/50"
                      >
                        <div className="flex items-start gap-3">
                          <div
                            className={`mt-1.5 h-2 w-2 shrink-0 rounded-full ${
                              activity.type === "success"
                                ? "bg-emerald-500"
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
                        <span className="shrink-0 text-xs text-muted-foreground">
                          {formatRelativeTime(activity.timestamp)}
                        </span>
                      </motion.div>
                    ))}
                  </motion.div>
                )}
              </CardContent>
            </AnimatedCard>

            {/* Quick Actions */}
            <AnimatedCard hoverLift={false} delay={0.25}>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Monitor className="h-5 w-5" />
                  Quick Actions
                </CardTitle>
                <CardDescription>Common administrative tasks</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="grid gap-2 sm:grid-cols-2">
                  {[
                    {
                      icon: Plus,
                      label: "Add Node",
                      href: "/nodes",
                      color: "text-emerald-500",
                      bg: "bg-emerald-500/10",
                    },
                    {
                      icon: FileSpreadsheet,
                      label: "Create Plan",
                      href: "/plans",
                      color: "text-blue-500",
                      bg: "bg-blue-500/10",
                    },
                    {
                      icon: Server,
                      label: "Provision VM",
                      href: "/vms",
                      color: "text-violet-500",
                      bg: "bg-violet-500/10",
                    },
                    {
                      icon: Users,
                      label: "Add Customer",
                      href: "/customers",
                      color: "text-amber-500",
                      bg: "bg-amber-500/10",
                    },
                    {
                      icon: Network,
                      label: "Manage IPs",
                      href: "/ip-sets",
                      color: "text-cyan-500",
                      bg: "bg-cyan-500/10",
                    },
                    {
                      icon: HardDrive,
                      label: "Storage",
                      href: "/storage-backends",
                      color: "text-rose-500",
                      bg: "bg-rose-500/10",
                    },
                  ].map((action) => {
                    const Icon = action.icon;
                    return (
                      <Button
                        key={action.label}
                        variant="outline"
                        className="h-auto justify-start gap-3 px-4 py-3 transition-all duration-150 hover:shadow-sm"
                        onClick={() => router.push(action.href)}
                      >
                        <div className={`rounded-md p-1.5 ${action.bg}`}>
                          <Icon className={`h-4 w-4 ${action.color}`} />
                        </div>
                        <span className="text-sm font-medium">
                          {action.label}
                        </span>
                      </Button>
                    );
                  })}
                </div>
              </CardContent>
            </AnimatedCard>
          </div>
        </div>
      </div>
    </PageTransition>
  );
}
