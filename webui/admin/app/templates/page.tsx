"use client";

import { useState, useEffect, useCallback } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@virtuestack/ui";
import { Button } from "@virtuestack/ui";
import { Badge } from "@virtuestack/ui";
import { Input } from "@virtuestack/ui";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@virtuestack/ui";
import { Label } from "@virtuestack/ui";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@virtuestack/ui";
import { HardDrive, Plus, Search, Loader2, Pencil, Trash2, Download, MoreHorizontal, Disc, Send, Database } from "lucide-react";
import { adminTemplatesApi, adminNodesApi, type Template, type UpdateTemplateRequest, type TemplateCacheEntry, type Node } from "@/lib/api-client";
import { useToast } from "@virtuestack/ui";
import { getStatusBadgeVariant } from "@/lib/status-badge";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@virtuestack/ui";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@virtuestack/ui";
import { TemplateEditDialog, type EditTemplateFormData } from "@/components/templates/TemplateEditDialog";
import { Checkbox } from "@/components/ui/checkbox";

type DialogAction = "create" | "edit" | "delete" | "import" | null;
type ISOSourceMode = "path" | "url";

const defaultBuildForm = {
  name: "",
  os_family: "ubuntu",
  os_version: "",
  iso_path: "",
  iso_url: "",
  node_id: "",
  storage_backend: "qcow",
  disk_size_gb: 10,
  memory_mb: 2048,
  vcpus: 2,
  root_password: "",
};

function hasDistributableSource(template: Template) {
  if (template.storage_backend === "ceph" || !template.file_path) {
    return false;
  }

  try {
    const parsed = new URL(template.file_path);
    return parsed.protocol === "http:" || parsed.protocol === "https:";
  } catch {
    return false;
  }
}

function getStatusBadge(status: string) {
  const statusLabels: Record<string, string> = {
    active: "Active",
    inactive: "Inactive",
    pending: "Pending",
    importing: "Importing",
  };

  return (
    <Badge variant={getStatusBadgeVariant(status) as React.ComponentProps<typeof Badge>["variant"]}>
      {statusLabels[status] || status}
    </Badge>
  );
}

interface TemplateFormData {
  name: string;
  os_family: string;
  rbd_image: string;
}

