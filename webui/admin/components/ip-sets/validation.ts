import { z } from "zod";

// Custom Zod refinements for IP validation
const ipv4CidrRegex = /^(\d{1,3}\.){3}\d{1,3}\/(\d{1,2})$/;
// IPv6 CIDR: accepts any valid IPv6 address prefix (full or compressed) followed by /prefix-length
const ipv6CidrRegex = /^(([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}|([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}|([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}|([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})|:((:[0-9a-fA-F]{1,4}){1,7}|:))\/(\d{1,3})$/;
const ipv4Regex = /^(\d{1,3}\.){3}\d{1,3}$/;
// IPv6: RFC-compliant pattern accepting full and compressed forms (including :: loopback/any)
const ipv6Regex = /^(([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}|([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}|([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}|([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})|:((:[0-9a-fA-F]{1,4}){1,7}|:))$/;

const isValidIPv4Octet = (ip: string): boolean => {
  const parts = ip.split(".").map(Number);
  return parts.length === 4 && parts.every((part) => part >= 0 && part <= 255);
};

const isValidIPv4Address = (ip: string): boolean => ipv4Regex.test(ip) && isValidIPv4Octet(ip);

const isValidIPv4Cidr = (value: string): boolean => {
  if (!ipv4CidrRegex.test(value)) {
    return false;
  }

  const [ip, prefix] = value.split("/");
  const prefixNum = parseInt(prefix, 10);
  return prefixNum >= 1 && prefixNum <= 32 && isValidIPv4Address(ip);
};

export const extractValidImportAddresses = (lines: readonly string[]): string[] => lines.flatMap((line) => {
  const trimmed = line.trim();
  if (!trimmed || trimmed.toLowerCase().startsWith("ip") || trimmed.startsWith("#")) {
    return [];
  }

  const address = trimmed.split(",")[0].trim();
  if (isValidIPv4Address(address) || isValidIPv4Cidr(address) || ipv6Regex.test(address) || ipv6CidrRegex.test(address)) {
    return [address];
  }

  return [];
});

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
      if (prefixNum < 1 || prefixNum > 32 || !isValidIPv4Address(ip)) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: "Invalid IPv4 CIDR format (e.g., 10.0.0.0/24)",
          path: ["network"],
        });
      }
    }
    // Validate gateway for IPv4
    if (!isValidIPv4Address(data.gateway)) {
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
    if (!ipv6Regex.test(data.gateway)) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "Invalid IPv6 address",
        path: ["gateway"],
      });
    }
  }
});

export type CreateIPSetFormData = z.infer<typeof createIPSetSchema>;

// Edit IP Set schema - fields are optional for partial updates
export const editIPSetSchema = z.object({
  name: z.string().min(1, "Name is required").max(100, "Name must be less than 100 characters").optional(),
  gateway: z.string().optional(),
  vlan_id: z.number().int().min(1, "VLAN ID must be between 1 and 4094").max(4094, "VLAN ID must be between 1 and 4094").optional().nullable(),
  location_id: z.string().optional().nullable(),
  node_ids: z.array(z.string()).optional(),
}).superRefine((data, ctx) => {
  // Only validate gateway if provided
  if (data.gateway && data.gateway.length > 0) {
    // Try IPv4 first
    const isIPv4 = isValidIPv4Address(data.gateway);
    // Then try IPv6 using the RFC-compliant regex (covers ::, ::1, and all compressed forms)
    const isIPv6 = ipv6Regex.test(data.gateway);

    if (!isIPv4 && !isIPv6) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "Invalid IP address format",
        path: ["gateway"],
      });
    }
  }
});

export type EditIPSetFormData = z.infer<typeof editIPSetSchema>;