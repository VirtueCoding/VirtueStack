import {
  LayoutDashboard,
  Server,
  Monitor,
  FileText,
  Network,
  Users,
  ShieldCheck,
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
  { href: "/plans", label: "Plans", icon: FileText },
  { href: "/ip-sets", label: "IP Sets", icon: Network },
  { href: "/customers", label: "Customers", icon: Users },
  { href: "/audit-logs", label: "Audit Logs", icon: ShieldCheck },
];
