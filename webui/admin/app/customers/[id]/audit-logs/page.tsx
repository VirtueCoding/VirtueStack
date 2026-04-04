"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  useToast,
} from "@virtuestack/ui";
import {
  ArrowLeft,
  ChevronLeft,
  ChevronRight,
  History,
  Loader2,
} from "lucide-react";

import { adminCustomersApi, type AuditLog, type CustomerDetail } from "@/lib/api-client";

const PAGE_SIZE = 20;

function getActionBadgeVariant(action: string): "success" | "destructive" | "warning" | "secondary" {
  if (action.includes("create") || action.includes("start")) return "success";
  if (action.includes("delete") || action.includes("stop")) return "destructive";
  if (action.includes("update") || action.includes("modify")) return "warning";
  return "secondary";
}

export default function CustomerAuditLogsPage() {
  const params = useParams<{ id: string }>();
  const customerId = typeof params.id === "string" ? params.id : "";
  const [customer, setCustomer] = useState<CustomerDetail | null>(null);
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [currentCursor, setCurrentCursor] = useState<string | undefined>(undefined);
  const [cursorStack, setCursorStack] = useState<string[]>([]);
  const [nextCursor, setNextCursor] = useState<string | undefined>(undefined);
  const [hasMore, setHasMore] = useState(false);
  const { toast } = useToast();

  const loadData = useCallback(async () => {
    if (!customerId) return;
    setLoading(true);
    try {
      const [customerResponse, logResponse] = await Promise.all([
        adminCustomersApi.getCustomer(customerId),
        adminCustomersApi.getCustomerAuditLogs(customerId, PAGE_SIZE, currentCursor),
      ]);
      setCustomer(customerResponse);
      setLogs(logResponse.data || []);
      setNextCursor(logResponse.meta?.next_cursor ?? undefined);
      setHasMore(logResponse.meta?.has_more ?? false);
    } catch {
      toast({
        title: "Error",
        description: "Failed to load customer audit logs.",
        variant: "destructive",
      });
    } finally {
      setLoading(false);
    }
  }, [currentCursor, customerId, toast]);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  const handleNextPage = () => {
    if (!nextCursor) return;
    setCursorStack((current) => [...current, currentCursor ?? ""]);
    setCurrentCursor(nextCursor);
  };

  const handlePrevPage = () => {
    setCursorStack((current) => {
      const stack = [...current];
      const previous = stack.pop();
      setCurrentCursor(previous === "" ? undefined : previous);
      return stack;
    });
  };

  return (
    <div className="min-h-screen bg-background p-6 md:p-8">
      <div className="mx-auto max-w-7xl space-y-8">
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div>
            <Button asChild variant="ghost" className="mb-2 -ml-4 w-fit">
              <Link href={`/customers/${customerId}`}>
                <ArrowLeft className="mr-2 h-4 w-4" />
                Customer details
              </Link>
            </Button>
            <h1 className="text-3xl font-bold tracking-tight">Customer audit logs</h1>
            <p className="text-muted-foreground">
              {customer ? `Recent audit events associated with ${customer.name}.` : "Loading customer context..."}
            </p>
          </div>
        </div>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <History className="h-5 w-5" />
              Audit trail
            </CardTitle>
            <CardDescription>
              Uses the dedicated admin customer audit endpoint to focus on this account&apos;s activity.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Timestamp</TableHead>
                    <TableHead>Actor</TableHead>
                    <TableHead>Action</TableHead>
                    <TableHead>Resource</TableHead>
                    <TableHead>Status</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {loading ? (
                    <TableRow>
                      <TableCell colSpan={5} className="h-24 text-center">
                        <div className="flex justify-center">
                          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                        </div>
                      </TableCell>
                    </TableRow>
                  ) : logs.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={5} className="h-24 text-center text-muted-foreground">
                        No audit activity found for this customer.
                      </TableCell>
                    </TableRow>
                  ) : (
                    logs.map((log) => (
                      <TableRow key={log.id}>
                        <TableCell className="whitespace-nowrap text-sm text-muted-foreground">
                          {new Date(log.timestamp).toLocaleString()}
                        </TableCell>
                        <TableCell>
                          <div className="flex flex-col">
                            <span className="font-medium capitalize">{log.actor_type}</span>
                            <span className="text-xs text-muted-foreground">{log.actor_id || "System"}</span>
                          </div>
                        </TableCell>
                        <TableCell>
                          <Badge variant={getActionBadgeVariant(log.action)} className="capitalize">
                            {log.action.replace(/\./g, " ")}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          <div className="flex flex-col">
                            <span className="text-xs font-semibold uppercase text-muted-foreground">{log.resource_type}</span>
                            <span className="text-sm">{log.resource_id || "N/A"}</span>
                          </div>
                        </TableCell>
                        <TableCell>
                          {log.success ? <Badge variant="success">Success</Badge> : <Badge variant="destructive">Failed</Badge>}
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>

            {(cursorStack.length > 0 || hasMore) && (
              <div className="mt-4 flex items-center justify-between">
                <p className="text-sm text-muted-foreground">Showing {logs.length} items</p>
                <div className="flex gap-2">
                  <Button variant="outline" size="sm" onClick={handlePrevPage} disabled={cursorStack.length === 0 || loading}>
                    <ChevronLeft className="mr-1 h-4 w-4" />
                    Previous
                  </Button>
                  <Button variant="outline" size="sm" onClick={handleNextPage} disabled={!hasMore || loading}>
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
