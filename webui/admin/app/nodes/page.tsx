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
  Eye,
  ArrowDownToLine,
  RefreshCcw,
  MoreVertical,
} from "lucide-react";

interface Node {
  id: string;
  name: string;
  hostname: string;
  status: "online" | "offline" | "draining" | "failed";
  location: string;
  vm_count: number;
  cpu_total: number;
  cpu_allocated: number;
  memory_total_gb: number;
  memory_allocated_gb: number;
}

const mockNodes: Node[] = [
  {
    id: "1",
    name: "node-prod-01",
    hostname: "hypervisor-01.virtuestack.local",
    status: "online",
    location: "US-East-1",
    vm_count: 24,
    cpu_total: 64,
    cpu_allocated: 48,
    memory_total_gb: 256,
    memory_allocated_gb: 192,
  },
  {
    id: "2",
    name: "node-prod-02",
    hostname: "hypervisor-02.virtuestack.local",
    status: "online",
    location: "US-East-1",
    vm_count: 18,
    cpu_total: 64,
    cpu_allocated: 32,
    memory_total_gb: 256,
    memory_allocated_gb: 128,
  },
  {
    id: "3",
    name: "node-prod-03",
    hostname: "hypervisor-03.virtuestack.local",
    status: "draining",
    location: "US-West-2",
    vm_count: 8,
    cpu_total: 48,
    cpu_allocated: 16,
    memory_total_gb: 192,
    memory_allocated_gb: 64,
  },
  {
    id: "4",
    name: "node-dev-01",
    hostname: "hypervisor-dev-01.virtuestack.local",
    status: "online",
    location: "EU-Central-1",
    vm_count: 32,
    cpu_total: 32,
    cpu_allocated: 28,
    memory_total_gb: 128,
    memory_allocated_gb: 112,
  },
  {
    id: "5",
    name: "node-dev-02",
    hostname: "hypervisor-dev-02.virtuestack.local",
    status: "offline",
    location: "EU-Central-1",
    vm_count: 0,
    cpu_total: 32,
    cpu_allocated: 0,
    memory_total_gb: 128,
    memory_allocated_gb: 0,
  },
  {
    id: "6",
    name: "node-staging-01",
    hostname: "hypervisor-staging-01.virtuestack.local",
    status: "failed",
    location: "AP-South-1",
    vm_count: 12,
    cpu_total: 48,
    cpu_allocated: 24,
    memory_total_gb: 192,
    memory_allocated_gb: 96,
  },
  {
    id: "7",
    name: "node-backup-01",
    hostname: "hypervisor-backup-01.virtuestack.local",
    status: "online",
    location: "US-East-1",
    vm_count: 6,
    cpu_total: 24,
    cpu_allocated: 8,
    memory_total_gb: 96,
    memory_allocated_gb: 32,
  },
  {
    id: "8",
    name: "node-gpu-01",
    hostname: "hypervisor-gpu-01.virtuestack.local",
    status: "online",
    location: "US-West-2",
    vm_count: 4,
    cpu_total: 32,
    cpu_allocated: 16,
    memory_total_gb: 512,
    memory_allocated_gb: 256,
  },
];

function getStatusBadge(status: Node["status"]) {
  const variants = {
    online: "success" as const,
    offline: "destructive" as const,
    draining: "warning" as const,
    failed: "destructive" as const,
  };

  const labels = {
    online: "Online",
    offline: "Offline",
    draining: "Draining",
    failed: "Failed",
  };

  return (
    <Badge variant={variants[status]}>{labels[status]}</Badge>
  );
}

function getResourceUsage(current: number, total: number, type: "cpu" | "memory") {
  const percentage = total > 0 ? (current / total) * 100 : 0;
  const unit = type === "cpu" ? "vCPU" : "GB";
  
  return (
    <div className="space-y-1">
      <div className="flex justify-between text-xs">
        <span className="text-muted-foreground">
          {current} / {total} {unit}
        </span>
        <span className="text-muted-foreground">{percentage.toFixed(0)}%</span>
      </div>
      <div className="h-2 w-full overflow-hidden rounded-full bg-secondary">
        <div
          className={`h-full transition-all ${
            percentage > 90
              ? "bg-red-500"
              : percentage > 70
              ? "bg-yellow-500"
              : "bg-green-500"
          }`}
          style={{ width: `${percentage}%` }}
        />
      </div>
    </div>
  );
}

