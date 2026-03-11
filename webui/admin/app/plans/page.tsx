"use client";

import { useState } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Server,
  Plus,
  Search,
  Edit,
  Trash2,
  Cpu,
  MemoryStick,
  HardDrive,
  Network,
  DollarSign,
} from "lucide-react";

interface Plan {
  id: string;
  name: string;
  vcpu: number;
  memory_mb: number;
  disk_gb: number;
  bandwidth_mbps: number;
  price_monthly: number;
  status: "active" | "inactive";
}

const mockPlans: Plan[] = [
  {
    id: "1",
    name: "Starter VM",
    vcpu: 1,
    memory_mb: 1024,
    disk_gb: 20,
    bandwidth_mbps: 100,
    price_monthly: 9.99,
    status: "active",
  },
  {
    id: "2",
    name: "Developer",
    vcpu: 2,
    memory_mb: 4096,
    disk_gb: 50,
    bandwidth_mbps: 500,
    price_monthly: 24.99,
    status: "active",
  },
  {
    id: "3",
    name: "Business Pro",
    vcpu: 4,
    memory_mb: 8192,
    disk_gb: 100,
    bandwidth_mbps: 1000,
    price_monthly: 49.99,
    status: "active",
  },
  {
    id: "4",
    name: "Enterprise",
    vcpu: 8,
    memory_mb: 16384,
    disk_gb: 250,
    bandwidth_mbps: 5000,
    price_monthly: 99.99,
    status: "active",
  },
  {
    id: "5",
    name: "Memory Optimized",
    vcpu: 4,
    memory_mb: 32768,
    disk_gb: 100,
    bandwidth_mbps: 2000,
    price_monthly: 79.99,
    status: "active",
  },
  {
    id: "6",
    name: "CPU Powerhouse",
    vcpu: 16,
    memory_mb: 32768,
    disk_gb: 500,
    bandwidth_mbps: 10000,
    price_monthly: 149.99,
    status: "active",
  },
  {
    id: "7",
    name: "Legacy Basic",
    vcpu: 1,
    memory_mb: 512,
    disk_gb: 10,
    bandwidth_mbps: 50,
    price_monthly: 4.99,
    status: "inactive",
  },
  {
    id: "8",
    name: "Storage Heavy",
    vcpu: 2,
    memory_mb: 4096,
    disk_gb: 1000,
    bandwidth_mbps: 500,
    price_monthly: 59.99,
    status: "inactive",
  },
];

function getStatusBadge(status: Plan["status"]) {
  const variants = {
    active: "success" as const,
    inactive: "secondary" as const,
  };

  const labels = {
    active: "Active",
    inactive: "Inactive",
  };

  return <Badge variant={variants[status]}>{labels[status]}</Badge>;
}

function formatPrice(price: number) {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
  }).format(price);
}

function formatMemory(mb: number) {
  if (mb >= 1024) {
    return `${(mb / 1024).toFixed(0)} GB`;
  }
  return `${mb} MB`;
}

export default function PlansPage() {
  const [searchTerm, setSearchTerm] = useState("");
  const [plans] = useState<Plan[]>(mockPlans);

  const filteredPlans = plans.filter((plan) =>
    plan.name.toLowerCase().includes(searchTerm.toLowerCase())
  );

  const handleEdit = (plan: Plan) => {
    console.log("Edit plan:", plan);
    // TODO: Implement edit action
  };

  const handleDelete = (plan: Plan) => {
    if (
      window.confirm(
        `Are you sure you want to delete plan "${plan.name}"? This cannot be undone.`
      )
    ) {
      console.log("Delete plan:", plan);
      // TODO: Implement delete action
    }
  };

  const activePlans = plans.filter((p) => p.status === "active").length;
  const totalPlans = plans.length;

  return (
    <div className="min-h-screen bg-background p-6 md:p-8">
      <div className="mx-auto max-w-7xl space-y-6">
        {/* Header */}
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">VM Plans</h1>
            <p className="text-muted-foreground">
              Manage pricing tiers and VM specifications
            </p>
          </div>
          <Button size="default">
            <Plus className="mr-2 h-4 w-4" />
            Create Plan
          </Button>
        </div>

        {/* Search and Filter */}
        <Card>
          <CardContent className="pt-6">
            <div className="relative">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Search plans by name..."
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
                className="pl-10"
              />
            </div>
          </CardContent>
        </Card>

        {/* Plans Table */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Server className="h-5 w-5" />
              All Plans
            </CardTitle>
            <CardDescription>
              {filteredPlans.length} of {totalPlans} plans displayed
            </CardDescription>
          </CardHeader>
          <CardContent>
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
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredPlans.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={8} className="h-24 text-center">
                        No plans found
                      </TableCell>
                    </TableRow>
                  ) : (
                    filteredPlans.map((plan) => (
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
                            <span>{plan.bandwidth_mbps} Mbps</span>
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2 font-semibold text-foreground">
                            <DollarSign className="h-4 w-4 text-muted-foreground" />
                            <span>{formatPrice(plan.price_monthly)}</span>
                          </div>
                        </TableCell>
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-2">
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => handleEdit(plan)}
                            >
                              <Edit className="h-3 w-3" />
                              Edit
                            </Button>
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => handleDelete(plan)}
                            >
                              <Trash2 className="h-3 w-3" />
                              Delete
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>
          </CardContent>
        </Card>

        {/* Summary Stats */}
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          <Card>
            <CardContent className="pt-6">
              <div className="flex items-center gap-4">
                <div className="flex h-12 w-12 items-center justify-center rounded-full bg-blue-500/10">
                  <Server className="h-5 w-5 text-blue-500" />
                </div>
                <div>
                  <div className="text-2xl font-bold">{totalPlans}</div>
                  <p className="text-xs text-muted-foreground">Total Plans</p>
                </div>
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="pt-6">
              <div className="flex items-center gap-4">
                <div className="flex h-12 w-12 items-center justify-center rounded-full bg-green-500/10">
                  <div className="h-3 w-3 rounded-full bg-green-500" />
                </div>
                <div>
                  <div className="text-2xl font-bold">{activePlans}</div>
                  <p className="text-xs text-muted-foreground">Active Plans</p>
                </div>
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="pt-6">
              <div className="flex items-center gap-4">
                <div className="flex h-12 w-12 items-center justify-center rounded-full bg-gray-500/10">
                  <div className="h-3 w-3 rounded-full bg-gray-500" />
                </div>
                <div>
                  <div className="text-2xl font-bold">
                    {totalPlans - activePlans}
                  </div>
                  <p className="text-xs text-muted-foreground">Inactive Plans</p>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
