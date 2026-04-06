"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Input,
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
  Loader2,
  Search,
  Server,
} from "lucide-react";

import { adminCustomersApi, adminVMsApi, type CustomerDetail, type VM } from "@/lib/api-client";
import { getStatusBadgeVariant } from "@/lib/status-badge";

const PAGE_SIZE = 20;

export default function CustomerVMsPage() {
  const params = useParams<{ id: string }>();
  const customerId = typeof params.id === "string" ? params.id : "";
  const [customer, setCustomer] = useState<CustomerDetail | null>(null);
  const [vms, setVMs] = useState<VM[]>([]);
  const [loading, setLoading] = useState(true);
  const [currentCursor, setCurrentCursor] = useState<string | undefined>(undefined);
  const [cursorStack, setCursorStack] = useState<string[]>([]);
  const [nextCursor, setNextCursor] = useState<string | undefined>(undefined);
  const [hasMore, setHasMore] = useState(false);
  const [searchTerm, setSearchTerm] = useState("");
  const { toast } = useToast();

  const loadData = useCallback(async () => {
    if (!customerId) return;
    setLoading(true);
    try {
      const [customerResponse, vmResponse] = await Promise.all([
        adminCustomersApi.getCustomer(customerId),
        adminVMsApi.getVMs({
          customer_id: customerId,
          per_page: PAGE_SIZE,
          cursor: currentCursor,
        }),
      ]);
      setCustomer(customerResponse);
      setVMs(vmResponse.data || []);
      setNextCursor(vmResponse.meta?.next_cursor ?? undefined);
      setHasMore(vmResponse.meta?.has_more ?? false);
    } catch {
      toast({
        title: "Error",
        description: "Failed to load the customer VM list.",
        variant: "destructive",
      });
    } finally {
      setLoading(false);
    }
  }, [currentCursor, customerId, toast]);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  const filteredVMs = useMemo(() => {
    const query = searchTerm.trim().toLowerCase();
    if (!query) return vms;
    return vms.filter((vm) =>
      [vm.id, vm.name, vm.hostname, vm.status, vm.node_id]
        .filter(Boolean)
        .some((value) => value!.toLowerCase().includes(query))
    );
  }, [searchTerm, vms]);

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
            <h1 className="text-3xl font-bold tracking-tight">Customer VMs</h1>
            <p className="text-muted-foreground">
              {customer ? `Virtual machines assigned to ${customer.name}.` : "Loading customer context..."}
            </p>
          </div>
        </div>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Server className="h-5 w-5" />
              Assigned virtual machines
            </CardTitle>
            <CardDescription>
              Filtered from the admin VM endpoint using the backend-supported customer_id query.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="mb-6 flex items-center gap-4">
              <div className="relative flex-1 max-w-sm">
                <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
                <Input
                  placeholder="Search this customer's VMs..."
                  className="pl-8"
                  value={searchTerm}
                  onChange={(event) => setSearchTerm(event.target.value)}
                />
              </div>
            </div>

            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Hostname</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Node ID</TableHead>
                    <TableHead>Created</TableHead>
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
                  ) : filteredVMs.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={5} className="h-24 text-center text-muted-foreground">
                        No VMs found for this customer.
                      </TableCell>
                    </TableRow>
                  ) : (
                    filteredVMs.map((vm) => (
                      <TableRow key={vm.id}>
                        <TableCell>
                          <div className="flex flex-col">
                            <span className="font-medium">{vm.name}</span>
                            <span className="text-xs text-muted-foreground">{vm.id}</span>
                          </div>
                        </TableCell>
                        <TableCell>{vm.hostname || "—"}</TableCell>
                        <TableCell>
                          <Badge variant={getStatusBadgeVariant(vm.status)} className="capitalize">
                            {vm.status}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-sm text-muted-foreground">{vm.node_id}</TableCell>
                        <TableCell className="whitespace-nowrap text-sm text-muted-foreground">
                          {new Date(vm.created_at).toLocaleString()}
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>

            {(cursorStack.length > 0 || hasMore) && (
              <div className="mt-4 flex items-center justify-between">
                <p className="text-sm text-muted-foreground">Showing {filteredVMs.length} items</p>
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
