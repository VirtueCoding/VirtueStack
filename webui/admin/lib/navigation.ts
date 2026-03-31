import {
  LayoutDashboard,
  Server,
  Monitor,
  FileText,
  Network,
  Users,
  ShieldCheck,
  HardDrive,
  Calendar,
  Database,
  GitBranch,
  KeyRound,
  CreditCard,
} from "lucide-react";
import { LucideIcon } from "lucide-react";

export interface NavItem {
  href: string;
  label: string;
  icon: LucideIcon;
}

export const adminNavItems: NavItem[] = [
  { href: "/dashboard", label: "Dashboard", icon: LayoutDashboard },
  { href: "/vms", label: "VMs", icon: Monitor },
  { href: "/nodes", label: "Nodes", icon: Server },
  { href: "/storage-backends", label: "Storage", icon: HardDrive },
  { href: "/failover-requests", label: "Failover", icon: GitBranch },
  { href: "/plans", label: "Plans", icon: FileText },
  { href: "/templates", label: "Templates", icon: HardDrive },
  { href: "/ip-sets", label: "IP Sets", icon: Network },
  { href: "/customers", label: "Customers", icon: Users },
  { href: "/billing", label: "Billing", icon: CreditCard },
  { href: "/backups", label: "Backups", icon: Database },
  { href: "/backup-schedules", label: "Schedules", icon: Calendar },
  { href: "/provisioning-keys", label: "Provisioning Keys", icon: KeyRound },
  { href: "/audit-logs", label: "Audit Logs", icon: ShieldCheck },
];
