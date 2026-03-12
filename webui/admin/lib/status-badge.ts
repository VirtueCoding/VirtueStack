/**
 * Maps entity status strings to Badge component variant names.
 * Shared across admin pages (nodes, customers, etc.)
 */
export function getStatusBadgeVariant(status: string): string {
  switch (status) {
    case "online":
    case "active":
    case "running":
      return "success";
    case "draining":
    case "warning":
    case "provisioning":
      return "warning";
    case "offline":
    case "failed":
    case "error":
    case "suspended":
      return "destructive";
    default:
      return "secondary";
  }
}
