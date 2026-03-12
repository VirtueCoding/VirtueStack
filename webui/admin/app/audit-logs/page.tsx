"use client";

import { useState } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Activity,
  Download,
  Filter,
  Calendar,
  Search,
} from "lucide-react";

interface AuditLog {
  id: string;
  timestamp: string;
  actor_type: "admin" | "customer" | "system";
  actor_name: string;
  action: "create" | "update" | "delete" | "read" | "login" | "logout";
  resource_type: string;
  resource_name: string;
  status: "success" | "failure";
  ip_address: string;
}

const mockAuditLogs: AuditLog[] = [
  {
    id: "1",
    timestamp: new Date(Date.now() - 2 * 60 * 1000).toISOString(),
    actor_type: "admin",
    actor_name: "admin@virtuestack.com",
    action: "create",
    resource_type: "vm",
    resource_name: "vm-prod-web-01",
    status: "success",
    ip_address: "192.168.1.100",
  },
  {
    id: "2",
    timestamp: new Date(Date.now() - 15 * 60 * 1000).toISOString(),
    actor_type: "customer",
    actor_name: "Acme Corporation",
    action: "update",
    resource_type: "vm",
    resource_name: "vm-dev-test-03",
    status: "success",
    ip_address: "203.0.113.45",
  },
  {
    id: "3",
    timestamp: new Date(Date.now() - 45 * 60 * 1000).toISOString(),
    actor_type: "system",
    actor_name: "Scheduler",
    action: "delete",
    resource_type: "snapshot",
    resource_name: "snap-db-backup-old",
    status: "success",
    ip_address: "127.0.0.1",
  },
  {
    id: "4",
    timestamp: new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString(),
    actor_type: "customer",
    actor_name: "TechStart Inc",
    action: "login",
    resource_type: "portal",
    resource_name: "Customer Portal",
    status: "failure",
    ip_address: "198.51.100.23",
  },
  {
    id: "5",
    timestamp: new Date(Date.now() - 3 * 60 * 60 * 1000).toISOString(),
    actor_type: "admin",
    actor_name: "ops@virtuestack.com",
    action: "delete",
    resource_type: "vm",
    resource_name: "vm-staging-legacy",
    status: "success",
    ip_address: "192.168.1.105",
  },
  {
    id: "6",
    timestamp: new Date(Date.now() - 5 * 60 * 60 * 1000).toISOString(),
    actor_type: "system",
    actor_name: "Backup Service",
    action: "create",
    resource_type: "snapshot",
    resource_name: "snap-prod-db-daily",
    status: "success",
    ip_address: "127.0.0.1",
  },
  {
    id: "7",
    timestamp: new Date(Date.now() - 8 * 60 * 60 * 1000).toISOString(),
    actor_type: "customer",
    actor_name: "Global Services Ltd",
    action: "read",
    resource_type: "vm",
    resource_name: "vm-analytics-02",
    status: "success",
    ip_address: "203.0.113.89",
  },
  {
    id: "8",
    timestamp: new Date(Date.now() - 12 * 60 * 60 * 1000).toISOString(),
    actor_type: "admin",
    actor_name: "admin@virtuestack.com",
    action: "update",
    resource_type: "node",
    resource_name: "node-hv-03",
    status: "failure",
    ip_address: "192.168.1.100",
  },
  {
    id: "9",
    timestamp: new Date(Date.now() - 18 * 60 * 60 * 1000).toISOString(),
    actor_type: "customer",
    actor_name: "DevShop Studio",
    action: "logout",
    resource_type: "portal",
    resource_name: "Customer Portal",
    status: "success",
    ip_address: "198.51.100.67",
  },
  {
    id: "10",
    timestamp: new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString(),
    actor_type: "system",
    actor_name: "Health Monitor",
    action: "read",
    resource_type: "node",
    resource_name: "node-hv-01",
    status: "success",
    ip_address: "127.0.0.1",
  },
  {
    id: "11",
    timestamp: new Date(Date.now() - 26 * 60 * 60 * 1000).toISOString(),
    actor_type: "admin",
    actor_name: "security@virtuestack.com",
    action: "read",
    resource_type: "audit_log",
    resource_name: "Security Review",
    status: "success",
    ip_address: "192.168.1.110",
  },
  {
    id: "12",
    timestamp: new Date(Date.now() - 30 * 60 * 60 * 1000).toISOString(),
    actor_type: "customer",
    actor_name: "CloudNine Systems",
    action: "create",
    resource_type: "vm",
    resource_name: "vm-ml-training-01",
    status: "success",
    ip_address: "203.0.113.120",
  },
];

