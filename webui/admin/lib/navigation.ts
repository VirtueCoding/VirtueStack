import {
  LayoutDashboard,
  Monitor,
  Server,
  HardDrive,
  GitBranch,
  FileText,
  Network,
  Users,
  CreditCard,
  Receipt,
  Database,
  Calendar,
  KeyRound,
  ShieldCheck,
  Layers,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";

export interface NavItem {
  href: string;
  label: string;
  icon: LucideIcon;
}

export interface NavGroup {
  label: string;
  items: NavItem[];
}

export const adminNavGroups: NavGroup[] = [
  {
    label: "Overview",
    items: [
      { href: "/dashboard", label: "Dashboard", icon: LayoutDashboard },
    ],
  },
  {
    label: "Infrastructure",
    items: [
      { href: "/nodes", label: "Nodes", icon: Server },
      { href: "/storage-backends", label: "Storage", icon: HardDrive },
      { href: "/failover-requests", label: "Failover", icon: GitBranch },
      { href: "/templates", label: "Templates", icon: Layers },
    ],
  },
  {
    label: "Virtual Machines",
    items: [
      { href: "/vms", label: "VMs", icon: Monitor },
      { href: "/plans", label: "Plans", icon: FileText },
      { href: "/ip-sets", label: "IP Sets", icon: Network },
    ],
  },
  {
    label: "Customers",
    items: [
      { href: "/customers", label: "Customers", icon: Users },
      { href: "/billing", label: "Billing", icon: CreditCard },
      { href: "/invoices", label: "Invoices", icon: Receipt },
    ],
  },
  {
    label: "System",
    items: [
      { href: "/backups", label: "Backups", icon: Database },
      { href: "/backup-schedules", label: "Schedules", icon: Calendar },
      { href: "/provisioning-keys", label: "Provisioning Keys", icon: KeyRound },
      { href: "/audit-logs", label: "Audit Logs", icon: ShieldCheck },
    ],
  },
];
