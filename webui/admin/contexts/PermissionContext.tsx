"use client";

import {
  createContext,
  useContext,
  useCallback,
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
];

export function PermissionProvider({ children }: { children: ReactNode }) {
  const { user, isAuthenticated, isLoading: authIsLoading } = useAuth();
  const permissions = isAuthenticated && user ? user.permissions || [] : [];
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
    (permissions: string[]): boolean => {
      // Super admins always have all permissions
      if (isSuperAdmin) return true;
      return permissions.some((p) => permissions.includes(p));
    },
    [permissions, isSuperAdmin]
  );

  const hasAllPermissions = useCallback(
    (permissions: string[]): boolean => {
      // Super admins always have all permissions
      if (isSuperAdmin) return true;
      return permissions.every((p) => permissions.includes(p));
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
