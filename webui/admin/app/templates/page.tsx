"use client";

import { useState, useEffect } from "react";
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
import { Label } from "@/components/ui/label";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { HardDrive, Plus, Search, Loader2, Pencil, Trash2, Download, MoreHorizontal } from "lucide-react";
import { adminTemplatesApi, type Template, type UpdateTemplateRequest } from "@/lib/api-client";
import { useToast } from "@/components/ui/use-toast";
import { getStatusBadgeVariant } from "@/lib/status-badge";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { TemplateEditDialog, type EditTemplateFormData } from "@/components/templates/TemplateEditDialog";

type DialogAction = "create" | "edit" | "delete" | "import" | null;

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
  const { toast } = useToast();

  useEffect(() => {
    loadTemplates();
  }, []);

  async function loadTemplates() {
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
  }

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
      setTimeout(loadTemplates, 2000);
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
          <Button onClick={() => openDialog("create")}>
            <Plus className="mr-2 h-4 w-4" />
            Create Template
          </Button>
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
    </div>
  );
}