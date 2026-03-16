export type BadgeVariant = "default" | "secondary" | "destructive" | "outline" | "success" | "warning";

export function getStatusBadgeVariant(status: string): BadgeVariant {
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
