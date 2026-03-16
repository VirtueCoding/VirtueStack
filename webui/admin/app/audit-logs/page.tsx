"use client";

import { useState, useEffect, useCallback } from "react";
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
  Search,
  Loader2,
  ChevronLeft,
  ChevronRight,
} from "lucide-react";
import { adminAuditLogsApi, type AuditLog } from "@/lib/api-client";
import { useToast } from "@/components/ui/use-toast";

const PAGE_SIZE = 20;

export default function AuditLogsPage() {
  const [searchTerm, setSearchTerm] = useState("");
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const { toast } = useToast();

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  const loadLogs = useCallback(async (currentPage: number) => {
    setLoading(true);
    try {
      const data = await adminAuditLogsApi.getAuditLogs(currentPage, PAGE_SIZE);
      setLogs(data.logs || []);
      setTotal(data.total || 0);
    } catch {
      toast({
        title: "Error",
        description: "Failed to load audit logs.",
        variant: "destructive",
      });
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    loadLogs(page);
  }, [page, loadLogs]);

  const filteredLogs = searchTerm.trim()
    ? logs.filter(
        (log) =>
          log.action.toLowerCase().includes(searchTerm.toLowerCase()) ||
          log.resource_type.toLowerCase().includes(searchTerm.toLowerCase()) ||
          (log.resource_id && log.resource_id.toLowerCase().includes(searchTerm.toLowerCase())) ||
          (log.actor_id && log.actor_id.toLowerCase().includes(searchTerm.toLowerCase())) ||
          log.actor_type.toLowerCase().includes(searchTerm.toLowerCase())
      )
    : logs;

  function getActionBadgeVariant(action: string): "success" | "destructive" | "warning" | "secondary" {
    if (action.includes("create") || action.includes("start")) return "success";
    if (action.includes("delete") || action.includes("stop")) return "destructive";
    if (action.includes("update") || action.includes("modify")) return "warning";
    return "secondary";
  }

  const handleExportCSV = () => {
    if (filteredLogs.length === 0) {
      toast({ title: "No Data", description: "No logs to export.", variant: "destructive" });
      return;
    }
    const headers = ["Timestamp", "Actor Type", "Actor ID", "Action", "Resource Type", "Resource ID", "Success", "IP Address"];
    const rows = filteredLogs.map((log) => [
      log.timestamp,
      log.actor_type,
      log.actor_id || "",
      log.action,
      log.resource_type,
      log.resource_id || "",
      log.success ? "true" : "false",
      log.actor_ip || "",
    ]);
    const csv = [headers, ...rows].map((row) => row.map((cell) => `"${String(cell).replace(/"/g, '""')}"`).join(",")).join("\n");
    const blob = new Blob([csv], { type: "text/csv;charset=utf-8;" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `audit-logs-${new Date().toISOString().split("T")[0]}.csv`;
    a.click();
    URL.revokeObjectURL(url);
    toast({ title: "Export Complete", description: `Exported ${filteredLogs.length} log entries.` });
  };

  return (
    <div className="min-h-screen bg-background p-6 md:p-8">
      <div className="mx-auto max-w-7xl space-y-8">
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">Audit Logs</h1>
            <p className="text-muted-foreground">
              Track and monitor system-wide activities
            </p>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" onClick={handleExportCSV}>
              <Download className="mr-2 h-4 w-4" />
              Export CSV
            </Button>
          </div>
        </div>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Activity className="h-5 w-5" />
              System Activity
            </CardTitle>
            <CardDescription>
              Detailed record of all administrative and customer actions ({total} total entries)
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
              <div className="relative flex-1 max-w-sm">
                <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
                <Input
                  placeholder="Search logs..."
                  className="pl-8"
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                />
              </div>
            </div>

            <div className="rounded-md border">
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
                  {loading ? (
                    <TableRow>
                      <TableCell colSpan={6} className="h-24 text-center">
                        <div className="flex justify-center">
                          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                        </div>
                      </TableCell>
                    </TableRow>
                  ) : filteredLogs.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={6} className="h-24 text-center">
                        No audit logs found.
                      </TableCell>
                    </TableRow>
                  ) : (
                    filteredLogs.map((log) => (
                      <TableRow key={log.id}>
                        <TableCell className="whitespace-nowrap text-sm text-muted-foreground">
                          {new Date(log.timestamp).toLocaleString()}
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            <Badge variant="outline" className="capitalize">
                              {log.actor_type}
                            </Badge>
                            <span className="text-sm font-medium">
                              {log.actor_id || "System"}
                            </span>
                          </div>
                        </TableCell>
                        <TableCell>
                          <Badge variant={getActionBadgeVariant(log.action)} className="capitalize">
                            {log.action.replace(/\./g, " ")}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          <div className="flex flex-col">
                            <span className="text-xs font-semibold uppercase text-muted-foreground">
                              {log.resource_type}
                            </span>
                            <span className="text-sm">
                              {log.resource_id || "N/A"}
                            </span>
                          </div>
                        </TableCell>
                        <TableCell>
                          {log.success ? (
                            <Badge variant="success">Success</Badge>
                          ) : (
                            <Badge variant="destructive">Failed</Badge>
                          )}
                        </TableCell>
                        <TableCell className="text-sm text-muted-foreground">
                          {log.actor_ip || "N/A"}
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>

            <div className="mt-4 flex items-center justify-between">
              <p className="text-sm text-muted-foreground">
                Page {page} of {totalPages}
              </p>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                  disabled={page <= 1 || loading}
                >
                  <ChevronLeft className="mr-1 h-4 w-4" />
                  Previous
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                  disabled={page >= totalPages || loading}
                >
                  Next
                  <ChevronRight className="ml-1 h-4 w-4" />
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
