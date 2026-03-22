"use client";

// Re-export usePermissions from the context for convenient imports
// This allows components to import from either:
// - import { usePermissions } from "@/hooks/usePermissions"
// - import { usePermissions } from "@/contexts/PermissionContext"
export { usePermissions, PERMISSION_GROUPS } from "@/contexts/PermissionContext";