export default function NodesPage() {
  const [searchTerm, setSearchTerm] = useState("");
  const [nodes] = useState<Node[]>(mockNodes);

  const filteredNodes = nodes.filter(
    (node) =>
      node.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      node.hostname.toLowerCase().includes(searchTerm.toLowerCase()) ||
      node.location.toLowerCase().includes(searchTerm.toLowerCase())
  );

  const handleView = (node: Node) => {
    console.log("View node:", node);
    // TODO: Implement view action
  };

  const handleDrain = (node: Node) => {
    if (window.confirm(`Are you sure you want to drain node "${node.name}"? This will migrate all VMs to other nodes.`)) {
      console.log("Drain node:", node);
      // TODO: Implement drain action
    }
  };

  const handleFailover = (node: Node) => {
    if (window.confirm(`Are you sure you want to initiate failover for node "${node.name}"? This will trigger HA recovery.`)) {
      console.log("Failover node:", node);
      // TODO: Implement failover action
    }
  };

  return (
    <div className="min-h-screen bg-background p-6 md:p-8">
      <div className="mx-auto max-w-7xl space-y-6">
        {/* Header */}
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">Compute Nodes</h1>
            <p className="text-muted-foreground">
              Manage and monitor hypervisor nodes
            </p>
          </div>
          <Button size="default">
            <Plus className="mr-2 h-4 w-4" />
            Add Node
          </Button>
        </div>

        {/* Search and Filter */}
        <Card>
          <CardContent className="pt-6">
            <div className="relative">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Search by name, hostname, or location..."
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
                className="pl-10"
              />
            </div>
          </CardContent>
        </Card>

        {/* Nodes Table */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Server className="h-5 w-5" />
              All Nodes
            </CardTitle>
            <CardDescription>
              {filteredNodes.length} of {nodes.length} nodes displayed
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Location</TableHead>
                    <TableHead className="text-center">VMs</TableHead>
                    <TableHead>CPU Usage</TableHead>
                    <TableHead>Memory Usage</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredNodes.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={7} className="h-24 text-center">
                        No nodes found
                      </TableCell>
                    </TableRow>
                  ) : (
                    filteredNodes.map((node) => (
                      <TableRow key={node.id}>
                        <TableCell>
                          <div className="space-y-1">
                            <div className="font-medium">{node.name}</div>
                            <div className="text-xs text-muted-foreground">
                              {node.hostname}
                            </div>
                          </div>
                        </TableCell>
                        <TableCell>{getStatusBadge(node.status)}</TableCell>
                        <TableCell className="text-muted-foreground">
                          {node.location}
                        </TableCell>
                        <TableCell className="text-center">
                          <Badge variant="secondary">{node.vm_count}</Badge>
                        </TableCell>
                        <TableCell className="w-[180px]">
                          {getResourceUsage(node.cpu_allocated, node.cpu_total, "cpu")}
                        </TableCell>
                        <TableCell className="w-[180px]">
                          {getResourceUsage(node.memory_allocated_gb, node.memory_total_gb, "memory")}
                        </TableCell>
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-2">
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => handleView(node)}
                            >
                              <Eye className="h-3 w-3" />
                              View
                            </Button>
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => handleDrain(node)}
                              disabled={node.status === "offline" || node.status === "failed"}
                            >
                              <ArrowDownToLine className="h-3 w-3" />
                              Drain
                            </Button>
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => handleFailover(node)}
                              disabled={node.status === "online" || node.status === "draining"}
                            >
                              <RefreshCcw className="h-3 w-3" />
                              Failover
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
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <Card>
            <CardContent className="pt-6">
              <div className="flex items-center gap-4">
                <div className="flex h-12 w-12 items-center justify-center rounded-full bg-green-500/10">
                  <div className="h-3 w-3 rounded-full bg-green-500" />
                </div>
                <div>
                  <div className="text-2xl font-bold">
                    {nodes.filter((n) => n.status === "online").length}
                  </div>
                  <p className="text-xs text-muted-foreground">Online</p>
                </div>
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="pt-6">
              <div className="flex items-center gap-4">
                <div className="flex h-12 w-12 items-center justify-center rounded-full bg-yellow-500/10">
                  <div className="h-3 w-3 rounded-full bg-yellow-500" />
                </div>
                <div>
                  <div className="text-2xl font-bold">
                    {nodes.filter((n) => n.status === "draining").length}
                  </div>
                  <p className="text-xs text-muted-foreground">Draining</p>
                </div>
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="pt-6">
              <div className="flex items-center gap-4">
                <div className="flex h-12 w-12 items-center justify-center rounded-full bg-red-500/10">
                  <div className="h-3 w-3 rounded-full bg-red-500" />
                </div>
                <div>
                  <div className="text-2xl font-bold">
                    {nodes.filter((n) => n.status === "offline" || n.status === "failed").length}
                  </div>
                  <p className="text-xs text-muted-foreground">Offline / Failed</p>
                </div>
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="pt-6">
              <div className="flex items-center gap-4">
                <div className="flex h-12 w-12 items-center justify-center rounded-full bg-blue-500/10">
                  <Server className="h-5 w-5 text-blue-500" />
                </div>
                <div>
                  <div className="text-2xl font-bold">
                    {nodes.reduce((acc, n) => acc + n.vm_count, 0)}
                  </div>
                  <p className="text-xs text-muted-foreground">Total VMs</p>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
