"use client";

import { useState } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
  User,
  Plus,
  Search,
  Eye,
  Ban,
  CheckCircle,
  Trash2,
  Loader2,
} from "lucide-react";
import { adminCustomersApi, type Customer } from "@/lib/api-client";
import { useToast } from "@/components/ui/use-toast";

const mockCustomers: Customer[] = [
  {
    id: "1",
    name: "Acme Corporation",
    email: "admin@acme.com",
    vm_count: 24,
    status: "active",
    created_at: "2025-01-15",
  },
  {
    id: "2",
    name: "TechStart Inc",
    email: "contact@techstart.io",
    vm_count: 8,
    status: "active",
    created_at: "2025-02-03",
  },
  {
    id: "3",
    name: "Global Services Ltd",
    email: "it@globalservices.com",
    vm_count: 42,
    status: "active",
    created_at: "2024-11-20",
  },
  {
    id: "4",
    name: "DevShop Studio",
    email: "hello@devshop.dev",
    vm_count: 12,
    status: "suspended",
    created_at: "2025-03-10",
  },
  {
    id: "5",
    name: "CloudNine Systems",
    email: "ops@cloudnine.net",
    vm_count: 67,
    status: "active",
    created_at: "2024-08-05",
  },
  {
    id: "6",
    name: "DataFlow Analytics",
    email: "support@dataflow.ai",
    vm_count: 31,
    status: "active",
    created_at: "2024-12-12",
  },
  {
    id: "7",
    name: "Pixel Perfect Design",
    email: "team@pixelperfect.design",
    vm_count: 5,
    status: "suspended",
    created_at: "2025-04-22",
  },
  {
    id: "8",
    name: "Enterprise Solutions Group",
    email: "admin@esg-corp.com",
    vm_count: 156,
    status: "active",
    created_at: "2024-06-18",
  },
];

function getStatusBadge(status: Customer["status"]) {
  const variants = {
    active: "success" as const,
    suspended: "destructive" as const,
  };

  const labels = {
    active: "Active",
    suspended: "Suspended",
  };

  return (
    <Badge variant={variants[status]}>{labels[status]}</Badge>
  );
}

