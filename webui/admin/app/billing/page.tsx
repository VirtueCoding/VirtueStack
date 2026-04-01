"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Card, CardContent, CardHeader, CardTitle } from "@virtuestack/ui";
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@virtuestack/ui";
import {
  CreditCard,
  DollarSign,
  RefreshCw,
  ArrowLeft,
  ArrowRight,
} from "lucide-react";
import {
  adminBillingApi,
  BillingPayment,
  BillingTransaction,
} from "@/lib/api-client";

function formatCents(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

const statusVariant: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
  completed: "default",
  pending: "secondary",
  failed: "destructive",
  refunded: "outline",
};

function PaymentsTab() {
  const [cursor, setCursor] = useState<string | undefined>();
  const [cursorStack, setCursorStack] = useState<string[]>([]);
  const [refundPaymentId, setRefundPaymentId] = useState<string | null>(null);
  const [refundAmount, setRefundAmount] = useState("");
  const [refundReason, setRefundReason] = useState("");
  const queryClient = useQueryClient();

  const { data, isLoading } = useQuery({
    queryKey: ["admin", "billing", "payments", cursor],
    queryFn: () => adminBillingApi.getPayments({ perPage: 20, cursor }),
  });

  const refundMutation = useMutation({
    mutationFn: () =>
      adminBillingApi.refundPayment(refundPaymentId!, {
        amount: Math.round(parseFloat(refundAmount) * 100),
        reason: refundReason,
      }),
    onSuccess: () => {
      setRefundPaymentId(null);
      setRefundAmount("");
      setRefundReason("");
      queryClient.invalidateQueries({ queryKey: ["admin", "billing"] });
    },
  });

  const payments = data?.data || [];
  const hasMore = data?.meta?.has_more || false;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <CreditCard className="h-5 w-5" />
          Payments
        </CardTitle>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Date</TableHead>
              <TableHead>Customer</TableHead>
              <TableHead>Gateway</TableHead>
              <TableHead>Amount</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              <TableRow>
                <TableCell colSpan={6} className="text-center py-8">
                  <RefreshCw className="h-4 w-4 animate-spin mx-auto" />
                </TableCell>
              </TableRow>
            ) : payments.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="text-center py-8 text-muted-foreground">
                  No payments found
                </TableCell>
              </TableRow>
            ) : (
              payments.map((p: BillingPayment) => (
                <TableRow key={p.id}>
                  <TableCell className="text-sm">{formatDate(p.created_at)}</TableCell>
                  <TableCell className="text-sm font-mono truncate max-w-[120px]">
                    {p.customer_id.slice(0, 8)}...
                  </TableCell>
                  <TableCell className="text-sm capitalize">{p.gateway}</TableCell>
                  <TableCell className="text-sm font-medium">
                    {formatCents(p.amount)}
                  </TableCell>
                  <TableCell>
                    <Badge variant={statusVariant[p.status] || "secondary"}>
                      {p.status}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    {p.status === "completed" && (
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => {
                          setRefundPaymentId(p.id);
                          setRefundAmount((p.amount / 100).toFixed(2));
                        }}
                      >
                        Refund
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>

        <div className="flex justify-between items-center mt-4">
          <Button
            variant="outline"
            size="sm"
            disabled={cursorStack.length === 0}
            onClick={() => {
              const prev = [...cursorStack];
              prev.pop();
              setCursorStack(prev);
              setCursor(prev[prev.length - 1]);
            }}
          >
            <ArrowLeft className="h-4 w-4 mr-1" /> Previous
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={!hasMore}
            onClick={() => {
              if (data?.meta?.next_cursor) {
                setCursorStack([...cursorStack, data.meta.next_cursor]);
                setCursor(data.meta.next_cursor);
              }
            }}
          >
            Next <ArrowRight className="h-4 w-4 ml-1" />
          </Button>
        </div>

        <Dialog
          open={refundPaymentId !== null}
          onOpenChange={() => setRefundPaymentId(null)}
        >
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Refund Payment</DialogTitle>
              <DialogDescription>
                Enter the refund amount and reason.
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div>
                <label className="text-sm font-medium">Amount ($)</label>
                <Input
                  type="number"
                  value={refundAmount}
                  onChange={(e) => setRefundAmount(e.target.value)}
                  step="0.01"
                  min="0.01"
                />
              </div>
              <div>
                <label className="text-sm font-medium">Reason</label>
                <Input
                  value={refundReason}
                  onChange={(e) => setRefundReason(e.target.value)}
                  placeholder="Reason for refund"
                />
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setRefundPaymentId(null)}>
                Cancel
              </Button>
              <Button
                variant="destructive"
                onClick={() => refundMutation.mutate()}
                disabled={
                  refundMutation.isPending ||
                  !refundAmount ||
                  !refundReason
                }
              >
                {refundMutation.isPending ? "Processing..." : "Refund"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </CardContent>
    </Card>
  );
}

function TransactionsTab() {
  const [cursor, setCursor] = useState<string | undefined>();
  const [cursorStack, setCursorStack] = useState<string[]>([]);

  const { data, isLoading } = useQuery({
    queryKey: ["admin", "billing", "transactions", cursor],
    queryFn: () => adminBillingApi.getTransactions({ perPage: 20, cursor }),
  });

  const transactions = data?.data || [];
  const hasMore = data?.meta?.has_more || false;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <DollarSign className="h-5 w-5" />
          Transactions
        </CardTitle>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Date</TableHead>
              <TableHead>Customer</TableHead>
              <TableHead>Type</TableHead>
              <TableHead>Amount</TableHead>
              <TableHead>Balance After</TableHead>
              <TableHead>Description</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              <TableRow>
                <TableCell colSpan={6} className="text-center py-8">
                  <RefreshCw className="h-4 w-4 animate-spin mx-auto" />
                </TableCell>
              </TableRow>
            ) : transactions.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="text-center py-8 text-muted-foreground">
                  No transactions found
                </TableCell>
              </TableRow>
            ) : (
              transactions.map((tx: BillingTransaction) => (
                <TableRow key={tx.id}>
                  <TableCell className="text-sm">{formatDate(tx.created_at)}</TableCell>
                  <TableCell className="text-sm font-mono truncate max-w-[120px]">
                    {tx.customer_id.slice(0, 8)}...
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant={
                        tx.type === "credit" || tx.type === "adjustment"
                          ? "default"
                          : "destructive"
                      }
                    >
                      {tx.type}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-sm font-medium">
                    {formatCents(tx.amount)}
                  </TableCell>
                  <TableCell className="text-sm">
                    {formatCents(tx.balance_after)}
                  </TableCell>
                  <TableCell className="text-sm max-w-[200px] truncate">
                    {tx.description}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>

        <div className="flex justify-between items-center mt-4">
          <Button
            variant="outline"
            size="sm"
            disabled={cursorStack.length === 0}
            onClick={() => {
              const prev = [...cursorStack];
              prev.pop();
              setCursorStack(prev);
              setCursor(prev[prev.length - 1]);
            }}
          >
            <ArrowLeft className="h-4 w-4 mr-1" /> Previous
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={!hasMore}
            onClick={() => {
              if (data?.meta?.next_cursor) {
                setCursorStack([...cursorStack, data.meta.next_cursor]);
                setCursor(data.meta.next_cursor);
              }
            }}
          >
            Next <ArrowRight className="h-4 w-4 ml-1" />
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

export default function AdminBillingPage() {
  const [activeTab, setActiveTab] = useState<"payments" | "transactions">("payments");

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold tracking-tight">Billing Management</h2>
        <p className="text-muted-foreground">
          View payments, transactions, and manage refunds.
        </p>
      </div>

      <div className="flex gap-2">
        <Button
          variant={activeTab === "payments" ? "default" : "outline"}
          size="sm"
          onClick={() => setActiveTab("payments")}
        >
          <CreditCard className="h-4 w-4 mr-1" />
          Payments
        </Button>
        <Button
          variant={activeTab === "transactions" ? "default" : "outline"}
          size="sm"
          onClick={() => setActiveTab("transactions")}
        >
          <DollarSign className="h-4 w-4 mr-1" />
          Transactions
        </Button>
      </div>

      {activeTab === "payments" ? <PaymentsTab /> : <TransactionsTab />}
    </div>
  );
}
