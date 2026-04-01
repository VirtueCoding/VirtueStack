"use client";

import { useState, useEffect, useCallback } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@virtuestack/ui";
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
  HardDrive,
  Plus,
  Search,
  Loader2,
  Pencil,
  Trash2,
} from "lucide-react";
import {
  adminStorageBackendsApi,
  type StorageBackend,
} from "@/lib/api-client";
import { useToast } from "@virtuestack/ui";
import { StorageBackendCreateDialog, type CreateStorageBackendFormData } from "@/components/storage-backends/StorageBackendCreateDialog";
import { StorageBackendEditDialog, type EditStorageBackendFormData } from "@/components/storage-backends/StorageBackendEditDialog";

function getHealthBadgeVariant(status: string): "success" | "warning" | "destructive" | "secondary" {
  switch (status) {
    case "healthy":
      return "success";
    case "warning":
      return "warning";
    case "critical":
      return "destructive";
    default:
      return "secondary";
  }
}

function HealthBadge({ status, message }: { status: string; message?: string }) {
  return (
    <div className="flex items-center gap-2">
      <Badge variant={getHealthBadgeVariant(status)} className="capitalize">
        {status}
      </Badge>
      {message && (
        <span className="text-xs text-muted-foreground" title={message}>
          {message.length > 30 ? `${message.substring(0, 30)}...` : message}
        </span>
      )}
    </div>
  );
}

function StorageTypeBadge({ type }: { type: string }) {
  const variant = type === "ceph" ? "default" : "secondary";
  return (
    <Badge variant={variant} className="uppercase">
      {type}
    </Badge>
  );
}

function getConfigDisplay(backend: StorageBackend): string {
  switch (backend.type) {
    case "ceph":
      return `pool: ${backend.ceph_pool || "N/A"}`;
    case "qcow":
      return `path: ${backend.storage_path || "N/A"}`;
    case "lvm":
      return `vg: ${backend.lvm_volume_group || "N/A"}, pool: ${backend.lvm_thin_pool || "N/A"}`;
    default:
      return "Unknown";
  }
}

