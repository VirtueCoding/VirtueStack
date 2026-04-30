"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  adminInvoicesApi,
  type Invoice,
  type InvoiceListParams,
} from "@/lib/api-client";
import {
  Button,
  Badge,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Input,
  useToast,
} from "@virtuestack/ui";
import { Download, Eye, Ban, FileText, Receipt } from "lucide-react";

function formatCents(cents: number, currency = "USD"): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency,
  }).format(cents / 100);
}

function statusVariant(
  status: Invoice["status"]
): "default" | "secondary" | "destructive" | "outline" {
  const variants: Record<
    string,
    "default" | "secondary" | "destructive" | "outline"
  > = {
    draft: "secondary",
    issued: "default",
    paid: "outline",
    void: "destructive",
  };
  return variants[status] ?? "secondary";
}

export default function InvoicesPage() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [filters, setFilters] = useState<InvoiceListParams>({ per_page: 20 });
  const [selectedInvoice, setSelectedInvoice] = useState<Invoice | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ["admin-invoices", filters],
    queryFn: () => adminInvoicesApi.list(filters),
  });

  const voidMutation = useMutation({
    mutationFn: (id: string) => adminInvoicesApi.voidInvoice(id),
    onSuccess: () => {
      toast({ title: "Invoice voided", description: "The invoice has been voided successfully." });
      queryClient.invalidateQueries({ queryKey: ["admin-invoices"] });
      setSelectedInvoice(null);
    },
    onError: () => toast({ title: "Error", description: "Failed to void invoice.", variant: "destructive" }),
  });

  const invoices = data?.data ?? [];
  const meta = data?.meta;

  return (
    <div className="min-h-screen bg-background p-6 md:p-8">
      <div className="mx-auto max-w-7xl space-y-6">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Invoices</h1>
          <p className="text-muted-foreground">View and manage billing invoices</p>
        </div>

        <div className="flex flex-wrap gap-3">
          <Select
            value={filters.status ?? "all"}
            onValueChange={(v) =>
              setFilters((f) => ({ ...f, status: v === "all" ? undefined : v, cursor: undefined }))
            }
          >
            <SelectTrigger className="w-[150px]">
              <SelectValue placeholder="Status" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All statuses</SelectItem>
              <SelectItem value="draft">Draft</SelectItem>
              <SelectItem value="issued">Issued</SelectItem>
              <SelectItem value="paid">Paid</SelectItem>
              <SelectItem value="void">Void</SelectItem>
            </SelectContent>
          </Select>
          <Input
            type="text"
            placeholder="Customer ID"
            className="w-[280px]"
            value={filters.customer_id ?? ""}
            onChange={(e) =>
              setFilters((f) => ({ ...f, customer_id: e.target.value || undefined, cursor: undefined }))
            }
          />
        </div>

        <div className="rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Invoice #</TableHead>
                <TableHead>Period</TableHead>
                <TableHead>Total</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Issued</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow>
                  <TableCell colSpan={6} className="text-center py-8 text-muted-foreground">
                    Loading invoices...
                  </TableCell>
                </TableRow>
              ) : invoices.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={6} className="text-center py-8 text-muted-foreground">
                    <Receipt className="mx-auto h-8 w-8 mb-2 opacity-50" />
                    No invoices found
                  </TableCell>
                </TableRow>
              ) : (
                invoices.map((inv) => (
                  <TableRow key={inv.id}>
                    <TableCell className="font-mono text-sm">{inv.invoice_number}</TableCell>
                    <TableCell className="text-sm">
                      {new Date(inv.period_start).toLocaleDateString()} –{" "}
                      {new Date(inv.period_end).toLocaleDateString()}
                    </TableCell>
                    <TableCell className="font-medium">{formatCents(inv.total, inv.currency)}</TableCell>
                    <TableCell>
                      <Badge variant={statusVariant(inv.status)}>{inv.status}</Badge>
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {inv.issued_at ? new Date(inv.issued_at).toLocaleDateString() : "—"}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-1">
                        <Button variant="ghost" size="icon" onClick={() => setSelectedInvoice(inv)} title="View">
                          <Eye className="h-4 w-4" />
                        </Button>
                        <Button variant="ghost" size="icon" asChild title="Download PDF">
                          <a href={adminInvoicesApi.getPDFUrl(inv.id)} target="_blank" rel="noopener noreferrer">
                            <Download className="h-4 w-4" />
                          </a>
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>

        {meta?.has_more && (
          <div className="flex justify-center">
            <Button variant="outline" onClick={() => setFilters((f) => ({ ...f, cursor: meta.next_cursor }))}>
              Load more
            </Button>
          </div>
        )}

        <Dialog open={!!selectedInvoice} onOpenChange={() => setSelectedInvoice(null)}>
          <DialogContent className="max-w-2xl">
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <FileText className="h-5 w-5" />
                Invoice {selectedInvoice?.invoice_number}
              </DialogTitle>
              <DialogDescription>
                {selectedInvoice?.period_start &&
                  `${new Date(selectedInvoice.period_start).toLocaleDateString()} – ${new Date(selectedInvoice.period_end).toLocaleDateString()}`}
              </DialogDescription>
            </DialogHeader>
            {selectedInvoice && (
              <div className="space-y-4">
                <div className="flex justify-between items-center">
                  <Badge variant={statusVariant(selectedInvoice.status)}>{selectedInvoice.status}</Badge>
                  <div className="flex gap-2">
                    <Button variant="outline" size="sm" asChild>
                      <a href={adminInvoicesApi.getPDFUrl(selectedInvoice.id)} target="_blank" rel="noopener noreferrer">
                        <Download className="mr-1 h-4 w-4" />PDF
                      </a>
                    </Button>
                    {(selectedInvoice.status === "draft" || selectedInvoice.status === "issued") && (
                      <Button
                        variant="destructive"
                        size="sm"
                        onClick={() => voidMutation.mutate(selectedInvoice.id)}
                        disabled={voidMutation.isPending}
                      >
                        <Ban className="mr-1 h-4 w-4" />Void
                      </Button>
                    )}
                  </div>
                </div>

                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Description</TableHead>
                      <TableHead className="text-center">Qty</TableHead>
                      <TableHead className="text-right">Unit Price</TableHead>
                      <TableHead className="text-right">Amount</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {selectedInvoice.line_items.map((item, idx) => (
                      <TableRow key={idx}>
                        <TableCell>{item.description}</TableCell>
                        <TableCell className="text-center">{item.quantity}</TableCell>
                        <TableCell className="text-right">{formatCents(item.unit_price)}</TableCell>
                        <TableCell className="text-right font-medium">{formatCents(item.amount)}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>

                <div className="border-t pt-3 space-y-1 text-sm">
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Subtotal</span>
                    <span>{formatCents(selectedInvoice.subtotal)}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Tax</span>
                    <span>{formatCents(selectedInvoice.tax_amount)}</span>
                  </div>
                  <div className="flex justify-between font-bold text-base">
                    <span>Total</span>
                    <span>{formatCents(selectedInvoice.total, selectedInvoice.currency)}</span>
                  </div>
                </div>
              </div>
            )}
          </DialogContent>
        </Dialog>
      </div>
    </div>
  );
}
