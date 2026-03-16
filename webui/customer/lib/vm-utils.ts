import type { VM } from "@/lib/api-client";

export function getStatusBadgeVariant(
  status: VM["status"]
): "success" | "secondary" | "destructive" | "warning" {
  switch (status) {
    case "running":
      return "success";
    case "stopped":
      return "secondary";
    case "error":
      return "destructive";
    case "provisioning":
      return "warning";
    default:
      return "warning";
  }
}

export function getStatusLabel(status: string): string {
  return status.charAt(0).toUpperCase() + status.slice(1);
}

export function formatMemory(mb: number): string {
  if (mb >= 1024) {
    return `${(mb / 1024).toFixed(1)} GB`;
  }
  return `${mb} MB`;
}
