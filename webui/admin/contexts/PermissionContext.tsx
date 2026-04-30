"use client";

import {
  createContext,
  useContext,
  useCallback,
  useMemo,
  ReactNode,
} from "react";
import { useAuth } from "@/lib/auth-context";

interface PermissionState {
  permissions: string[];
  isLoading: boolean;
}

interface PermissionContextType extends PermissionState {
  hasPermission: (permission: string) => boolean;
  hasAnyPermission: (permissions: string[]) => boolean;
  hasAllPermissions: (permissions: string[]) => boolean;
  isSuperAdmin: boolean;
}

const PermissionContext = createContext<PermissionContextType | undefined>(undefined);

// Permission definition interface
interface PermissionDefinition {
  name: string;
  description: string;
}

interface PermissionGroup {
  resource: string;
  label: string;
  permissions: PermissionDefinition[];
}

// Permission definitions grouped by resource for UI display
export const PERMISSION_GROUPS: PermissionGroup[] = [
  {
    resource: "plans",
    label: "Plans",
    permissions: [
      { name: "plans:read", description: "View plans" },
      { name: "plans:write", description: "Create and update plans" },
      { name: "plans:delete", description: "Delete plans" },
    ],
  },
  {
    resource: "nodes",
    label: "Nodes",
    permissions: [
      { name: "nodes:read", description: "View nodes" },
      { name: "nodes:write", description: "Create and update nodes" },
      { name: "nodes:delete", description: "Delete nodes" },
    ],
  },
  {
    resource: "customers",
    label: "Customers",
    permissions: [
      { name: "customers:read", description: "View customers" },
      { name: "customers:write", description: "Update customer accounts" },
      { name: "customers:delete", description: "Delete customers" },
    ],
  },
  {
    resource: "vms",
    label: "Virtual Machines",
    permissions: [
      { name: "vms:read", description: "View VMs" },
      { name: "vms:write", description: "Create and modify VMs" },
      { name: "vms:delete", description: "Delete VMs" },
    ],
  },
  {
    resource: "settings",
    label: "Settings",
    permissions: [
      { name: "settings:read", description: "View settings" },
      { name: "settings:write", description: "Modify settings" },
    ],
  },
  {
    resource: "backups",
    label: "Backups",
    permissions: [
      { name: "backups:read", description: "View backups" },
      { name: "backups:write", description: "Manage backups" },
    ],
  },
  {
    resource: "ipsets",
    label: "IP Sets",
    permissions: [
      { name: "ipsets:read", description: "View IP sets" },
      { name: "ipsets:write", description: "Create and update IP sets" },
      { name: "ipsets:delete", description: "Delete IP sets" },
    ],
  },
  {
    resource: "templates",
    label: "Templates",
    permissions: [
      { name: "templates:read", description: "View templates" },
      { name: "templates:write", description: "Manage templates" },
    ],
  },
  {
    resource: "rdns",
    label: "RDNS",
    permissions: [
      { name: "rdns:read", description: "View RDNS records" },
      { name: "rdns:write", description: "Manage RDNS records" },
    ],
  },
  {
    resource: "audit_logs",
    label: "Audit Logs",
    permissions: [
      { name: "audit_logs:read", description: "View audit logs" },
    ],
  },
  {
    resource: "storage_backends",
    label: "Storage Backends",
    permissions: [
      { name: "storage_backends:read", description: "View storage backends" },
      { name: "storage_backends:write", description: "Create and update storage backends" },
      { name: "storage_backends:delete", description: "Delete storage backends" },
    ],
  },
  {
    resource: "billing",
    label: "Billing",
    permissions: [
      { name: "billing:read", description: "View billing records" },
      { name: "billing:write", description: "Manage billing records" },
    ],
  },
];

export function PermissionProvider({ children }: { children: ReactNode }) {
  const { user, isAuthenticated, isLoading: authIsLoading } = useAuth();
  const permissions = useMemo(
    () => (isAuthenticated && user ? user.permissions || [] : []),
    [isAuthenticated, user]
  );
  const isLoading = authIsLoading;

  const isSuperAdmin = user?.role === "super_admin";

  const hasPermission = useCallback(
    (permission: string): boolean => {
      // Super admins always have all permissions
      if (isSuperAdmin) return true;
      return permissions.includes(permission);
    },
    [permissions, isSuperAdmin]
  );

  const hasAnyPermission = useCallback(
    (requiredPermissions: string[]): boolean => {
      // Super admins always have all permissions
      if (isSuperAdmin) return true;
      return requiredPermissions.some((permission) => permissions.includes(permission));
    },
    [permissions, isSuperAdmin]
  );

  const hasAllPermissions = useCallback(
    (requiredPermissions: string[]): boolean => {
      // Super admins always have all permissions
      if (isSuperAdmin) return true;
      return requiredPermissions.every((permission) => permissions.includes(permission));
    },
    [permissions, isSuperAdmin]
  );

  const value: PermissionContextType = {
    permissions,
    isLoading,
    hasPermission,
    hasAnyPermission,
    hasAllPermissions,
    isSuperAdmin,
  };

  return (
    <PermissionContext.Provider value={value}>
      {children}
    </PermissionContext.Provider>
  );
}

export function usePermissions(): PermissionContextType {
  const context = useContext(PermissionContext);
  if (context === undefined) {
    throw new Error("usePermissions must be used within a PermissionProvider");
  }
  return context;
}
