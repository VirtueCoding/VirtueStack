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

export function formatBytes(bytes: number): string {
  if (bytes <= 0) return "0 Bytes";
  const k = 1024;
  const sizes = ["Bytes", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  const clampedIndex = Math.min(i, sizes.length - 1);
  return parseFloat((bytes / Math.pow(k, clampedIndex)).toFixed(2)) + " " + sizes[clampedIndex];
}
