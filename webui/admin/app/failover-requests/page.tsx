"use client";

import { useCallback, useEffect, useState } from "react";
import { Eye, GitBranch, Loader2, RefreshCw } from "lucide-react";

import { Badge } from "@virtuestack/ui";
import { Button } from "@virtuestack/ui";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@virtuestack/ui";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@virtuestack/ui";
import { Input } from "@virtuestack/ui";
import { Label } from "@virtuestack/ui";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@virtuestack/ui";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@virtuestack/ui";
import { useToast } from "@virtuestack/ui";
import {
  adminFailoverRequestsApi,
  type FailoverRequest,
  type PaginatedResponse,
} from "@/lib/api-client";

const statusOptions = ["all", "pending", "approved", "in_progress", "completed", "failed", "cancelled"] as const;

function formatDateTime(value?: string): string {
  if (!value) return "—";
  return new Date(value).toLocaleString();
}

function getStatusVariant(status: string): "success" | "warning" | "destructive" | "secondary" {
  switch (status) {
    case "completed":
      return "success";
    case "pending":
    case "approved":
    case "in_progress":
      return "warning";
    case "failed":
    case "cancelled":
      return "destructive";
    default:
      return "secondary";
  }
}

function formatResult(result?: FailoverRequest["result"]): string {
  if (!result) return "No result payload recorded.";
  if (typeof result === "string") return result;
  return JSON.stringify(result, null, 2);
}