function getActorBadge(actorType: AuditLog["actor_type"]) {
  const variants = {
    admin: "default" as const,
    customer: "secondary" as const,
    system: "outline" as const,
  };

  const labels = {
    admin: "Admin",
    customer: "Customer",
    system: "System",
  };

  return <Badge variant={variants[actorType]}>{labels[actorType]}</Badge>;
}

function getActionBadge(action: AuditLog["action"]) {
  const colors = {
    create: "bg-green-500/10 text-green-500 border-green-500/20",
    update: "bg-blue-500/10 text-blue-500 border-blue-500/20",
    delete: "bg-red-500/10 text-red-500 border-red-500/20",
    read: "bg-gray-500/10 text-gray-500 border-gray-500/20",
    login: "bg-purple-500/10 text-purple-500 border-purple-500/20",
    logout: "bg-orange-500/10 text-orange-500 border-orange-500/20",
  };

  return (
    <Badge className={`${colors[action]} border font-medium`}>
      {action.charAt(0).toUpperCase() + action.slice(1)}
    </Badge>
  );
}

function getStatusBadge(status: AuditLog["status"]) {
  const variants = {
    success: "success" as const,
    failure: "destructive" as const,
  };

  const labels = {
    success: "Success",
    failure: "Failed",
  };

  return <Badge variant={variants[status]}>{labels[status]}</Badge>;
}

function formatRelativeTime(dateString: string): string {
  const date = new Date(dateString);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMinutes = Math.floor(diffMs / (1000 * 60));
  const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

  if (diffMinutes < 1) {
    return "Just now";
  } else if (diffMinutes < 60) {
    return `${diffMinutes} minute${diffMinutes > 1 ? "s" : ""} ago`;
  } else if (diffHours < 24) {
    return `${diffHours} hour${diffHours > 1 ? "s" : ""} ago`;
  } else {
    return `${diffDays} day${diffDays > 1 ? "s" : ""} ago`;
  }
}

