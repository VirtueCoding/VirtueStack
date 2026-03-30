"use client";

import { Button } from "@virtuestack/ui";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@virtuestack/ui";
import { Cpu, MemoryStick, HardDrive, Network, DollarSign, Camera, Archive, Disc, Edit, Trash2 } from "lucide-react";
import type { Plan } from "@/lib/api-client";

interface PlanListProps {
  plans: Plan[];
  onEdit: (plan: Plan) => void;
  onDelete: (plan: Plan) => void;
  getStatusBadge: (status: Plan["status"]) => React.ReactNode;
  formatMemory: (mb: number) => string;
  formatPrice: (cents: number) => string;
  canWrite?: boolean;
  canDelete?: boolean;
}

export function PlanList({ plans, onEdit, onDelete, getStatusBadge, formatMemory, formatPrice, canWrite = true, canDelete = true }: PlanListProps) {
  if (plans.length === 0) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        No plans found
      </div>
    );
  }

  return (
    <div className="overflow-x-auto">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Status</TableHead>
            <TableHead>vCPU</TableHead>
            <TableHead>Memory</TableHead>
            <TableHead>Disk</TableHead>
            <TableHead>Bandwidth</TableHead>
            <TableHead>Price/Month</TableHead>
            <TableHead className="text-center">Snapshots</TableHead>
            <TableHead className="text-center">Backups</TableHead>
            <TableHead className="text-center">ISOs</TableHead>
            <TableHead className="text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {plans.map((plan) => (
            <TableRow key={plan.id}>
              <TableCell>
                <div className="font-medium">{plan.name}</div>
              </TableCell>
              <TableCell>{getStatusBadge(plan.status)}</TableCell>
              <TableCell>
                <div className="flex items-center gap-2">
                  <Cpu className="h-4 w-4 text-muted-foreground" />
                  <span>{plan.vcpu}</span>
                </div>
              </TableCell>
              <TableCell>
                <div className="flex items-center gap-2">
                  <MemoryStick className="h-4 w-4 text-muted-foreground" />
                  <span>{formatMemory(plan.memory_mb)}</span>
                </div>
              </TableCell>
              <TableCell>
                <div className="flex items-center gap-2">
                  <HardDrive className="h-4 w-4 text-muted-foreground" />
                  <span>{plan.disk_gb} GB</span>
                </div>
              </TableCell>
              <TableCell>
                <div className="flex items-center gap-2">
                  <Network className="h-4 w-4 text-muted-foreground" />
                  <span>{plan.port_speed_mbps} Mbps</span>
                </div>
              </TableCell>
              <TableCell>
                <div className="flex items-center gap-2 font-semibold text-foreground">
                  <DollarSign className="h-4 w-4 text-muted-foreground" />
                  <span>{formatPrice(plan.price_monthly)}</span>
                </div>
              </TableCell>
              <TableCell className="text-center">
                <div className="flex items-center justify-center gap-1">
                  <Camera className="h-3.5 w-3.5 text-muted-foreground" />
                  <span>{plan.snapshot_limit ?? 2}</span>
                </div>
              </TableCell>
              <TableCell className="text-center">
                <div className="flex items-center justify-center gap-1">
                  <Archive className="h-3.5 w-3.5 text-muted-foreground" />
                  <span>{plan.backup_limit ?? 2}</span>
                </div>
              </TableCell>
              <TableCell className="text-center">
                <div className="flex items-center justify-center gap-1">
                  <Disc className="h-3.5 w-3.5 text-muted-foreground" />
                  <span>{plan.iso_upload_limit ?? 2}</span>
                </div>
              </TableCell>
              <TableCell className="text-right">
                <div className="flex justify-end gap-2">
                  {canWrite && (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => onEdit(plan)}
                    >
                      <Edit className="mr-1 h-3 w-3" />
                      Edit
                    </Button>
                  )}
                  {canDelete && (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => onDelete(plan)}
                      className="text-destructive hover:bg-destructive/10"
                    >
                      <Trash2 className="mr-1 h-3 w-3" />
                      Delete
                    </Button>
                  )}
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}