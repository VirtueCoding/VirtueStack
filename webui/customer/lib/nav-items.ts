import { Monitor, Settings } from "lucide-react";

export const navItems = [
  { href: "/vms", label: "My VMs", icon: Monitor },
  { href: "/settings", label: "Settings", icon: Settings },
] as const;