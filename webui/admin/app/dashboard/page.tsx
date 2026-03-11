"use client";

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
  AlertCircle,
  Plus,
  FileSpreadsheet,
  HardDrive,
  Activity,
} from "lucide-react";

interface DashboardStats {
  totalVMs: number;
  totalNodes: number;
  totalCustomers: number;
  activeAlerts: number;
}

interface ActivityItem {
  id: string;
  action: string;
  resource: string;
  timestamp: string;
  type: "info" | "warning" | "success" | "error";
}

const mockStats: DashboardStats = {
  totalVMs: 247,
  totalNodes: 12,
  totalCustomers: 89,
  activeAlerts: 3,
};

const mockActivities: ActivityItem[] = [
  {
    id: "1",
    action: "VM created",
    resource: "vm-prod-web-01",
    timestamp: "2 minutes ago",
    type: "success",
  },
  {
    id: "2",
    action: "Node health check failed",
    resource: "node-hv-03",
    timestamp: "15 minutes ago",
    type: "error",
  },
  {
    id: "3",
    action: "Customer registered",
    resource: "Acme Corp",
    timestamp: "1 hour ago",
    type: "info",
  },
  {
    id: "4",
    action: "Backup completed",
    resource: "vm-db-primary",
    timestamp: "2 hours ago",
    type: "success",
  },
  {
    id: "5",
    action: "High CPU usage detected",
    resource: "vm-analytics-02",
    timestamp: "3 hours ago",
    type: "warning",
  },
  {
    id: "6",
    action: "VM stopped",
    resource: "vm-dev-test-05",
    timestamp: "5 hours ago",
    type: "info",
  },
];

const statCards = [
  {
    title: "Total VMs",
    value: mockStats.totalVMs.toString(),
    icon: Server,
    description: "Virtual machines running",
  },
  {
    title: "Total Nodes",
    value: mockStats.totalNodes.toString(),
    icon: HardDrive,
    description: "Hypervisor nodes",
  },
  {
    title: "Total Customers",
    value: mockStats.totalCustomers.toString(),
    icon: Users,
    description: "Active accounts",
  },
  {
    title: "Active Alerts",
    value: mockStats.activeAlerts.toString(),
    icon: AlertCircle,
    description: "Requires attention",
  },
];

export default function DashboardPage() {
  return (
    <div className="min-h-screen bg-background p-6 md:p-8">
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
            <Button variant="outline" size="default">
              <Activity className="mr-2 h-4 w-4" />
              View Logs
            </Button>
            <Button size="default">
              <Plus className="mr-2 h-4 w-4" />
              Quick Action
            </Button>
          </div>
        </div>

        {/* Stats Grid */}
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
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

        {/* Main Content Grid */}
        <div className="grid gap-6 lg:grid-cols-2">
          {/* Recent Activity */}
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
              <div className="space-y-4">
                {mockActivities.map((activity) => (
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
            </CardContent>
          </Card>

          {/* Quick Actions */}
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
                <Button variant="outline" className="justify-start">
                  <Plus className="mr-2 h-4 w-4" />
                  Add New Node
                </Button>
                <Button variant="outline" className="justify-start">
                  <FileSpreadsheet className="mr-2 h-4 w-4" />
                  Create VM Plan
                </Button>
                <Button variant="outline" className="justify-start">
                  <Server className="mr-2 h-4 w-4" />
                  Provision VM
                </Button>
                <Button variant="outline" className="justify-start">
                  <Users className="mr-2 h-4 w-4" />
                  Add Customer
                </Button>
                <Button variant="outline" className="justify-start">
                  <AlertCircle className="mr-2 h-4 w-4" />
                  View All Alerts
                </Button>
                <Button variant="outline" className="justify-start">
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
