import { z } from "zod";

// Custom Zod refinements for IP validation
const ipv4CidrRegex = /^(\d{1,3}\.){3}\d{1,3}\/(\d{1,2})$/;
const ipv6CidrRegex = /^([0-9a-fA-F:]+)\/(\d{1,3})$/;
const ipv4Regex = /^(\d{1,3}\.){3}\d{1,3}$/;
const ipv6Regex = /^([0-9a-fA-F]{0,4}:){2,7}[0-9a-fA-F]{0,4}$/;

const isValidIPv4Octet = (ip: string): boolean => {
  const parts = ip.split(".").map(Number);
  return parts.every((part) => part >= 0 && part <= 255);
};

export const createIPSetSchema = z.object({
  name: z.string().min(1, "Name is required").max(100, "Name must be less than 100 characters"),
  network: z.string().min(1, "Network CIDR is required"),
  gateway: z.string().min(1, "Gateway is required"),
  ip_version: z.union([z.literal(4), z.literal(6)]),
  location_id: z.string().optional(),
  vlan_id: z.number().int().min(1).max(4094).optional(),
  node_ids: z.array(z.string()).optional(),
}).superRefine((data, ctx) => {
  // Validate network CIDR based on IP version
  if (data.ip_version === 4) {
    if (!ipv4CidrRegex.test(data.network)) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "Invalid IPv4 CIDR format (e.g., 10.0.0.0/24)",
        path: ["network"],
      });
    } else {
      const [ip, prefix] = data.network.split("/");
      const prefixNum = parseInt(prefix, 10);
      if (prefixNum < 1 || prefixNum > 32 || !isValidIPv4Octet(ip)) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: "Invalid IPv4 CIDR format (e.g., 10.0.0.0/24)",
          path: ["network"],
        });
      }
    }
    // Validate gateway for IPv4
    if (!ipv4Regex.test(data.gateway) || !isValidIPv4Octet(data.gateway)) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "Invalid IPv4 address",
        path: ["gateway"],
      });
    }
  } else {
    if (!ipv6CidrRegex.test(data.network)) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "Invalid IPv6 CIDR format (e.g., 2001:db8::/32)",
        path: ["network"],
      });
    } else {
      const [, prefix] = data.network.split("/");
      const prefixNum = parseInt(prefix, 10);
      if (prefixNum < 1 || prefixNum > 128) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: "Invalid IPv6 CIDR format (e.g., 2001:db8::/32)",
          path: ["network"],
        });
      }
    }
    // Validate gateway for IPv6
    if (!ipv6Regex.test(data.gateway) && data.gateway !== "::" && !data.gateway.includes("::")) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "Invalid IPv6 address",
        path: ["gateway"],
      });
    }
  }
});

export type CreateIPSetFormData = z.infer<typeof createIPSetSchema>;