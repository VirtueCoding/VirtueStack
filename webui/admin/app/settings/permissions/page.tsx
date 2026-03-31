"use client";

import { useState, useEffect, useCallback } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@virtuestack/ui";
import { Button } from "@virtuestack/ui";
import { Badge } from "@virtuestack/ui";
import { Checkbox } from "@/components/ui/checkbox";
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
import { Shield, Loader2, Edit, Save } from "lucide-react";
import { adminPermissionsApi, type Admin } from "@/lib/api-client";
import { useToast } from "@virtuestack/ui";
import { usePermissions, PERMISSION_GROUPS } from "@/hooks/usePermissions";
import { useRouter } from "next/navigation";

export default function PermissionsPage() {
  const { toast } = useToast();
  const router = useRouter();
  const { isSuperAdmin, isLoading: permissionsLoading } = usePermissions();
  const [admins, setAdmins] = useState<Admin[]>([]);
  const [loading, setLoading] = useState(true);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [selectedAdmin, setSelectedAdmin] = useState<Admin | null>(null);
  const [editingPermissions, setEditingPermissions] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);

  const loadData = useCallback(async () => {
    try {
      const adminsData = await adminPermissionsApi.getAdmins();
      setAdmins(adminsData || []);
    } catch (error) {
      toast({
        title: "Error",
        description: "Failed to load admin permissions.",
        variant: "destructive",
      });
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    // Wait until identity is resolved before redirecting
    if (permissionsLoading) return;
    if (!isSuperAdmin) {
      router.push("/settings");
      return;
    }
    loadData();
  }, [loadData, isSuperAdmin, permissionsLoading, router]);

  const handleEdit = (admin: Admin) => {
    setSelectedAdmin(admin);
    setEditingPermissions(admin.permissions || []);
    setEditDialogOpen(true);
  };

  const handleTogglePermission = (permissionName: string) => {
    setEditingPermissions((prev) =>
      prev.includes(permissionName)
        ? prev.filter((p) => p !== permissionName)
        : [...prev, permissionName]
    );
  };

  const handleToggleResource = (resource: string, checked: boolean) => {
    const group = PERMISSION_GROUPS.find((g) => g.resource === resource);
    if (!group) return;

    const resourcePermissions = group.permissions.map((p) => p.name);

    if (checked) {
      // Add all permissions for this resource
      setEditingPermissions((prev) => {
        const newPerms = new Set(prev);
        resourcePermissions.forEach((p) => newPerms.add(p));
        return Array.from(newPerms);
      });
    } else {
      // Remove all permissions for this resource
      setEditingPermissions((prev) =>
        prev.filter((p) => !resourcePermissions.includes(p))
      );
    }
  };

  const handleSave = async () => {
    if (!selectedAdmin) return;

    setSaving(true);
    try {
      const updated = await adminPermissionsApi.updateAdminPermissions(
        selectedAdmin.id,
        editingPermissions
      );
      setAdmins((prev) =>
        prev.map((a) => (a.id === updated.id ? { ...a, ...updated } : a))
      );
      toast({
        title: "Permissions Updated",
        description: `Permissions for ${selectedAdmin.email} have been updated.`,
      });
      setEditDialogOpen(false);
      setSelectedAdmin(null);
    } catch (error) {
      toast({
        title: "Update Failed",
        description: error instanceof Error ? error.message : "Failed to update permissions",
        variant: "destructive",
      });
    } finally {
      setSaving(false);
    }
  };

  const getRoleBadgeVariant = (role: string) => {
    switch (role) {
      case "super_admin":
        return "default";
      case "admin":
        return "secondary";
      default:
        return "outline";
    }
  };

  const getPermissionCount = (admin: Admin) => {
    if (admin.role === "super_admin") return "All";
    if (!admin.permissions || admin.permissions.length === 0) {
      return "Default";
    }
    return admin.permissions.length.toString();
  };

  if (!isSuperAdmin) {
    return null;
  }

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-background p-6 md:p-8">
      <div className="mx-auto max-w-7xl space-y-8">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Permission Management</h1>
          <p className="text-muted-foreground">
            Manage fine-grained permissions for admin users
          </p>
        </div>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Shield className="h-5 w-5" />
              Admin Permissions
            </CardTitle>
            <CardDescription>
              View and manage permissions for all admin accounts. Super admins have all permissions automatically.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Email</TableHead>
                    <TableHead>Name</TableHead>
                    <TableHead>Role</TableHead>
                    <TableHead>2FA</TableHead>
                    <TableHead>Permissions</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {admins.map((admin) => (
                    <TableRow key={admin.id}>
                      <TableCell className="font-medium">{admin.email}</TableCell>
                      <TableCell>{admin.name || "—"}</TableCell>
                      <TableCell>
                        <Badge variant={getRoleBadgeVariant(admin.role) as React.ComponentProps<typeof Badge>["variant"]}>
                          {admin.role}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        {admin.totp_enabled ? (
                          <Badge variant="outline" className="bg-green-50 text-green-700 border-green-200 dark:bg-green-950 dark:text-green-300 dark:border-green-800">
                            Enabled
                          </Badge>
                        ) : (
                          <Badge variant="outline" className="bg-gray-50 text-gray-500 border-gray-200 dark:bg-gray-950 dark:text-gray-400 dark:border-gray-800">
                            Disabled
                          </Badge>
                        )}
                      </TableCell>
                      <TableCell>
                        <span className="text-sm">{getPermissionCount(admin)}</span>
                      </TableCell>
                      <TableCell className="text-right">
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => handleEdit(admin)}
                          title="Edit permissions"
                        >
                          <Edit className="h-4 w-4" />
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Edit Permissions Dialog */}
      <Dialog open={editDialogOpen} onOpenChange={setEditDialogOpen}>
        <DialogContent className="max-w-2xl max-h-[80vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Edit Permissions</DialogTitle>
            <DialogDescription>
              Modify permissions for {selectedAdmin?.email}
              {selectedAdmin?.role === "super_admin" && (
                <span className="block mt-2 text-sm text-yellow-600 dark:text-yellow-400">
                  Note: Super admins automatically have all permissions.
                </span>
              )}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-6 py-4">
            {PERMISSION_GROUPS.map((group) => {
              const resourcePermissions = group.permissions.map((p) => p.name);
              const allSelected = resourcePermissions.every((p) =>
                editingPermissions.includes(p)
              );
              const someSelected = resourcePermissions.some((p) =>
                editingPermissions.includes(p)
              );

              return (
                <div key={group.resource} className="space-y-3">
                  <div className="flex items-center gap-2">
                    <Checkbox
                      id={`resource-${group.resource}`}
                      checked={allSelected}
                      ref={(el: HTMLButtonElement | null) => {
                        if (el) {
                          (el as HTMLButtonElement & { indeterminate: boolean }).indeterminate = someSelected && !allSelected;
                        }
                      }}
                      onCheckedChange={(checked: boolean | "indeterminate") =>
                        handleToggleResource(group.resource, checked === true)
                      }
                      disabled={selectedAdmin?.role === "super_admin"}
                    />
                    <label
                      htmlFor={`resource-${group.resource}`}
                      className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70"
                    >
                      {group.label}
                    </label>
                  </div>
                  <div className="ml-6 grid grid-cols-1 sm:grid-cols-2 gap-2">
                    {group.permissions.map((permission) => (
                      <div key={permission.name} className="flex items-center gap-2">
                        <Checkbox
                          id={permission.name}
                          checked={editingPermissions.includes(permission.name)}
                          onCheckedChange={() => handleTogglePermission(permission.name)}
                          disabled={selectedAdmin?.role === "super_admin"}
                        />
                        <label
                          htmlFor={permission.name}
                          className="text-sm text-muted-foreground peer-disabled:cursor-not-allowed peer-disabled:opacity-70"
                        >
                          {permission.description}
                        </label>
                      </div>
                    ))}
                  </div>
                </div>
              );
            })}
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setEditDialogOpen(false)}>
              Cancel
            </Button>
            <Button onClick={handleSave} disabled={saving || selectedAdmin?.role === "super_admin"}>
              {saving ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <Save className="mr-2 h-4 w-4" />
              )}
              Save Changes
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}