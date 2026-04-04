"use client";

import { useState, useEffect, useCallback, useRef } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@virtuestack/ui";
import { Button } from "@virtuestack/ui";
import { Badge } from "@virtuestack/ui";
import { Input } from "@virtuestack/ui";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@virtuestack/ui";
import {
  Activity,
  Download,
  Search,
  Loader2,
  ChevronLeft,
  ChevronRight,
} from "lucide-react";
import { adminAuditLogsApi, type AuditLog } from "@/lib/api-client";
import { useToast } from "@virtuestack/ui";

const PAGE_SIZE = 20;

export default function AuditLogsPage() {
  const [searchTerm, setSearchTerm] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [nextCursor, setNextCursor] = useState<string | undefined>(undefined);
  const [cursorStack, setCursorStack] = useState<string[]>([]);
  const [hasMore, setHasMore] = useState(false);
  const [currentCursor, setCurrentCursor] = useState<string | undefined>(undefined);
  const [loading, setLoading] = useState(true);
  const { toast } = useToast();
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const loadLogs = useCallback(async (cursor?: string, search?: string) => {
    setLoading(true);
    try {
      const data = await adminAuditLogsApi.getAuditLogs(PAGE_SIZE, search || undefined, cursor);
      setLogs(data.data || []);
      setNextCursor(data.meta?.next_cursor ?? undefined);
      setHasMore(data.meta?.has_more ?? false);
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

  const handleSearchChange = (value: string) => {
    setSearchTerm(value);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      setDebouncedSearch(value);
      setCurrentCursor(undefined);
      setCursorStack([]);
    }, 400);
  };

  useEffect(() => {
    loadLogs(currentCursor, debouncedSearch);
  }, [currentCursor, debouncedSearch, loadLogs]);

  const filteredLogs = logs;

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
    try {
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
    } catch {
      toast({ title: "Export Failed", description: "Failed to generate CSV export.", variant: "destructive" });
    }
  };

  const handleNextPage = () => {
    if (nextCursor) {
      setCursorStack((prev) => [...prev, currentCursor ?? ""]);
      setCurrentCursor(nextCursor);
    }
  };

  const handlePrevPage = () => {
    setCursorStack((prev) => {
      const stack = [...prev];
      const prevCursor = stack.pop();
      setCurrentCursor(prevCursor === "" ? undefined : prevCursor);
      return stack;
    });
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
              Detailed record of all administrative and customer actions
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
                  onChange={(e) => handleSearchChange(e.target.value)}
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

            {(cursorStack.length > 0 || hasMore) && (
              <div className="mt-4 flex items-center justify-between">
                <p className="text-sm text-muted-foreground">
                  Showing {filteredLogs.length} items
                </p>
                <div className="flex gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handlePrevPage}
                    disabled={cursorStack.length === 0 || loading}
                  >
                    <ChevronLeft className="mr-1 h-4 w-4" />
                    Previous
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleNextPage}
                    disabled={!hasMore || loading}
                  >
                    Next
                    <ChevronRight className="ml-1 h-4 w-4" />
                  </Button>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