export default function StorageBackendsPage() {
  const [searchTerm, setSearchTerm] = useState("");
  const [backends, setBackends] = useState<StorageBackend[]>([]);
  const [loading, setLoading] = useState(true);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [isCreating, setIsCreating] = useState(false);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [editingBackend, setEditingBackend] = useState<StorageBackend | null>(null);
  const [isSaving, setIsSaving] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deletingBackend, setDeletingBackend] = useState<StorageBackend | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);
  const { toast } = useToast();

  const fetchBackends = useCallback(async () => {
    try {
      const data = await adminStorageBackendsApi.getStorageBackends();
      setBackends(data || []);
    } catch (err) {
      toast({
        title: "Error",
        description: "Failed to load storage backends.",
        variant: "destructive",
      });
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    fetchBackends();
  }, [fetchBackends]);

  const filteredBackends = backends.filter(
    (backend) =>
      backend.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      backend.type.toLowerCase().includes(searchTerm.toLowerCase())
  );

  const handleCreate = async (data: CreateStorageBackendFormData) => {
    setIsCreating(true);
    try {
      await adminStorageBackendsApi.createStorageBackend({
        name: data.name,
        type: data.type,
        ceph_pool: data.ceph_pool || undefined,
        ceph_user: data.ceph_user || undefined,
        ceph_monitors: data.ceph_monitors || undefined,
        ceph_keyring_path: data.ceph_keyring_path || undefined,
        storage_path: data.storage_path || undefined,
        lvm_volume_group: data.lvm_volume_group || undefined,
        lvm_thin_pool: data.lvm_thin_pool || undefined,
        node_ids: data.node_ids,
      });
      toast({
        title: "Storage Backend Created",
        description: `Storage backend "${data.name}" has been created successfully.`,
      });
      await fetchBackends();
    } finally {
      setIsCreating(false);
    }
  };

  const handleEdit = (backend: StorageBackend) => {
    setEditingBackend(backend);
    setEditDialogOpen(true);
  };

  const handleSave = async (data: EditStorageBackendFormData) => {
    if (!editingBackend) return;
    setIsSaving(true);
    try {
      await adminStorageBackendsApi.updateStorageBackend(editingBackend.id, {
        name: data.name,
        ceph_pool: data.ceph_pool || undefined,
        ceph_user: data.ceph_user || undefined,
        ceph_monitors: data.ceph_monitors || undefined,
        ceph_keyring_path: data.ceph_keyring_path || undefined,
        storage_path: data.storage_path || undefined,
        lvm_volume_group: data.lvm_volume_group || undefined,
        lvm_thin_pool: data.lvm_thin_pool || undefined,
      });
      // Update node assignments
      if (data.node_ids !== undefined) {
        await adminStorageBackendsApi.assignStorageBackendNodes(editingBackend.id, data.node_ids);
      }
      toast({
        title: "Storage Backend Updated",
        description: `Storage backend "${editingBackend.name}" has been updated successfully.`,
      });
      await fetchBackends();
    } finally {
      setIsSaving(false);
    }
  };

  const handleDeleteClick = (backend: StorageBackend) => {
    setDeletingBackend(backend);
    setDeleteDialogOpen(true);
  };

  const handleDeleteConfirm = async () => {
    if (!deletingBackend) return;
    setIsDeleting(true);
    try {
      await adminStorageBackendsApi.deleteStorageBackend(deletingBackend.id);
      toast({
        title: "Storage Backend Deleted",
        description: `Storage backend "${deletingBackend.name}" has been deleted.`,
      });
      await fetchBackends();
      setDeleteDialogOpen(false);
      setDeletingBackend(null);
    } catch (error) {
      toast({
        title: "Delete Failed",
        description: error instanceof Error ? error.message : "Failed to delete storage backend",
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
            <h1 className="text-3xl font-bold tracking-tight">Storage Backends</h1>
            <p className="text-muted-foreground">
              Manage storage backends for VM disk provisioning
            </p>
          </div>
          <Button onClick={() => setCreateDialogOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            Add Storage Backend
          </Button>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>Storage Configuration</CardTitle>
            <CardDescription>
              Configure Ceph, QCOW2, and LVM thin storage backends for your cluster
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="mb-6 flex items-center gap-4">
              <div className="relative flex-1 max-w-sm">
                <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
                <Input
                  placeholder="Search backends..."
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
                    <TableHead>Type</TableHead>
                    <TableHead>Configuration</TableHead>
                    <TableHead>Nodes</TableHead>
                    <TableHead>Health</TableHead>
                    <TableHead>Capacity</TableHead>
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
                  ) : filteredBackends.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={7} className="h-24 text-center">
                        No storage backends found.
                      </TableCell>
                    </TableRow>
                  ) : (
                    filteredBackends.map((backend) => (
                      <TableRow key={backend.id}>
                        <TableCell className="font-medium">{backend.name}</TableCell>
                        <TableCell>
                          <StorageTypeBadge type={backend.type} />
                        </TableCell>
                        <TableCell className="text-muted-foreground">
                          {getConfigDisplay(backend)}
                        </TableCell>
                        <TableCell>
                          <Badge variant="outline">
                            {backend.nodes?.length || 0} nodes
                          </Badge>
                        </TableCell>
                        <TableCell>
                          <HealthBadge status={backend.health_status} message={backend.health_message} />
                          {backend.type === "lvm" && backend.lvm_data_percent !== undefined && (
                            <div className="mt-1 text-xs text-muted-foreground">
                              Data: {backend.lvm_data_percent.toFixed(1)}%
                              {backend.lvm_metadata_percent !== undefined && (
                                <span className="ml-2">Meta: {backend.lvm_metadata_percent.toFixed(1)}%</span>
                              )}
                            </div>
                          )}
                        </TableCell>
                        <TableCell>
                          {backend.total_gb !== undefined ? (
                            <div className="text-xs">
                              <div>{backend.used_gb?.toFixed(1) || 0} / {backend.total_gb} GB</div>
                              <div className="text-muted-foreground">
                                {backend.available_gb?.toFixed(1) || 0} GB available
                              </div>
                            </div>
                          ) : (
                            <span className="text-xs text-muted-foreground">N/A</span>
                          )}
                        </TableCell>
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-2">
                            <Button
                              variant="ghost"
                              size="icon"
                              onClick={() => handleEdit(backend)}
                              title="Edit Storage Backend"
                            >
                              <Pencil className="h-4 w-4" />
                              <span className="sr-only">Edit</span>
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon"
                              onClick={() => handleDeleteClick(backend)}
                              title="Delete Storage Backend"
                              className="text-destructive hover:text-destructive"
                            >
                              <Trash2 className="h-4 w-4" />
                              <span className="sr-only">Delete</span>
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

        {/* Create Storage Backend Dialog */}
        <StorageBackendCreateDialog
          open={createDialogOpen}
          onOpenChange={setCreateDialogOpen}
          onCreate={handleCreate}
          isCreating={isCreating}
        />

        {/* Edit Storage Backend Dialog */}
        <StorageBackendEditDialog
          open={editDialogOpen}
          onOpenChange={setEditDialogOpen}
          backend={editingBackend}
          onSave={handleSave}
          isSaving={isSaving}
          onDelete={handleDeleteClick}
        />

        {/* Delete Confirmation Dialog */}
        {deleteDialogOpen && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
            <Card className="w-full max-w-md mx-4">
                <CardHeader>
                  <CardTitle>Delete Storage Backend</CardTitle>
                  <CardDescription>
                    Are you sure you want to delete &quot;{deletingBackend?.name}&quot;?
                    This action cannot be undone.
                  </CardDescription>
                </CardHeader>
              <CardContent>
                {deletingBackend?.nodes && deletingBackend.nodes.length > 0 && (
                  <div className="mb-4 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
                    Warning: This backend has {deletingBackend.nodes.length} node(s) assigned.
                    You must remove node assignments before deleting.
                  </div>
                )}
              </CardContent>
              <div className="flex justify-end gap-2 p-6 pt-0">
                <Button
                  variant="outline"
                  onClick={() => {
                    setDeleteDialogOpen(false);
                    setDeletingBackend(null);
                  }}
                  disabled={isDeleting}
                >
                  Cancel
                </Button>
                <Button
                  variant="destructive"
                  onClick={handleDeleteConfirm}
                  disabled={isDeleting || (deletingBackend?.nodes?.length || 0) > 0}
                >
                  {isDeleting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                  Delete
                </Button>
              </div>
            </Card>
          </div>
        )}
      </div>
    </div>
  );
}
