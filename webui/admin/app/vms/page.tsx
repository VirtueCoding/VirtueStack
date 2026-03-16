"use client";

import { useState, useEffect } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Server,
  Search,
  Trash2,
  Loader2,
  Eye,
} from "lucide-react";
import { adminVMsApi, type VM } from "@/lib/api-client";
import { useToast } from "@/components/ui/use-toast";
import { getStatusBadgeVariant } from "@/lib/status-badge";

export default function VMsPage() {
  const [searchTerm, setSearchTerm] = useState("");
  const [vms, setVMs] = useState<VM[]>([]);
  const [loading, setLoading] = useState(true);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [selectedVM, setSelectedVM] = useState<VM | null>(null);
  const [viewDialogOpen, setViewDialogOpen] = useState(false);
  const [viewVM, setViewVM] = useState<VM | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);
  const { toast } = useToast();

  useEffect(() => {
    async function fetchVMs() {
      try {
        const data = await adminVMsApi.getVMs();
        setVMs(data || []);
      } catch (err) {
        toast({
          title: "Error",
          description: "Failed to load VMs.",
          variant: "destructive",
        });
      } finally {
        setLoading(false);
      }
    }
    fetchVMs();
  }, [toast]);

  const filteredVMs = vms.filter(
    (vm) =>
      vm.id.toLowerCase().includes(searchTerm.toLowerCase()) ||
      vm.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      vm.status.toLowerCase().includes(searchTerm.toLowerCase()) ||
      vm.customer_id.toLowerCase().includes(searchTerm.toLowerCase())
  );

  const handleDelete = async () => {
    if (!selectedVM) return;
    setIsDeleting(true);
    try {
      await adminVMsApi.deleteVM(selectedVM.id);
      setVMs((prev) => prev.filter((vm) => vm.id !== selectedVM.id));
      toast({
        title: "VM Deleted",
        description: `VM "${selectedVM.name}" has been deleted.`,
      });
      setDeleteDialogOpen(false);
      setSelectedVM(null);
    } catch (error) {
      toast({
        title: "Delete Failed",
        description: error instanceof Error ? error.message : "Failed to delete VM",
        variant: "destructive",
      });
    } finally {
      setIsDeleting(false);
    }
  };

  return (
    <div className="min-h-screen bg-background p-6 md:p-8">
      <div className="mx-auto max-w-7xl space-y-8">
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">Virtual Machines</h1>
            <p className="text-muted-foreground">
              Manage all virtual machines across the cluster
            </p>
          </div>
        </div>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Server className="h-5 w-5" />
              All VMs
            </CardTitle>
            <CardDescription>
              {filteredVMs.length} of {vms.length} VMs displayed
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="mb-6 flex items-center gap-4">
              <div className="relative flex-1 max-w-sm">
                <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
                <Input
                  placeholder="Search VMs by ID, name, status..."
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
                    <TableHead>Name</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Customer ID</TableHead>
                    <TableHead>Node ID</TableHead>
                    <TableHead>Created</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
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
                  ) : filteredVMs.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={6} className="h-24 text-center">
                        No VMs found.
                      </TableCell>
                    </TableRow>
                  ) : (
                    filteredVMs.map((vm) => (
                      <TableRow key={vm.id}>
                        <TableCell>
                          <div className="font-medium">{vm.name}</div>
                          <div className="text-xs text-muted-foreground font-mono">
                            {vm.id.substring(0, 8)}...
                          </div>
                        </TableCell>
                        <TableCell>
                          <Badge variant={getStatusBadgeVariant(vm.status)} className="capitalize">
                            {vm.status}
                          </Badge>
                        </TableCell>
                        <TableCell className="font-mono text-xs text-muted-foreground">
                          {vm.customer_id.substring(0, 8)}...
                        </TableCell>
                        <TableCell className="font-mono text-xs text-muted-foreground">
                          {vm.node_id.substring(0, 8)}...
                        </TableCell>
                        <TableCell className="text-muted-foreground">
                          {new Date(vm.created_at).toLocaleDateString()}
                        </TableCell>
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-2">
                            <Button
                              variant="ghost"
                              size="icon"
                              title="View Details"
                              onClick={() => {
                                setViewVM(vm);
                                setViewDialogOpen(true);
                              }}
                            >
                              <Eye className="h-4 w-4" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon"
                              title="Delete VM"
                              onClick={() => {
                                setSelectedVM(vm);
                                setDeleteDialogOpen(true);
                              }}
                            >
                              <Trash2 className="h-4 w-4 text-destructive" />
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

        <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Delete Virtual Machine</DialogTitle>
              <DialogDescription>
                Are you sure you want to permanently delete <strong>{selectedVM?.name}</strong>?
                This action cannot be undone and will destroy all associated data.
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button variant="outline" onClick={() => setDeleteDialogOpen(false)}>
                Cancel
              </Button>
              <Button
                variant="destructive"
                onClick={handleDelete}
                disabled={isDeleting}
              >
                {isDeleting ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Deleting...
                  </>
                ) : (
                  "Delete VM"
                )}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Dialog open={viewDialogOpen} onOpenChange={setViewDialogOpen}>
          <DialogContent className="max-w-lg">
            <DialogHeader>
              <DialogTitle>Virtual Machine Details</DialogTitle>
              <DialogDescription>
                Detailed information for <strong>{viewVM?.name}</strong>
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-3 py-2">
              <div className="grid grid-cols-2 gap-2 text-sm">
                <span className="text-muted-foreground">ID:</span>
                <span className="font-mono">{viewVM?.id}</span>
                <span className="text-muted-foreground">Status:</span>
                <Badge variant={getStatusBadgeVariant(viewVM?.status || "unknown")} className="capitalize">{viewVM?.status}</Badge>
                <span className="text-muted-foreground">Customer ID:</span>
                <span className="font-mono">{viewVM?.customer_id}</span>
                <span className="text-muted-foreground">Node ID:</span>
                <span className="font-mono">{viewVM?.node_id}</span>
                <span className="text-muted-foreground">Created:</span>
                <span>{viewVM?.created_at ? new Date(viewVM.created_at).toLocaleString() : "—"}</span>
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setViewDialogOpen(false)}>
                Close
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </div>
  );
}
