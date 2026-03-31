"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { CreditCard, ArrowUpCircle, ArrowDownCircle, DollarSign, Clock, History } from "lucide-react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { RequireAuth } from "@/lib/require-auth";
import { Sidebar } from "@/components/sidebar";
import { MobileNav } from "@/components/mobile-nav";
import { ThemeToggle } from "@/components/theme-toggle";
import {
  billingApi,
  BillingTransaction,
  BillingPayment,
  TopUpConfig,
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

function BalanceCard() {
  const { data: balance, isLoading } = useQuery({
    queryKey: ["billing", "balance"],
    queryFn: () => billingApi.getBalance(),
  });

  return (
    <div className="rounded-lg border bg-card p-6">
      <div className="flex items-center gap-2 text-muted-foreground mb-2">
        <DollarSign className="h-4 w-4" />
        <span className="text-sm font-medium">Current Balance</span>
      </div>
      {isLoading ? (
        <div className="h-8 w-24 animate-pulse rounded bg-muted" />
      ) : (
        <p className="text-3xl font-bold">
          {balance ? formatCents(balance.balance) : "$0.00"}
        </p>
      )}
      <p className="text-xs text-muted-foreground mt-1">
        {balance?.currency || "USD"}
      </p>
    </div>
  );
}

function TopUpSection() {
  const queryClient = useQueryClient();
  const [selectedAmount, setSelectedAmount] = useState<number | null>(null);
  const [selectedGateway, setSelectedGateway] = useState<string | null>(null);

  const { data: config } = useQuery({
    queryKey: ["billing", "topup-config"],
    queryFn: () => billingApi.getTopUpConfig(),
  });

  const topUpMutation = useMutation({
    mutationFn: (amount: number) => {
      const gateway = selectedGateway || config?.gateways[0] || "stripe";
      const returnURL =
        gateway === "paypal"
          ? `${window.location.origin}/billing/paypal-return`
          : window.location.href;
      return billingApi.initiateTopUp({
        gateway,
        amount,
        currency: config?.currency || "USD",
        return_url: returnURL,
        cancel_url: window.location.href,
      });
    },
    onSuccess: (data) => {
      if (data.payment_url) {
        window.location.href = data.payment_url;
      }
      queryClient.invalidateQueries({ queryKey: ["billing"] });
    },
  });

  if (!config) return null;

  const gateways = config.gateways || [];
  const activeGateway = selectedGateway || gateways[0] || "stripe";

  return (
    <div className="rounded-lg border bg-card p-6">
      <div className="flex items-center gap-2 mb-4">
        <ArrowUpCircle className="h-5 w-5 text-green-500" />
        <h3 className="text-lg font-semibold">Add Funds</h3>
      </div>
      {gateways.length > 1 && (
        <div className="flex gap-2 mb-4">
          {gateways.map((gw) => (
            <button
              key={gw}
              onClick={() => setSelectedGateway(gw)}
              className={`rounded-md border px-3 py-1.5 text-xs font-medium capitalize transition-colors ${
                activeGateway === gw
                  ? "border-primary bg-primary text-primary-foreground"
                  : "border-border hover:bg-accent"
              }`}
            >
              {gw === "paypal" ? "PayPal" : gw === "stripe" ? "Credit Card" : gw}
            </button>
          ))}
        </div>
      )}
      <div className="grid grid-cols-3 sm:grid-cols-5 gap-2 mb-4">
        {config.presets.map((amount) => (
          <button
            key={amount}
            onClick={() => setSelectedAmount(amount)}
            className={`rounded-md border px-3 py-2 text-sm font-medium transition-colors ${
              selectedAmount === amount
                ? "border-primary bg-primary text-primary-foreground"
                : "border-border hover:bg-accent"
            }`}
          >
            {formatCents(amount)}
          </button>
        ))}
      </div>
      <button
        onClick={() => selectedAmount && topUpMutation.mutate(selectedAmount)}
        disabled={!selectedAmount || topUpMutation.isPending}
        className="w-full rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
      >
        {topUpMutation.isPending
          ? "Processing..."
          : `Add Funds via ${activeGateway === "paypal" ? "PayPal" : "Card"}`}
      </button>
    </div>
  );
}

function TransactionList() {
  const { data, isLoading } = useQuery({
    queryKey: ["billing", "transactions"],
    queryFn: () => billingApi.getTransactions({ perPage: 20 }),
  });

  const transactions = data?.data || [];

  return (
    <div className="rounded-lg border bg-card">
      <div className="p-4 border-b">
        <h3 className="font-semibold flex items-center gap-2">
          <History className="h-4 w-4" />
          Transaction History
        </h3>
      </div>
      {isLoading ? (
        <div className="p-8 text-center text-muted-foreground">Loading...</div>
      ) : transactions.length === 0 ? (
        <div className="p-8 text-center text-muted-foreground">
          No transactions yet
        </div>
      ) : (
        <div className="divide-y">
          {transactions.map((tx: BillingTransaction) => (
            <div key={tx.id} className="flex items-center justify-between p-4">
              <div className="flex items-center gap-3">
                {tx.type === "credit" || tx.type === "adjustment" ? (
                  <ArrowUpCircle className="h-4 w-4 text-green-500" />
                ) : (
                  <ArrowDownCircle className="h-4 w-4 text-red-500" />
                )}
                <div>
                  <p className="text-sm font-medium">{tx.description}</p>
                  <p className="text-xs text-muted-foreground">
                    {formatDate(tx.created_at)}
                  </p>
                </div>
              </div>
              <div className="text-right">
                <p
                  className={`text-sm font-medium ${
                    tx.type === "credit" || tx.type === "adjustment"
                      ? "text-green-600"
                      : "text-red-600"
                  }`}
                >
                  {tx.type === "credit" || tx.type === "adjustment" ? "+" : "-"}
                  {formatCents(tx.amount)}
                </p>
                <p className="text-xs text-muted-foreground">
                  Balance: {formatCents(tx.balance_after)}
                </p>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function PaymentList() {
  const { data, isLoading } = useQuery({
    queryKey: ["billing", "payments"],
    queryFn: () => billingApi.getPayments({ perPage: 20 }),
  });

  const payments = data?.data || [];

  const statusColors: Record<string, string> = {
    completed: "text-green-600 bg-green-50 dark:bg-green-900/20",
    pending: "text-yellow-600 bg-yellow-50 dark:bg-yellow-900/20",
    failed: "text-red-600 bg-red-50 dark:bg-red-900/20",
    refunded: "text-blue-600 bg-blue-50 dark:bg-blue-900/20",
  };

  return (
    <div className="rounded-lg border bg-card">
      <div className="p-4 border-b">
        <h3 className="font-semibold flex items-center gap-2">
          <CreditCard className="h-4 w-4" />
          Payment History
        </h3>
      </div>
      {isLoading ? (
        <div className="p-8 text-center text-muted-foreground">Loading...</div>
      ) : payments.length === 0 ? (
        <div className="p-8 text-center text-muted-foreground">
          No payments yet
        </div>
      ) : (
        <div className="divide-y">
          {payments.map((payment: BillingPayment) => (
            <div key={payment.id} className="flex items-center justify-between p-4">
              <div className="flex items-center gap-3">
                <CreditCard className="h-4 w-4 text-muted-foreground" />
                <div>
                  <p className="text-sm font-medium capitalize">{payment.gateway}</p>
                  <p className="text-xs text-muted-foreground">
                    {formatDate(payment.created_at)}
                  </p>
                </div>
              </div>
              <div className="flex items-center gap-3">
                <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${statusColors[payment.status] || ""}`}>
                  {payment.status}
                </span>
                <p className="text-sm font-medium">{formatCents(payment.amount)}</p>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export default function BillingPage() {
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);

  return (
    <RequireAuth>
      <div className="flex h-screen">
        <Sidebar collapsed={sidebarCollapsed} setCollapsed={setSidebarCollapsed} />
        <div className="flex-1 flex flex-col overflow-hidden">
          <header className="flex h-14 items-center justify-between border-b px-4 lg:px-6">
            <MobileNav />
            <h1 className="text-lg font-semibold">Billing</h1>
            <ThemeToggle />
          </header>
          <main className="flex-1 overflow-y-auto p-4 lg:p-6">
            <div className="mx-auto max-w-4xl space-y-6">
              <div className="grid gap-6 md:grid-cols-2">
                <BalanceCard />
                <TopUpSection />
              </div>
              <Tabs defaultValue="transactions">
                <TabsList>
                  <TabsTrigger value="transactions">
                    <Clock className="h-4 w-4 mr-1" />
                    Transactions
                  </TabsTrigger>
                  <TabsTrigger value="payments">
                    <CreditCard className="h-4 w-4 mr-1" />
                    Payments
                  </TabsTrigger>
                </TabsList>
                <TabsContent value="transactions" className="mt-4">
                  <TransactionList />
                </TabsContent>
                <TabsContent value="payments" className="mt-4">
                  <PaymentList />
                </TabsContent>
              </Tabs>
            </div>
          </main>
        </div>
      </div>
    </RequireAuth>
  );
}