function formatDateTime(dateString: string): string {
  return new Date(dateString).toLocaleString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export default function AuditLogsPage() {
  const [auditLogs] = useState<AuditLog[]>(mockAuditLogs);
  const [filterAction, setFilterAction] = useState<string>("all");
  const [filterStatus, setFilterStatus] = useState<string>("all");
  const [filterActor, setFilterActor] = useState<string>("all");
  const [searchTerm, setSearchTerm] = useState("");

  const filteredLogs = auditLogs.filter((log) => {
    const matchesAction = filterAction === "all" || log.action === filterAction;
    const matchesStatus = filterStatus === "all" || log.status === filterStatus;
    const matchesActor = filterActor === "all" || log.actor_type === filterActor;
    const matchesSearch =
      searchTerm === "" ||
      log.actor_name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      log.resource_name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      log.ip_address.includes(searchTerm);

    return matchesAction && matchesStatus && matchesActor && matchesSearch;
  });

  const handleExport = () => {
    if (filteredLogs.length === 0) {
      return;
    }

    const headers = [
      "Timestamp",
      "Actor Type",
      "Actor Name",
      "Action",
      "Resource Name",
      "Resource Type",
      "Status",
      "IP Address",
    ];

    const escapeCsv = (value: string): string => {
      const str = String(value);
      if (str.includes('"') || str.includes(',') || str.includes('\n') || str.includes('\r')) {
        return `"${str.replace(/"/g, '""')}"`;
      }
      return str;
    };

    const chunks: string[] = [];
    chunks.push(headers.join(","));

    const batchSize = 1000;
    for (let i = 0; i < filteredLogs.length; i += batchSize) {
      const batch = filteredLogs.slice(i, i + batchSize);
      const rows = batch.map((log) => {
        const row = [
          escapeCsv(log.timestamp),
          escapeCsv(log.actor_type),
          escapeCsv(log.actor_name),
          escapeCsv(log.action),
          escapeCsv(log.resource_name),
          escapeCsv(log.resource_type),
          escapeCsv(log.status),
          escapeCsv(log.ip_address),
        ];
        return row.join(",");
      });
      chunks.push(rows.join("\n"));
    }

    const csvContent = chunks.join("\n");

    const now = new Date();
    const dateStr = now.toISOString().split("T")[0];
    const timeStr = now.toTimeString().split(" ")[0].replace(/:/g, "-");

    const filterParts: string[] = [];
    if (filterAction !== "all") filterParts.push(filterAction);
    if (filterStatus !== "all") filterParts.push(filterStatus);
    if (filterActor !== "all") filterParts.push(filterActor);
    if (searchTerm) filterParts.push("search");
    const filterSuffix = filterParts.length > 0 ? `_${filterParts.join("-")}` : "";

    const filename = `audit-logs_${dateStr}_${timeStr}${filterSuffix}.csv`;

    const blob = new Blob([csvContent], { type: "text/csv;charset=utf-8;" });
    const link = document.createElement("a");
    const url = URL.createObjectURL(blob);

    link.setAttribute("href", url);
    link.setAttribute("download", filename);
    link.style.visibility = "hidden";
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);

    setTimeout(() => URL.revokeObjectURL(url), 0);
  };

  const actions = ["all", "create", "update", "delete", "read", "login", "logout"];
  const statuses = ["all", "success", "failure"];
  const actors = ["all", "admin", "customer", "system"];

  return (
    <div className="min-h-screen bg-background p-6 md:p-8">
      <div className="mx-auto max-w-7xl space-y-6">
        {/* Header */}
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">Audit Logs</h1>
            <p className="text-muted-foreground">
              System activity and security events
            </p>
          </div>
          <div className="flex items-center gap-2">
            <div className="flex items-center gap-2 rounded-full bg-green-500/10 px-3 py-1">
              <div className="h-2 w-2 animate-pulse rounded-full bg-green-500" />
              <span className="text-xs font-medium text-green-500">Live</span>
            </div>
            <Button variant="outline" size="default" onClick={handleExport}>
              <Download className="mr-2 h-4 w-4" />
              Export CSV
            </Button>
          </div>
        </div>

        {/* Filters */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <Filter className="h-4 w-4" />
              Filters
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-5">
              <div className="lg:col-span-2">
                <div className="relative">
                  <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    placeholder="Search actor, resource, IP..."
                    value={searchTerm}
                    onChange={(e) => setSearchTerm(e.target.value)}
                    className="pl-10"
                  />
                </div>
              </div>
              <div>
                <select
                  value={filterAction}
                  onChange={(e) => setFilterAction(e.target.value)}
                  className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                >
                  {actions.map((action) => (
                    <option key={action} value={action}>
                      {action === "all" ? "All Actions" : action.charAt(0).toUpperCase() + action.slice(1)}
                    </option>
                  ))}
                </select>
              </div>
              <div>
                <select
                  value={filterStatus}
                  onChange={(e) => setFilterStatus(e.target.value)}
                  className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                >
                  {statuses.map((status) => (
                    <option key={status} value={status}>
                      {status === "all" ? "All Status" : status.charAt(0).toUpperCase() + status.slice(1)}
                    </option>
                  ))}
                </select>
              </div>
              <div>
                <select
                  value={filterActor}
                  onChange={(e) => setFilterActor(e.target.value)}
                  className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                >
                  {actors.map((actor) => (
                    <option key={actor} value={actor}>
                      {actor === "all" ? "All Actors" : actor.charAt(0).toUpperCase() + actor.slice(1)}
                    </option>
                  ))}
                </select>
              </div>
            </div>
          </CardContent>
        </Card>

        {/* Audit Logs Table */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Activity className="h-5 w-5" />
              System Activity
            </CardTitle>
            <CardDescription>
              {filteredLogs.length} of {auditLogs.length} events displayed
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Timestamp</TableHead>
                    <TableHead>Actor</TableHead>
                    <TableHead>Action</TableHead>
                    <TableHead>Resource</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>IP Address</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredLogs.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={6} className="h-24 text-center">
                        No audit logs found
                      </TableCell>
                    </TableRow>
                  ) : (
                    filteredLogs.map((log) => (
                      <TableRow key={log.id}>
                        <TableCell>
                          <div className="space-y-1">
                            <div className="text-sm font-medium">
                              {formatRelativeTime(log.timestamp)}
                            </div>
                            <div className="text-xs text-muted-foreground">
                              {formatDateTime(log.timestamp)}
                            </div>
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="space-y-1">
                            {getActorBadge(log.actor_type)}
                            <div className="text-xs text-muted-foreground">
                              {log.actor_name}
                            </div>
                          </div>
                        </TableCell>
                        <TableCell>{getActionBadge(log.action)}</TableCell>
                        <TableCell>
                          <div className="space-y-1">
                            <div className="text-sm font-medium">
                              {log.resource_name}
                            </div>
                            <div className="text-xs text-muted-foreground">
                              {log.resource_type}
                            </div>
                          </div>
                        </TableCell>
                        <TableCell>{getStatusBadge(log.status)}</TableCell>
                        <TableCell className="font-mono text-xs">
                          {log.ip_address}
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>
          </CardContent>
        </Card>

        {/* Pagination */}
        <Card>
          <CardContent className="flex items-center justify-between py-4">
            <p className="text-sm text-muted-foreground">
              Showing <span className="font-medium">{filteredLogs.length}</span> of{" "}
              <span className="font-medium">{auditLogs.length}{" "}</span>
              audit logs
            </p>
            <div className="flex items-center gap-2">
              <Button variant="outline" size="sm" disabled>
                Previous
              </Button>
              <Button variant="outline" size="sm" disabled>
                Next
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