export default function FailoverRequestsPage() {
  const { toast } = useToast();
  const [response, setResponse] = useState<PaginatedResponse<FailoverRequest> | null>(null);
  const [loading, setLoading] = useState(true);
  const [viewingId, setViewingId] = useState<string | null>(null);
  const [selectedRequest, setSelectedRequest] = useState<FailoverRequest | null>(null);
  const [detailOpen, setDetailOpen] = useState(false);
  const [nodeFilter, setNodeFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState<(typeof statusOptions)[number]>("all");
  const [cursorStack, setCursorStack] = useState<string[]>([]);
  const [nextCursor, setNextCursor] = useState<string | undefined>(undefined);

  const loadRequests = useCallback(async () => {
    setLoading(true);
    try {
      const currentCursor = cursorStack.length > 0 ? cursorStack[cursorStack.length - 1] : undefined;
      const data = await adminFailoverRequestsApi.getFailoverRequests({
        per_page: 20,
        cursor: currentCursor,
        node_id: nodeFilter.trim() || undefined,
        status: statusFilter === "all" ? undefined : statusFilter,
      });
      setResponse(data);
      setNextCursor(data.meta.next_cursor);
    } catch (error) {
      toast({
        title: "Error",
        description: error instanceof Error ? error.message : "Failed to load failover requests.",
        variant: "destructive",
      });
    } finally {
      setLoading(false);
    }
  }, [nodeFilter, cursorStack, statusFilter, toast]);

  useEffect(() => {
    loadRequests();
  }, [loadRequests]);

  const handleView = async (requestId: string) => {
    setViewingId(requestId);
    try {
      const data = await adminFailoverRequestsApi.getFailoverRequest(requestId);
      setSelectedRequest(data);
      setDetailOpen(true);
    } catch (error) {
      toast({
        title: "Error",
        description: error instanceof Error ? error.message : "Failed to load failover request.",
        variant: "destructive",
      });
    } finally {
      setViewingId(null);
    }
  };

  const handleStatusChange = (value: (typeof statusOptions)[number]) => {
    setStatusFilter(value);
    setCursorStack([]);
    setNextCursor(undefined);
  };

  const requests = response?.data || [];
  const meta = response?.meta;

  return (
    <div className="min-h-screen bg-background p-6 md:p-8">
      <div className="mx-auto max-w-7xl space-y-8">
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">Failover Requests</h1>
            <p className="text-muted-foreground">
              Review cluster evacuation requests raised when a node needs its VMs moved elsewhere.
            </p>
          </div>
          <Button variant="outline" onClick={loadRequests}>
            <RefreshCw className="mr-2 h-4 w-4" />
            Refresh
          </Button>
        </div>

        <div className="grid gap-4 md:grid-cols-3">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-base">What this feature does</CardTitle>
              <CardDescription>
                A failover request tracks the evacuation of all VMs from a problematic node.
              </CardDescription>
            </CardHeader>
            <CardContent className="text-sm text-muted-foreground">
              Operators use it to monitor whether a node drain or emergency failover has been requested, approved, completed, or failed.
            </CardContent>
          </Card>
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-base">Operational value</CardTitle>
              <CardDescription>
                The request lifecycle creates an audit trail for infrastructure incidents.
              </CardDescription>
            </CardHeader>
            <CardContent className="text-sm text-muted-foreground">
              It helps admins understand who initiated the action, why it happened, when it completed, and any machine-readable result payload returned by the workflow.
            </CardContent>
          </Card>
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-base">Current page</CardTitle>
              <CardDescription>
                Showing results for the current filters.
              </CardDescription>
            </CardHeader>
            <CardContent className="text-sm text-muted-foreground">
              Filter by node UUID or lifecycle status to inspect a specific incident or migration event.
            </CardContent>
          </Card>
        </div>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <GitBranch className="h-5 w-5" />
              Failover Activity
            </CardTitle>
            <CardDescription>
              These records are read-only history; failover is initiated from node operations, while this screen provides observability and detail.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-6">
            <div className="grid gap-4 md:grid-cols-[minmax(0,1fr),220px]">
              <div className="space-y-2">
                <Label htmlFor="node-filter">Filter by node UUID</Label>
                <Input
                  id="node-filter"
                  value={nodeFilter}
                  onChange={(event) => {
                    setNodeFilter(event.target.value);
                    setCursorStack([]);
                    setNextCursor(undefined);
                  }}
                  placeholder="Optional node UUID"
                />
              </div>
              <div className="space-y-2">
                <Label>Status</Label>
                <Select value={statusFilter} onValueChange={handleStatusChange}>
                  <SelectTrigger>
                    <SelectValue placeholder="All statuses" />
                  </SelectTrigger>
                  <SelectContent>
                    {statusOptions.map((status) => (
                      <SelectItem key={status} value={status} className="capitalize">
                        {status.replace(/_/g, " ")}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Node</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Requested By</TableHead>
                    <TableHead>Reason</TableHead>
                    <TableHead>Created</TableHead>
                    <TableHead>Completed</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {loading ? (
                    <TableRow>
                      <TableCell colSpan={7} className="h-24 text-center">
                        <div className="flex justify-center">
                          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                        </div>
                      </TableCell>
                    </TableRow>
                  ) : requests.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={7} className="h-24 text-center text-muted-foreground">
                        No failover requests found.
                      </TableCell>
                    </TableRow>
                  ) : (
                    requests.map((request) => (
                      <TableRow key={request.id}>
                        <TableCell className="font-mono text-xs text-muted-foreground">{request.node_id}</TableCell>
                        <TableCell>
                          <Badge variant={getStatusVariant(request.status)} className="capitalize">
                            {request.status.replace(/_/g, " ")}
                          </Badge>
                        </TableCell>
                        <TableCell className="font-mono text-xs text-muted-foreground">{request.requested_by}</TableCell>
                        <TableCell className="max-w-xs text-sm text-muted-foreground">
                          {request.reason || "—"}
                        </TableCell>
                        <TableCell>{formatDateTime(request.created_at)}</TableCell>
                        <TableCell>{formatDateTime(request.completed_at)}</TableCell>
                        <TableCell className="text-right">
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => handleView(request.id)}
                            disabled={viewingId === request.id}
                            title="View failover request details"
                          >
                            {viewingId === request.id ? (
                              <Loader2 className="h-4 w-4 animate-spin" />
                            ) : (
                              <Eye className="h-4 w-4" />
                            )}
                            <span className="sr-only">View</span>
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>

            <div className="flex items-center justify-end gap-4">
              <div className="flex gap-2">
                <Button variant="outline" onClick={() => setCursorStack((s) => s.slice(0, -1))} disabled={cursorStack.length === 0}>
                  Previous
                </Button>
                <Button variant="outline" onClick={() => { if (nextCursor) setCursorStack((s) => [...s, nextCursor]); }} disabled={!nextCursor}>
                  Next
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>

        <Dialog open={detailOpen} onOpenChange={setDetailOpen}>
          <DialogContent className="max-w-3xl">
            <DialogHeader>
              <DialogTitle>Failover Request Details</DialogTitle>
              <DialogDescription>
                Review the request lifecycle and any result payload produced by the failover workflow.
              </DialogDescription>
            </DialogHeader>
            {selectedRequest && (
              <div className="space-y-4">
                <div className="grid gap-4 md:grid-cols-2">
                  <div>
                    <Label className="text-xs uppercase text-muted-foreground">Request ID</Label>
                    <p className="mt-1 font-mono text-xs">{selectedRequest.id}</p>
                  </div>
                  <div>
                    <Label className="text-xs uppercase text-muted-foreground">Node ID</Label>
                    <p className="mt-1 font-mono text-xs">{selectedRequest.node_id}</p>
                  </div>
                  <div>
                    <Label className="text-xs uppercase text-muted-foreground">Status</Label>
                    <div className="mt-1">
                      <Badge variant={getStatusVariant(selectedRequest.status)} className="capitalize">
                        {selectedRequest.status.replace(/_/g, " ")}
                      </Badge>
                    </div>
                  </div>
                  <div>
                    <Label className="text-xs uppercase text-muted-foreground">Requested By</Label>
                    <p className="mt-1 font-mono text-xs">{selectedRequest.requested_by}</p>
                  </div>
                  <div>
                    <Label className="text-xs uppercase text-muted-foreground">Created</Label>
                    <p className="mt-1 text-sm">{formatDateTime(selectedRequest.created_at)}</p>
                  </div>
                  <div>
                    <Label className="text-xs uppercase text-muted-foreground">Approved</Label>
                    <p className="mt-1 text-sm">{formatDateTime(selectedRequest.approved_at)}</p>
                  </div>
                  <div>
                    <Label className="text-xs uppercase text-muted-foreground">Completed</Label>
                    <p className="mt-1 text-sm">{formatDateTime(selectedRequest.completed_at)}</p>
                  </div>
                  <div>
                    <Label className="text-xs uppercase text-muted-foreground">Last Updated</Label>
                    <p className="mt-1 text-sm">{formatDateTime(selectedRequest.updated_at)}</p>
                  </div>
                </div>
                <div>
                  <Label className="text-xs uppercase text-muted-foreground">Reason</Label>
                  <p className="mt-1 text-sm text-muted-foreground">{selectedRequest.reason || "No reason provided."}</p>
                </div>
                <div>
                  <Label className="text-xs uppercase text-muted-foreground">Result Payload</Label>
                  <pre className="mt-1 max-h-80 overflow-auto rounded-md border bg-muted p-3 text-xs">
                    {formatResult(selectedRequest.result)}
                  </pre>
                </div>
              </div>
            )}
          </DialogContent>
        </Dialog>
      </div>
    </div>
  );
}