export default function TemplatesPage() {
  const [searchTerm, setSearchTerm] = useState("");
  const [templates, setTemplates] = useState<Template[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [dialogAction, setDialogAction] = useState<DialogAction>(null);
  const [selectedTemplate, setSelectedTemplate] = useState<Template | null>(null);
  const [saving, setSaving] = useState(false);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [editSaving, setEditSaving] = useState(false);
  const [formData, setFormData] = useState<TemplateFormData>({
    name: "",
    os_family: "debian",
    rbd_image: "",
  });
  const [buildDialogOpen, setBuildDialogOpen] = useState(false);
  const [buildForm, setBuildForm] = useState({ ...defaultBuildForm });
  const [buildLoading, setBuildLoading] = useState(false);
  const [isoSourceMode, setIsoSourceMode] = useState<ISOSourceMode>("url");
  const [distributeDialogOpen, setDistributeDialogOpen] = useState(false);
  const [distributeTemplate, setDistributeTemplate] = useState<Template | null>(null);
  const [distributeLoading, setDistributeLoading] = useState(false);
  const [availableNodes, setAvailableNodes] = useState<Node[]>([]);
  const [selectedNodeIds, setSelectedNodeIds] = useState<string[]>([]);
  const [cacheStatusDialogOpen, setCacheStatusDialogOpen] = useState(false);
  const [cacheStatusTemplate, setCacheStatusTemplate] = useState<Template | null>(null);
  const [cacheEntries, setCacheEntries] = useState<TemplateCacheEntry[]>([]);
  const [cacheStatusLoading, setCacheStatusLoading] = useState(false);
  const { toast } = useToast();

  const loadTemplates = useCallback(async () => {
    try {
      setLoading(true);
      const data = await adminTemplatesApi.getTemplates();
      setTemplates(data || []);
    } catch (err) {
      setError("Failed to load templates");
      toast({
        title: "Error",
        description: "Failed to load templates.",
        variant: "destructive",
      });
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    void loadTemplates();
  }, [loadTemplates]);

  const filteredTemplates = templates.filter((template) =>
    template.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
    template.os_family.toLowerCase().includes(searchTerm.toLowerCase())
  );

  const openDialog = (action: DialogAction, template?: Template) => {
    setDialogAction(action);
    if (template) {
      setSelectedTemplate(template);
      // Open edit dialog separately for the TemplateEditDialog component
      if (action === "edit") {
        setEditDialogOpen(true);
        return;
      }
      setFormData({
        name: template.name,
        os_family: template.os_family,
        rbd_image: template.rbd_image || "",
      });
    } else {
      setSelectedTemplate(null);
      setFormData({ name: "", os_family: "debian", rbd_image: "" });
    }
    setDialogOpen(true);
  };

  const closeDialog = () => {
    setDialogOpen(false);
    setDialogAction(null);
    setSelectedTemplate(null);
    setFormData({ name: "", os_family: "debian", rbd_image: "" });
  };

  const handleCreate = async () => {
    setSaving(true);
    try {
      const created = await adminTemplatesApi.createTemplate({
        name: formData.name,
        os_family: formData.os_family,
        rbd_image: formData.rbd_image || undefined,
      });
      setTemplates((prev) => [...prev, created]);
      toast({
        title: "Template Created",
        description: `Template "${created.name}" has been created successfully.`,
      });
      closeDialog();
    } catch (err) {
      toast({
        title: "Error",
        description: err instanceof Error ? err.message : "Failed to create template",
        variant: "destructive",
      });
    } finally {
      setSaving(false);
    }
  };

  const handleEditSave = async (data: EditTemplateFormData) => {
    if (!selectedTemplate) return;
    setEditSaving(true);
    try {
      const updateData: UpdateTemplateRequest = {};
      // Only include fields that have values
      if (data.name !== undefined) updateData.name = data.name;
      if (data.os_family !== undefined) updateData.os_family = data.os_family;
      if (data.os_version !== undefined) updateData.os_version = data.os_version;
      if (data.rbd_image !== undefined) updateData.rbd_image = data.rbd_image;
      if (data.rbd_snapshot !== undefined) updateData.rbd_snapshot = data.rbd_snapshot;
      if (data.min_disk_gb !== undefined) updateData.min_disk_gb = data.min_disk_gb;
      if (data.supports_cloudinit !== undefined) updateData.supports_cloudinit = data.supports_cloudinit;
      if (data.is_active !== undefined) updateData.is_active = data.is_active;
      if (data.sort_order !== undefined) updateData.sort_order = data.sort_order;
      if (data.description !== undefined) updateData.description = data.description;
      if (data.storage_backend !== undefined) updateData.storage_backend = data.storage_backend;
      if (data.file_path !== undefined) updateData.file_path = data.file_path;

      const updated = await adminTemplatesApi.updateTemplate(selectedTemplate.id, updateData);
      setTemplates((prev) => prev.map((t) => (t.id === updated.id ? updated : t)));
      toast({
        title: "Template Updated",
        description: `Template "${updated.name}" has been updated successfully.`,
      });
    } finally {
      setEditSaving(false);
    }
  };

  const handleDelete = async () => {
    if (!selectedTemplate) return;
    setSaving(true);
    try {
      await adminTemplatesApi.deleteTemplate(selectedTemplate.id);
      setTemplates((prev) => prev.filter((t) => t.id !== selectedTemplate.id));
      toast({
        title: "Template Deleted",
        description: `Template "${selectedTemplate.name}" has been permanently deleted.`,
      });
      closeDialog();
    } catch (err) {
      toast({
        title: "Error",
        description: err instanceof Error ? err.message : "Failed to delete template",
        variant: "destructive",
      });
    } finally {
      setSaving(false);
    }
  };

  const handleImport = async () => {
    if (!selectedTemplate) return;
    setSaving(true);
    try {
      await adminTemplatesApi.importTemplate(selectedTemplate.id);
      toast({
        title: "Import Started",
        description: `Template "${selectedTemplate.name}" import has been initiated.`,
      });
      closeDialog();
      // Reload templates to get updated status
      const reloadId = setTimeout(loadTemplates, 2000);
      return () => clearTimeout(reloadId);
    } catch (err) {
      toast({
        title: "Error",
        description: err instanceof Error ? err.message : "Failed to import template",
        variant: "destructive",
      });
    } finally {
      setSaving(false);
    }
  };

  const handleBuildFromISO = async () => {
    const hasISOSource = isoSourceMode === "path" ? buildForm.iso_path : buildForm.iso_url;
    if (!buildForm.name || !hasISOSource || !buildForm.node_id) {
      toast({ title: "Error", description: "Name, ISO source, and Node ID are required", variant: "destructive" });
      return;
    }
    setBuildLoading(true);
    try {
      const payload = {
        ...buildForm,
        iso_path: isoSourceMode === "path" ? buildForm.iso_path : "",
        iso_url: isoSourceMode === "url" ? buildForm.iso_url : "",
      };
      const result = await adminTemplatesApi.buildTemplateFromISO(payload);
      toast({ title: "Build Started", description: `Template build task created: ${result.task_id}` });
      setBuildDialogOpen(false);
      setBuildForm({ ...defaultBuildForm });
      setIsoSourceMode("url");
      setTimeout(() => loadTemplates(), 2000);
    } catch (err) {
      toast({ title: "Error", description: err instanceof Error ? err.message : "Failed to start build", variant: "destructive" });
    } finally {
      setBuildLoading(false);
    }
  };

  const openDistributeDialog = async (template: Template) => {
    setDistributeTemplate(template);
    setSelectedNodeIds([]);
    setDistributeDialogOpen(true);
    try {
      const response = await adminNodesApi.getNodes();
      setAvailableNodes(response.data || []);
    } catch {
      toast({ title: "Error", description: "Failed to load nodes", variant: "destructive" });
    }
  };

  const handleDistribute = async () => {
    if (!distributeTemplate || selectedNodeIds.length === 0) return;
    setDistributeLoading(true);
    try {
      const result = await adminTemplatesApi.distributeTemplate(distributeTemplate.id, selectedNodeIds);
      toast({ title: "Distribution Started", description: `Distribution task created: ${result.task_id}` });
      setDistributeDialogOpen(false);
      setDistributeTemplate(null);
      setSelectedNodeIds([]);
    } catch (err) {
      toast({
        title: "Error",
        description: err instanceof Error ? err.message : "Failed to start distribution",
        variant: "destructive",
      });
    } finally {
      setDistributeLoading(false);
    }
  };

  const openCacheStatusDialog = async (template: Template) => {
    setCacheStatusTemplate(template);
    setCacheStatusDialogOpen(true);
    setCacheStatusLoading(true);
    try {
      const status = await adminTemplatesApi.getTemplateCacheStatus(template.id);
      setCacheEntries(status.entries || []);
    } catch {
      toast({ title: "Error", description: "Failed to load cache status", variant: "destructive" });
    } finally {
      setCacheStatusLoading(false);
    }
  };

  const toggleNodeSelection = (nodeId: string) => {
    setSelectedNodeIds((prev) =>
      prev.includes(nodeId)
        ? prev.filter((id) => id !== nodeId)
        : [...prev, nodeId]
    );
  };

  const activeTemplates = templates.filter((t) => t.status === "active").length;
  const totalTemplates = templates.length;

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <p className="text-destructive">{error}</p>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-background p-6 md:p-8">
      <div className="mx-auto max-w-7xl space-y-6">
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">OS Templates</h1>
            <p className="text-muted-foreground">
              Manage VM templates for OS installation
            </p>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" onClick={() => setBuildDialogOpen(true)}>
              <Disc className="mr-2 h-4 w-4" />
              Build from ISO
            </Button>
            <Button onClick={() => openDialog("create")}>
              <Plus className="mr-2 h-4 w-4" />
              Create Template
            </Button>
          </div>
        </div>

        <Card>
          <CardContent className="pt-6">
            <div className="relative">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Search templates by name or OS family..."
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
              <HardDrive className="h-5 w-5" />
              All Templates
            </CardTitle>
            <CardDescription>
              {filteredTemplates.length} of {totalTemplates} templates displayed
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>OS Family</TableHead>
                  <TableHead>RBD Image</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Created</TableHead>
                  <TableHead className="w-[50px]"></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredTemplates.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={6} className="text-center text-muted-foreground py-8">
                      No templates found
                    </TableCell>
                  </TableRow>
                ) : (
                  filteredTemplates.map((template) => (
                    <TableRow key={template.id}>
                      <TableCell className="font-medium">{template.name}</TableCell>
                      <TableCell className="capitalize">{template.os_family}</TableCell>
                      <TableCell className="font-mono text-sm">
                        {template.rbd_image || "-"}
                      </TableCell>
                      <TableCell>{getStatusBadge(template.status)}</TableCell>
                      <TableCell className="text-muted-foreground">
                        {new Date(template.created_at).toLocaleDateString()}
                      </TableCell>
                      <TableCell>
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button variant="ghost" size="icon">
                              <MoreHorizontal className="h-4 w-4" />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end">
                            <DropdownMenuItem onClick={() => openDialog("edit", template)}>
                              <Pencil className="mr-2 h-4 w-4" />
                              Edit
                            </DropdownMenuItem>
                            <DropdownMenuItem onClick={() => openDialog("import", template)}>
                              <Download className="mr-2 h-4 w-4" />
                              Import
                            </DropdownMenuItem>
                            {hasDistributableSource(template) && (
                              <DropdownMenuItem onClick={() => openDistributeDialog(template)}>
                                <Send className="mr-2 h-4 w-4" />
                                Distribute to Nodes
                              </DropdownMenuItem>
                            )}
                            <DropdownMenuItem onClick={() => openCacheStatusDialog(template)}>
                              <Database className="mr-2 h-4 w-4" />
                              Cache Status
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              className="text-destructive"
                              onClick={() => openDialog("delete", template)}
                            >
                              <Trash2 className="mr-2 h-4 w-4" />
                              Delete
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>

        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          <Card>
            <CardContent className="pt-6">
              <div className="flex items-center gap-4">
                <div className="flex h-12 w-12 items-center justify-center rounded-full bg-blue-500/10">
                  <HardDrive className="h-5 w-5 text-blue-500" />
                </div>
                <div>
                  <div className="text-2xl font-bold">{totalTemplates}</div>
                  <p className="text-xs text-muted-foreground">Total Templates</p>
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
                  <div className="text-2xl font-bold">{activeTemplates}</div>
                  <p className="text-xs text-muted-foreground">Active Templates</p>
                </div>
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="pt-6">
              <div className="flex items-center gap-4">
                <div className="flex h-12 w-12 items-center justify-center rounded-full bg-gray-500/10">
                  <div className="h-3 w-3 rounded-full bg-gray-500" />
                </div>
                <div>
                  <div className="text-2xl font-bold">
                    {totalTemplates - activeTemplates}
                  </div>
                  <p className="text-xs text-muted-foreground">Inactive Templates</p>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>

      {/* Create Dialog */}
      <Dialog
        open={dialogOpen && dialogAction === "create"}
        onOpenChange={(open) => { if (!open) closeDialog(); }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create Template</DialogTitle>
            <DialogDescription>
              Add a new OS template for VM provisioning.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="name">Name</Label>
              <Input
                id="name"
                value={formData.name}
                onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                placeholder="e.g., Ubuntu 24.04 LTS"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="os_family">OS Family</Label>
              <Input
                id="os_family"
                value={formData.os_family}
                onChange={(e) => setFormData({ ...formData, os_family: e.target.value })}
                placeholder="e.g., debian, ubuntu, centos"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="rbd_image">RBD Image (Optional)</Label>
              <Input
                id="rbd_image"
                value={formData.rbd_image}
                onChange={(e) => setFormData({ ...formData, rbd_image: e.target.value })}
                placeholder="e.g., vs-images/ubuntu-24.04"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={closeDialog}>
              Cancel
            </Button>
            <Button
              onClick={handleCreate}
              disabled={saving || !formData.name}
            >
              {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Create
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Edit Dialog */}
      <TemplateEditDialog
        open={editDialogOpen}
        onOpenChange={setEditDialogOpen}
        template={selectedTemplate}
        onSave={handleEditSave}
        isSaving={editSaving}
      />

      {/* Import Dialog */}
      <Dialog
        open={dialogOpen && dialogAction === "import"}
        onOpenChange={(open) => { if (!open) closeDialog(); }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Import Template</DialogTitle>
            <DialogDescription>
              Import the template image from the RBD snapshot to make it available for VM provisioning.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={closeDialog}>
              Cancel
            </Button>
            <Button onClick={handleImport} disabled={saving}>
              {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Import
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Dialog */}
      <Dialog
        open={dialogOpen && dialogAction === "delete"}
        onOpenChange={(open) => { if (!open) closeDialog(); }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Template</DialogTitle>
            <DialogDescription>
              Are you sure you want to permanently delete template &quot;{selectedTemplate?.name}&quot;?
              This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={closeDialog}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDelete} disabled={saving}>
              {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Build from ISO Dialog */}
      <Dialog open={buildDialogOpen} onOpenChange={setBuildDialogOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>Build Template from ISO</DialogTitle>
            <DialogDescription>
              Create a VM template by performing an unattended OS installation from an ISO image.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 max-h-[60vh] overflow-y-auto pr-1">
            <div className="space-y-2">
              <Label htmlFor="build-name">Template Name</Label>
              <Input
                id="build-name"
                value={buildForm.name}
                onChange={(e) => setBuildForm({ ...buildForm, name: e.target.value })}
                placeholder="e.g., Ubuntu 24.04 LTS"
              />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="build-os_family">OS Family</Label>
                <Select
                  value={buildForm.os_family}
                  onValueChange={(value) => setBuildForm({ ...buildForm, os_family: value })}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="ubuntu">Ubuntu</SelectItem>
                    <SelectItem value="debian">Debian</SelectItem>
                    <SelectItem value="almalinux">AlmaLinux</SelectItem>
                    <SelectItem value="rocky">Rocky Linux</SelectItem>
                    <SelectItem value="centos">CentOS</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="build-os_version">OS Version</Label>
                <Input
                  id="build-os_version"
                  value={buildForm.os_version}
                  onChange={(e) => setBuildForm({ ...buildForm, os_version: e.target.value })}
                  placeholder="e.g., 24.04"
                />
              </div>
            </div>
            <div className="space-y-2">
              <Label>ISO Source</Label>
              <div className="flex gap-2 mb-2">
                <Button
                  type="button"
                  variant={isoSourceMode === "url" ? "default" : "outline"}
                  size="sm"
                  onClick={() => { setIsoSourceMode("url"); setBuildForm({ ...buildForm, iso_path: "" }); }}
                >
                  Download from URL
                </Button>
                <Button
                  type="button"
                  variant={isoSourceMode === "path" ? "default" : "outline"}
                  size="sm"
                  onClick={() => { setIsoSourceMode("path"); setBuildForm({ ...buildForm, iso_url: "" }); }}
                >
                  Local Path
                </Button>
              </div>
              {isoSourceMode === "url" ? (
                <Input
                  id="build-iso_url"
                  value={buildForm.iso_url}
                  onChange={(e) => setBuildForm({ ...buildForm, iso_url: e.target.value })}
                  placeholder="https://cdimage.debian.org/debian-cd/current/amd64/iso-cd/debian-12.9.0-amd64-netinst.iso"
                />
              ) : (
                <Input
                  id="build-iso_path"
                  value={buildForm.iso_path}
                  onChange={(e) => setBuildForm({ ...buildForm, iso_path: e.target.value })}
                  placeholder="/var/lib/virtuestack/iso/ubuntu-24.04.iso"
                />
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="build-node_id">Node ID</Label>
              <Input
                id="build-node_id"
                value={buildForm.node_id}
                onChange={(e) => setBuildForm({ ...buildForm, node_id: e.target.value })}
                placeholder="UUID of the target node"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="build-storage_backend">Storage Backend</Label>
              <Select
                value={buildForm.storage_backend}
                onValueChange={(value) => setBuildForm({ ...buildForm, storage_backend: value })}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="qcow">Local QCOW2</SelectItem>
                  <SelectItem value="ceph">Ceph (RBD)</SelectItem>
                  <SelectItem value="lvm">LVM Thin</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="grid grid-cols-3 gap-4">
              <div className="space-y-2">
                <Label htmlFor="build-disk_size_gb">Disk (GB)</Label>
                <Input
                  id="build-disk_size_gb"
                  type="number"
                  value={buildForm.disk_size_gb}
                  onChange={(e) => setBuildForm({ ...buildForm, disk_size_gb: parseInt(e.target.value) || 10 })}
                  min={5}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="build-memory_mb">RAM (MB)</Label>
                <Input
                  id="build-memory_mb"
                  type="number"
                  value={buildForm.memory_mb}
                  onChange={(e) => setBuildForm({ ...buildForm, memory_mb: parseInt(e.target.value) || 2048 })}
                  min={512}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="build-vcpus">vCPUs</Label>
                <Input
                  id="build-vcpus"
                  type="number"
                  value={buildForm.vcpus}
                  onChange={(e) => setBuildForm({ ...buildForm, vcpus: parseInt(e.target.value) || 2 })}
                  min={1}
                  max={8}
                />
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="build-root_password">Root Password (optional)</Label>
              <Input
                id="build-root_password"
                type="password"
                value={buildForm.root_password}
                onChange={(e) => setBuildForm({ ...buildForm, root_password: e.target.value })}
                placeholder="Leave empty for default"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setBuildDialogOpen(false)}>
              Cancel
            </Button>
            <Button
              onClick={handleBuildFromISO}
              disabled={buildLoading || !buildForm.name || !buildForm.node_id || (isoSourceMode === "path" ? !buildForm.iso_path : !buildForm.iso_url)}
            >
              {buildLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Start Build
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Distribute Template Dialog */}
      <Dialog open={distributeDialogOpen} onOpenChange={setDistributeDialogOpen}>
        <DialogContent className="sm:max-w-[500px]">
          <DialogHeader>
            <DialogTitle>Distribute Template</DialogTitle>
            <DialogDescription>
              Push &quot;{distributeTemplate?.name}&quot; to selected QCOW/LVM nodes.
              Ceph nodes access templates from the shared pool automatically.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <Label>Select target nodes:</Label>
            {availableNodes.length === 0 ? (
              <p className="text-sm text-muted-foreground">No nodes available</p>
            ) : (
              <div className="max-h-[300px] overflow-y-auto space-y-2">
                {availableNodes.map((node) => (
                  <div
                    key={node.id}
                    className="flex items-center space-x-3 rounded-md border p-3"
                  >
                    <Checkbox
                      id={`node-${node.id}`}
                      checked={selectedNodeIds.includes(node.id)}
                      onCheckedChange={() => toggleNodeSelection(node.id)}
                    />
                    <label htmlFor={`node-${node.id}`} className="flex-1 cursor-pointer text-sm">
                      <span className="font-medium">{node.hostname}</span>
                      <span className="ml-2 text-muted-foreground">({node.status})</span>
                    </label>
                  </div>
                ))}
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDistributeDialogOpen(false)}>
              Cancel
            </Button>
            <Button
              onClick={handleDistribute}
              disabled={distributeLoading || selectedNodeIds.length === 0}
            >
              {distributeLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Distribute ({selectedNodeIds.length} node{selectedNodeIds.length !== 1 ? "s" : ""})
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Cache Status Dialog */}
      <Dialog open={cacheStatusDialogOpen} onOpenChange={setCacheStatusDialogOpen}>
        <DialogContent className="sm:max-w-[600px]">
          <DialogHeader>
            <DialogTitle>Cache Status</DialogTitle>
            <DialogDescription>
              Template distribution status for &quot;{cacheStatusTemplate?.name}&quot;
              {cacheStatusTemplate?.storage_backend === "ceph" && (
                <span className="block mt-1 text-blue-500">
                  Ceph templates are accessed directly from the shared pool — no per-node caching needed.
                </span>
              )}
              {cacheStatusTemplate && !hasDistributableSource(cacheStatusTemplate) && cacheStatusTemplate.storage_backend !== "ceph" && (
                <span className="block mt-1 text-amber-500">
                  Distribution is only available for templates with a controller-accessible HTTP(S) source URL.
                </span>
              )}
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            {cacheStatusLoading ? (
              <div className="flex justify-center py-8">
                <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
              </div>
            ) : cacheEntries.length === 0 ? (
              <p className="text-center text-sm text-muted-foreground py-8">
                No cache entries found. Template has not been distributed to any nodes yet.
              </p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Node</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Size</TableHead>
                    <TableHead>Synced</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {cacheEntries.map((entry) => (
                    <TableRow key={entry.node_id}>
                      <TableCell className="font-mono text-sm">{entry.node_id.slice(0, 8)}...</TableCell>
                      <TableCell>
                        <Badge variant={
                          entry.status === "ready" ? "default" :
                          entry.status === "downloading" ? "secondary" :
                          entry.status === "failed" ? "destructive" : "outline"
                        }>
                          {entry.status}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        {entry.size_bytes
                          ? `${(entry.size_bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`
                          : "-"}
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {entry.synced_at
                          ? new Date(entry.synced_at).toLocaleString()
                          : "-"}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCacheStatusDialogOpen(false)}>
              Close
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