function getCustomerAvatar(name: string) {
  const initials = name
    .split(" ")
    .map((word) => word[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();

  return (
    <Avatar>
      <AvatarFallback className="bg-primary/10 text-primary font-semibold">
        {initials}
      </AvatarFallback>
    </Avatar>
  );
}

type DialogAction = "suspend" | "unsuspend" | "delete" | null;

export default function CustomersPage() {
  const [searchTerm, setSearchTerm] = useState("");
  const [customers, setCustomers] = useState<Customer[]>(mockCustomers);
  const [loadingId, setLoadingId] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [dialogAction, setDialogAction] = useState<DialogAction>(null);
  const [selectedCustomer, setSelectedCustomer] = useState<Customer | null>(null);
  const { toast } = useToast();

  const filteredCustomers = customers.filter(
    (customer) =>
      customer.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      customer.email.toLowerCase().includes(searchTerm.toLowerCase())
  );

  const handleView = (customer: Customer) => {
    toast({
      title: "Customer Details",
      description: `Viewing ${customer.name} (${customer.email})`,
    });
  };

  const openConfirmDialog = (customer: Customer, action: DialogAction) => {
    setSelectedCustomer(customer);
    setDialogAction(action);
    setDialogOpen(true);
  };

  const handleConfirmAction = async () => {
    if (!selectedCustomer || !dialogAction) return;

    setDialogOpen(false);
    setLoadingId(selectedCustomer.id);

    try {
      if (dialogAction === "suspend") {
        await adminCustomersApi.suspendCustomer(selectedCustomer.id);
        toast({
          title: "Customer Suspended",
          description: `Customer "${selectedCustomer.name}" has been suspended.`,
        });
        setCustomers((prev) =>
          prev.map((c) =>
            c.id === selectedCustomer.id ? { ...c, status: "suspended" } : c
          )
        );
      } else if (dialogAction === "unsuspend") {
        await adminCustomersApi.unsuspendCustomer(selectedCustomer.id);
        toast({
          title: "Customer Unsuspended",
          description: `Customer "${selectedCustomer.name}" has been unsuspended.`,
        });
        setCustomers((prev) =>
          prev.map((c) =>
            c.id === selectedCustomer.id ? { ...c, status: "active" } : c
          )
        );
      } else if (dialogAction === "delete") {
        await adminCustomersApi.deleteCustomer(selectedCustomer.id);
        toast({
          title: "Customer Deleted",
          description: `Customer "${selectedCustomer.name}" has been permanently deleted.`,
        });
        setCustomers((prev) => prev.filter((c) => c.id !== selectedCustomer.id));
      }
    } catch (error) {
      toast({
        title: "Action Failed",
        description: error instanceof Error ? error.message : `Failed to ${dialogAction} customer`,
        variant: "destructive",
      });
    } finally {
      setLoadingId(null);
      setSelectedCustomer(null);
      setDialogAction(null);
    }
  };

  const getDialogContent = () => {
    if (!selectedCustomer || !dialogAction) return null;

    switch (dialogAction) {
      case "suspend":
        return {
          title: "Suspend Customer",
          description: `Are you sure you want to suspend customer "${selectedCustomer.name}"? This will suspend all their VMs.`,
          confirmText: "Suspend",
          variant: "destructive" as const,
        };
      case "unsuspend":
        return {
          title: "Unsuspend Customer",
          description: `Are you sure you want to unsuspend customer "${selectedCustomer.name}"?`,
          confirmText: "Unsuspend",
          variant: "default" as const,
        };
      case "delete":
        return {
          title: "Delete Customer",
          description: `Are you sure you want to permanently delete customer "${selectedCustomer.name}"? This action cannot be undone.`,
          confirmText: "Delete",
          variant: "destructive" as const,
        };
      default:
        return null;
    }
  };

  const dialogContent = getDialogContent();

  const totalCustomers = customers.length;
  const activeCustomers = customers.filter((c) => c.status === "active").length;
  const suspendedCustomers = customers.filter((c) => c.status === "suspended").length;

  return (
    <div className="min-h-screen bg-background p-6 md:p-8">
      <div className="mx-auto max-w-7xl space-y-6">
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">Customers</h1>
            <p className="text-muted-foreground">
              Manage customer accounts and subscriptions
            </p>
          </div>
          <Button size="default">
            <Plus className="mr-2 h-4 w-4" />
            Add Customer
          </Button>
        </div>

        <div className="grid gap-4 sm:grid-cols-3">
          <Card>
            <CardContent className="pt-6">
              <div className="flex items-center gap-4">
                <div className="flex h-12 w-12 items-center justify-center rounded-full bg-blue-500/10">
                  <User className="h-5 w-5 text-blue-500" />
                </div>
                <div>
                  <div className="text-2xl font-bold">{totalCustomers}</div>
                  <p className="text-xs text-muted-foreground">Total Customers</p>
                </div>
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="pt-6">
              <div className="flex items-center gap-4">
                <div className="flex h-12 w-12 items-center justify-center rounded-full bg-green-500/10">
                  <div className="h-3 w-3 rounded-full bg-green-500" />
                </div>
                <div>
                  <div className="text-2xl font-bold">{activeCustomers}</div>
                  <p className="text-xs text-muted-foreground">Active</p>
                </div>
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="pt-6">
              <div className="flex items-center gap-4">
                <div className="flex h-12 w-12 items-center justify-center rounded-full bg-red-500/10">
                  <Ban className="h-5 w-5 text-red-500" />
                </div>
                <div>
                  <div className="text-2xl font-bold">{suspendedCustomers}</div>
                  <p className="text-xs text-muted-foreground">Suspended</p>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>

        <Card>
          <CardContent className="pt-6">
            <div className="relative">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Search by name or email..."
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
                className="pl-10"
              />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <User className="h-5 w-5" />
              All Customers
            </CardTitle>
            <CardDescription>
              {filteredCustomers.length} of {customers.length} customers displayed
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Customer</TableHead>
                    <TableHead>Email</TableHead>
                    <TableHead className="text-center">VMs</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Created</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredCustomers.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={6} className="h-24 text-center">
                        No customers found
                      </TableCell>
                    </TableRow>
                  ) : (
                    filteredCustomers.map((customer) => (
                      <TableRow key={customer.id}>
                        <TableCell>
                          <div className="flex items-center gap-3">
                            {getCustomerAvatar(customer.name)}
                            <div className="font-medium">{customer.name}</div>
                          </div>
                        </TableCell>
                        <TableCell className="text-muted-foreground">
                          {customer.email}
                        </TableCell>
                        <TableCell className="text-center">
                          <Badge variant="secondary">{customer.vm_count}</Badge>
                        </TableCell>
                        <TableCell>{getStatusBadge(customer.status)}</TableCell>
                        <TableCell className="text-muted-foreground">
                          {new Date(customer.created_at).toLocaleDateString("en-US", {
                            year: "numeric",
                            month: "short",
                            day: "numeric",
                          })}
                        </TableCell>
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-2">
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => handleView(customer)}
                              disabled={loadingId === customer.id}
                            >
                              <Eye className="mr-1 h-3 w-3" />
                              View
                            </Button>
                            {customer.status === "active" ? (
                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() => openConfirmDialog(customer, "suspend")}
                                disabled={loadingId === customer.id}
                              >
                                <Ban className="mr-1 h-3 w-3" />
                                Suspend
                              </Button>
                            ) : (
                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() => openConfirmDialog(customer, "unsuspend")}
                                disabled={loadingId === customer.id}
                              >
                                <CheckCircle className="mr-1 h-3 w-3" />
                                Unsuspend
                              </Button>
                            )}
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => openConfirmDialog(customer, "delete")}
                              disabled={loadingId === customer.id}
                              className="text-destructive hover:bg-destructive/10"
                            >
                              {loadingId === customer.id ? (
                                <Loader2 className="mr-1 h-3 w-3 animate-spin" />
                              ) : (
                                <Trash2 className="mr-1 h-3 w-3" />
                              )}
                              Delete
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>
          </CardContent>
        </Card>
      </div>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{dialogContent?.title}</DialogTitle>
            <DialogDescription>{dialogContent?.description}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>
              Cancel
            </Button>
            <Button 
              variant={dialogContent?.variant || "default"}
              onClick={handleConfirmAction}
            >
              {dialogContent?.confirmText}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
