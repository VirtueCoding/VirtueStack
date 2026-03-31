"use client";

import { Badge } from "@virtuestack/ui";
import { Button } from "@virtuestack/ui";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@virtuestack/ui";
import { MapPin, Eye, Pencil } from "lucide-react";

export interface IPSetDisplay {
  id: string;
  name: string;
  type: "ipv4" | "ipv6";
  location: string;
  total_ips: number;
  available_ips: number;
  cidr: string;
}

function getTypeBadge(type: IPSetDisplay["type"]) {
  const variants = {
    ipv4: "default" as const,
    ipv6: "secondary" as const,
  };

  return (
    <Badge variant={variants[type]} className="font-mono">
      {type.toUpperCase()}
    </Badge>
  );
}

function getUsagePercentage(available: number, total: number) {
  if (total === 0) return 0;
  return ((total - available) / total) * 100;
}

function getUsageColor(percentage: number) {
  if (percentage > 90) return "bg-red-500";
  if (percentage > 70) return "bg-yellow-500";
  return "bg-green-500";
}

function formatNumber(num: number): string {
  if (num >= 1000000) return (num / 1000000).toFixed(1) + "M";
  if (num >= 1000) return (num / 1000).toFixed(1) + "K";
  return num.toString();
}

interface IPSetListProps {
  ipSets: IPSetDisplay[];
  onView?: (ipSet: IPSetDisplay) => void;
  onEdit?: (ipSet: IPSetDisplay) => void;
}

export function IPSetList({ ipSets, onView, onEdit }: IPSetListProps) {
  if (ipSets.length === 0) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        No IP sets found
      </div>
    );
  }

  return (
    <div className="overflow-x-auto">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Type</TableHead>
            <TableHead>Location</TableHead>
            <TableHead>CIDR</TableHead>
            <TableHead>Total IPs</TableHead>
            <TableHead>Available IPs</TableHead>
            <TableHead>Usage</TableHead>
            <TableHead className="text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {ipSets.map((ipSet) => {
            const usagePercentage = getUsagePercentage(ipSet.available_ips, ipSet.total_ips);
            return (
              <TableRow key={ipSet.id}>
                <TableCell>
                  <div className="font-medium">{ipSet.name}</div>
                </TableCell>
                <TableCell>{getTypeBadge(ipSet.type)}</TableCell>
                <TableCell>
                  <div className="flex items-center gap-2 text-muted-foreground">
                    <MapPin className="h-3 w-3" />
                    {ipSet.location}
                  </div>
                </TableCell>
                <TableCell className="font-mono text-xs text-muted-foreground">
                  {ipSet.cidr}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {formatNumber(ipSet.total_ips)}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {formatNumber(ipSet.available_ips)}
                </TableCell>
                <TableCell className="w-[180px]">
                  <div className="space-y-1">
                    <div className="flex justify-between text-xs">
                      <span className="text-muted-foreground">
                        {usagePercentage.toFixed(0)}% used
                      </span>
                    </div>
                    <div className="h-2 w-full overflow-hidden rounded-full bg-secondary">
                      <div
                        className={`h-full transition-all ${getUsageColor(usagePercentage)}`}
                        style={{ width: `${usagePercentage}%` }}
                      />
                    </div>
                  </div>
                </TableCell>
                <TableCell className="text-right">
                  <div className="flex justify-end gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => onView?.(ipSet)}
                      disabled={!onView}
                    >
                      <Eye className="h-3 w-3 mr-1" />
                      View
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => onEdit?.(ipSet)}
                      disabled={!onEdit}
                    >
                      <Pencil className="h-3 w-3 mr-1" />
                      Edit
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </div>
  );
}