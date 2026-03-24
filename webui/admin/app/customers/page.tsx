"use client";

import { useState, useEffect, useCallback } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { getStatusBadgeVariant } from "@/lib/status-badge";
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
  Plus,
  Search,
  Eye,
  Ban,
  CheckCircle,
  Trash2,
  Loader2,
  Pencil,
} from "lucide-react";
import { adminCustomersApi, type Customer } from "@/lib/api-client";
import { useRouter } from "next/navigation";
import { useToast } from "@/components/ui/use-toast";
import { CustomerCreateDialog, type CreateCustomerFormData } from "@/components/customers/CustomerCreateDialog";
import { CustomerEditDialog, type EditCustomerFormData } from "@/components/customers/CustomerEditDialog";

type DialogAction = "suspend" | "unsuspend" | "delete" | null;

export default function CustomersPage() {
  const router = useRouter();
  const [searchTerm, setSearchTerm] = useState("");
  const [customers, setCustomers] = useState<Customer[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingId, setLoadingId] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [dialogAction, setDialogAction] = useState<DialogAction>(null);
  const [selectedCustomer, setSelectedCustomer] = useState<Customer | null>(null);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [editCustomer, setEditCustomer] = useState<Customer | null>(null);
  const [isCreating, setIsCreating] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const { toast } = useToast();

  const loadCustomers = useCallback(async () => {
    try {
      const response = await adminCustomersApi.getCustomers();
      setCustomers(response.data || []);
    } catch (err) {
      toast({
        title: "Error",
        description: "Failed to load customers.",
        variant: "destructive",
      });
    }
  }, [toast]);

  useEffect(() => {
    async function loadData() {
      await loadCustomers();
      setLoading(false);
    }
    loadData();
  }, [loadCustomers]);

  const filteredCustomers = customers.filter(
    (customer) =>
      customer.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      customer.email.toLowerCase().includes(searchTerm.toLowerCase())
  );

  const handleView = (customer: Customer) => {
    router.push(`/customers/${customer.id}`);
  };

  const openConfirmDialog = (customer: Customer, action: DialogAction) => {
    setSelectedCustomer(customer);
    setDialogAction(action);
    setDialogOpen(true);
  };

  const openEditDialog = (customer: Customer) => {
    setEditCustomer(customer);
    setEditDialogOpen(true);
  };

  const handleConfirmAction = async () => {
    if (!selectedCustomer || !dialogAction) return;

    setLoadingId(selectedCustomer.id);
    setDialogOpen(false);

    try {
      if (dialogAction === "suspend") {
        await adminCustomersApi.suspendCustomer(selectedCustomer.id);
        toast({
          title: "Customer Suspended",
          description: `${selectedCustomer.name} has been suspended.`,
        });
      } else if (dialogAction === "unsuspend") {
        await adminCustomersApi.unsuspendCustomer(selectedCustomer.id);
        toast({
          title: "Customer Unsuspended",
          description: `${selectedCustomer.name} has been reactivated.`,
        });
      } else if (dialogAction === "delete") {
        await adminCustomersApi.deleteCustomer(selectedCustomer.id);
        toast({
          title: "Customer Deleted",
          description: `${selectedCustomer.name} has been permanently deleted.`,
        });
      }

      await loadCustomers();
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

  const handleCreateCustomer = async (data: CreateCustomerFormData) => {
    setIsCreating(true);
    try {
      await adminCustomersApi.createCustomer({
        name: data.name,
        email: data.email,
        password: data.password,
        phone: data.phone || undefined,
      });
      toast({
        title: "Customer Created",
        description: `${data.name} has been created successfully.`,
      });
      await loadCustomers();
    } catch (error) {
      throw error;
    } finally {
      setIsCreating(false);
    }
  };

  const handleEditCustomer = async (data: EditCustomerFormData) => {
    if (!editCustomer) return;
    setIsSaving(true);
    try {
      await adminCustomersApi.updateCustomer(editCustomer.id, {
        name: data.name,
        status: data.status,
      });
      toast({
        title: "Customer Updated",
        description: `${data.name} has been updated successfully.`,
      });
      await loadCustomers();
    } catch (error) {
      throw error;
    } finally {
      setIsSaving(false);
    }
  };

  function getInitials(name: string) {
    if (!name?.trim()) return "??";
    return name
      .split(" ")
      .map((n) => n[0])
      .join("")
      .toUpperCase()
      .substring(0, 2);
  }

  return (
    <div className="min-h-screen bg-background p-6 md:p-8">
      <div className="mx-auto max-w-7xl space-y-8">
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">Customers</h1>
            <p className="text-muted-foreground">
              Manage client accounts and subscriptions
            </p>
          </div>
          <Button onClick={() => setCreateDialogOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            Add Customer
          </Button>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>Customer Directory</CardTitle>
            <CardDescription>
              View and manage all registered customers
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="mb-6 flex items-center gap-4">
              <div className="relative flex-1 max-w-sm">
                <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
                <Input
                  placeholder="Search customers..."
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
                    <TableHead>Customer</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>VMs</TableHead>
                    <TableHead>Joined</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
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
                  ) : filteredCustomers.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={5} className="h-24 text-center">
                        No customers found.
                      </TableCell>
                    </TableRow>
                  ) : (
                    filteredCustomers.map((customer) => (
                      <TableRow key={customer.id}>
                        <TableCell>
                          <div className="flex items-center gap-3">
                            <Avatar className="h-9 w-9">
                              <AvatarFallback>{getInitials(customer.name)}</AvatarFallback>
                            </Avatar>
                            <div>
                              <div className="font-medium">{customer.name}</div>
                              <div className="text-xs text-muted-foreground">
                                {customer.email}
                              </div>
                            </div>
                          </div>
                        </TableCell>
                        <TableCell>
                          <Badge variant={getStatusBadgeVariant(customer.status) as React.ComponentProps<typeof Badge>["variant"]} className="capitalize">
                            {customer.status}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-1 font-medium">
                            {customer.vm_count}
                          </div>
                        </TableCell>
                        <TableCell className="text-muted-foreground">
                          {new Date(customer.created_at).toLocaleDateString()}
                        </TableCell>
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-2">
                            <Button
                              variant="ghost"
                              size="icon"
                              onClick={() => handleView(customer)}
                              title="View Profile"
                            >
                              <Eye className="h-4 w-4" />
                              <span className="sr-only">View Profile</span>
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon"
                              onClick={() => openEditDialog(customer)}
                              disabled={loadingId === customer.id}
                              title="Edit Customer"
                            >
                              <Pencil className="h-4 w-4" />
                              <span className="sr-only">Edit Customer</span>
                            </Button>
                            {customer.status === "active" ? (
                              <Button
                                variant="ghost"
                                size="icon"
                                onClick={() => openConfirmDialog(customer, "suspend")}
                                disabled={loadingId === customer.id}
                                title="Suspend Account"
                              >
                                {loadingId === customer.id && dialogAction === "suspend" ? (
                                  <Loader2 className="h-4 w-4 animate-spin" />
                                ) : (
                                  <Ban className="h-4 w-4 text-warning" />
                                )}
                                <span className="sr-only">Suspend Account</span>
                              </Button>
                            ) : (
                              <Button
                                variant="ghost"
                                size="icon"
                                onClick={() => openConfirmDialog(customer, "unsuspend")}
                                disabled={loadingId === customer.id}
                                title="Unsuspend Account"
                              >
                                {loadingId === customer.id && dialogAction === "unsuspend" ? (
                                  <Loader2 className="h-4 w-4 animate-spin" />
                                ) : (
                                  <CheckCircle className="h-4 w-4 text-success" />
                                )}
                                <span className="sr-only">Unsuspend Account</span>
                              </Button>
                            )}

                            <Button
                              variant="ghost"
                              size="icon"
                              onClick={() => openConfirmDialog(customer, "delete")}
                              disabled={loadingId === customer.id}
                              title="Delete Account"
                            >
                              {loadingId === customer.id && dialogAction === "delete" ? (
                                <Loader2 className="h-4 w-4 animate-spin" />
                              ) : (
                                <Trash2 className="h-4 w-4 text-destructive" />
                              )}
                              <span className="sr-only">Delete Account</span>
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

        {/* Confirmation Dialog for Suspend/Unsuspend/Delete */}
        <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>
                {dialogAction === "suspend" && "Suspend Customer"}
                {dialogAction === "unsuspend" && "Unsuspend Customer"}
                {dialogAction === "delete" && "Delete Customer"}
              </DialogTitle>
              <DialogDescription>
                {dialogAction === "suspend" && (
                  <>
                    Are you sure you want to suspend <strong>{selectedCustomer?.name}</strong>?
                    This will immediately disable access to their account and optionally stop their VMs.
                  </>
                )}
                {dialogAction === "unsuspend" && (
                  <>
                    Are you sure you want to unsuspend <strong>{selectedCustomer?.name}</strong>?
                    This will restore their account access.
                  </>
                )}
                {dialogAction === "delete" && (
                  <>
                    Are you sure you want to permanently delete <strong>{selectedCustomer?.name}</strong>?
                    This action cannot be undone and will destroy all associated VMs and data.
                  </>
                )}
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button variant="outline" onClick={() => setDialogOpen(false)}>
                Cancel
              </Button>
              <Button
                variant={
                  dialogAction === "delete"
                    ? "destructive"
                    : "default"
                }
                onClick={handleConfirmAction}
              >
                Confirm {dialogAction && dialogAction.charAt(0).toUpperCase() + dialogAction.slice(1)}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {/* Create Customer Dialog */}
        <CustomerCreateDialog
          open={createDialogOpen}
          onOpenChange={setCreateDialogOpen}
          onCreate={handleCreateCustomer}
          isCreating={isCreating}
        />

        {/* Edit Customer Dialog */}
        <CustomerEditDialog
          open={editDialogOpen}
          onOpenChange={setEditDialogOpen}
          customer={editCustomer}
          onSave={handleEditCustomer}
          isSaving={isSaving}
        />
      </div>
    </div>
  );
}