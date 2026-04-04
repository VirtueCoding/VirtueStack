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
  useToast,
} from "@virtuestack/ui";
import {
  ArrowLeft,
  Boxes,
  History,
  Loader2,
  Mail,
  Phone,
  ShieldCheck,
  UserRound,
} from "lucide-react";

import { adminCustomersApi, type CustomerDetail } from "@/lib/api-client";

function getCustomerStatusVariant(status: CustomerDetail["status"]): "success" | "destructive" | "warning" | "secondary" {
  switch (status) {
    case "active":
      return "success";
    case "suspended":
      return "destructive";
    case "pending_verification":
      return "warning";
    default:
      return "secondary";
  }
}

export default function CustomerDetailPage() {
  const params = useParams<{ id: string }>();
  const customerId = typeof params.id === "string" ? params.id : "";
  const [customer, setCustomer] = useState<CustomerDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const { toast } = useToast();

  const loadCustomer = useCallback(async () => {
    if (!customerId) return;
    setLoading(true);
    try {
      setCustomer(await adminCustomersApi.getCustomer(customerId));
    } catch {
      toast({
        title: "Error",
        description: "Failed to load customer details.",
        variant: "destructive",
      });
    } finally {
      setLoading(false);
    }
  }, [customerId, toast]);

  useEffect(() => {
    void loadCustomer();
  }, [loadCustomer]);

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (!customer) {
    return (
      <div className="min-h-screen bg-background p-6 md:p-8">
        <div className="mx-auto max-w-5xl">
          <Card>
            <CardHeader>
              <CardTitle>Customer unavailable</CardTitle>
              <CardDescription>The requested customer could not be loaded.</CardDescription>
            </CardHeader>
            <CardContent>
              <Button asChild variant="outline">
                <Link href="/customers">
                  <ArrowLeft className="mr-2 h-4 w-4" />
                  Back to customers
                </Link>
              </Button>
            </CardContent>
          </Card>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-background p-6 md:p-8">
      <div className="mx-auto max-w-5xl space-y-8">
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div>
            <Button asChild variant="ghost" className="mb-2 -ml-4 w-fit">
              <Link href="/customers">
                <ArrowLeft className="mr-2 h-4 w-4" />
                Customers
              </Link>
            </Button>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-3xl font-bold tracking-tight">{customer.name}</h1>
              <Badge variant={getCustomerStatusVariant(customer.status)} className="capitalize">
                {customer.status.replace(/_/g, " ")}
              </Badge>
            </div>
            <p className="text-muted-foreground">
              Customer profile, operational counts, and quick links to related records.
            </p>
          </div>
          <div className="flex gap-2">
            <Button asChild variant="outline">
              <Link href={`/customers/${customer.id}/vms`}>
                <Boxes className="mr-2 h-4 w-4" />
                View VMs
              </Link>
            </Button>
            <Button asChild variant="outline">
              <Link href={`/customers/${customer.id}/audit-logs`}>
                <History className="mr-2 h-4 w-4" />
                Audit Logs
              </Link>
            </Button>
          </div>
        </div>

        <div className="grid gap-4 md:grid-cols-3">
          <Card>
            <CardHeader className="pb-2">
              <CardDescription>Total VMs</CardDescription>
              <CardTitle className="text-3xl">{customer.vm_count}</CardTitle>
            </CardHeader>
          </Card>
          <Card>
            <CardHeader className="pb-2">
              <CardDescription>Active VMs</CardDescription>
              <CardTitle className="text-3xl">{customer.active_vms}</CardTitle>
            </CardHeader>
          </Card>
          <Card>
            <CardHeader className="pb-2">
              <CardDescription>Backups</CardDescription>
              <CardTitle className="text-3xl">{customer.backup_count}</CardTitle>
            </CardHeader>
          </Card>
        </div>

        <div className="grid gap-6 lg:grid-cols-[1.35fr_0.85fr]">
          <Card>
            <CardHeader>
              <CardTitle>Customer details</CardTitle>
              <CardDescription>Core profile details returned by the admin API.</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-4 sm:grid-cols-2">
              <div className="rounded-lg border p-4">
                <div className="mb-2 flex items-center gap-2 text-sm font-medium text-muted-foreground">
                  <UserRound className="h-4 w-4" />
                  Customer ID
                </div>
                <p className="break-all text-sm">{customer.id}</p>
              </div>
              <div className="rounded-lg border p-4">
                <div className="mb-2 flex items-center gap-2 text-sm font-medium text-muted-foreground">
                  <Mail className="h-4 w-4" />
                  Email
                </div>
                <p className="text-sm">{customer.email}</p>
              </div>
              <div className="rounded-lg border p-4">
                <div className="mb-2 flex items-center gap-2 text-sm font-medium text-muted-foreground">
                  <Phone className="h-4 w-4" />
                  Phone
                </div>
                <p className="text-sm">{customer.phone || "Not provided"}</p>
              </div>
              <div className="rounded-lg border p-4">
                <div className="mb-2 flex items-center gap-2 text-sm font-medium text-muted-foreground">
                  <ShieldCheck className="h-4 w-4" />
                  Auth provider
                </div>
                <p className="text-sm capitalize">{customer.auth_provider || "local"}</p>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Timeline</CardTitle>
              <CardDescription>Useful timestamps for support and audit follow-up.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4 text-sm">
              <div className="rounded-lg border p-4">
                <p className="font-medium">Created</p>
                <p className="text-muted-foreground">{new Date(customer.created_at).toLocaleString()}</p>
              </div>
              <div className="rounded-lg border p-4">
                <p className="font-medium">Last updated</p>
                <p className="text-muted-foreground">
                  {customer.updated_at ? new Date(customer.updated_at).toLocaleString() : "Not available"}
                </p>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
