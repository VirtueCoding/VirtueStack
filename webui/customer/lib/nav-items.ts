import { Monitor, Settings, CreditCard } from "lucide-react";

export const navItems = [
  { href: "/vms", label: "My VMs", icon: Monitor },
  { href: "/billing", label: "Billing", icon: CreditCard },
  { href: "/settings", label: "Settings", icon: Settings },
] as const;