"use client";

import { useState } from "react";
import { Play, Square, RotateCw, Plus, Server } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

interface VM {
  id: string;
  name: string;
  hostname: string;
  status: "running" | "stopped" | "error" | "provisioning";
  ipv4: string;
  vcpu: number;
  memory_mb: number;
  disk_gb: number;
}

// Mock data - will be replaced with API calls later
const mockVMs: VM[] = [
  {
    id: "vm-001",
    name: "web-server-prod",
    hostname: "web-prod-01.internal",
    status: "running",
    ipv4: "10.0.1.15",
    vcpu: 4,
    memory_mb: 8192,
    disk_gb: 100,
  },
  {
    id: "vm-002",
    name: "db-server-prod",
    hostname: "db-prod-01.internal",
    status: "running",
    ipv4: "10.0.1.20",
    vcpu: 8,
    memory_mb: 16384,
    disk_gb: 500,
  },
  {
    id: "vm-003",
    name: "api-staging",
    hostname: "api-staging-01.internal",
    status: "stopped",
    ipv4: "10.0.2.10",
    vcpu: 2,
    memory_mb: 4096,
    disk_gb: 50,
  },
  {
    id: "vm-004",
    name: "test-environment",
    hostname: "test-01.internal",
    status: "error",
    ipv4: "10.0.3.5",
    vcpu: 2,
    memory_mb: 2048,
    disk_gb: 30,
  },
  {
    id: "vm-005",
    name: "analytics-worker",
    hostname: "analytics-01.internal",
    status: "provisioning",
    ipv4: "10.0.1.25",
    vcpu: 4,
    memory_mb: 8192,
    disk_gb: 200,
  },
];

function getStatusBadgeVariant(
  status: VM["status"]
): "success" | "secondary" | "destructive" | "warning" {
  switch (status) {
    case "running":
      return "success";
    case "stopped":
      return "secondary";
    case "error":
      return "destructive";
    case "provisioning":
      return "warning";
  }
}

function getStatusLabel(status: VM["status"]): string {
  return status.charAt(0).toUpperCase() + status.slice(1);
}

function formatMemory(mb: number): string {
  if (mb >= 1024) {
    return `${(mb / 1024).toFixed(1)} GB`;
  }
  return `${mb} MB`;
}

export default function VMsPage() {
  const [vms, setVms] = useState<VM[]>(mockVMs);
  const [isLoading, setIsLoading] = useState(false);

  const handleStart = (id: string) => {
    console.log(`Starting VM: ${id}`);
    // TODO: Implement API call
  };

  const handleStop = (id: string) => {
    console.log(`Stopping VM: ${id}`);
    // TODO: Implement API call
  };

  const handleRestart = (id: string) => {
    console.log(`Restarting VM: ${id}`);
    // TODO: Implement API call
  };

  const handleCreateVM = () => {
    console.log("Create new VM");
    // TODO: Implement navigation to create VM page
  };

  if (isLoading) {
    return (
      <div className="flex min-h-[400px] items-center justify-center">
        <div className="text-center">
          <Server className="mx-auto h-12 w-12 animate-pulse text-primary" />
          <p className="mt-4 text-muted-foreground">Loading virtual machines...</p>
        </div>
      </div>
    );
  }

  if (vms.length === 0) {
    return (
      <Card>
        <CardHeader className="text-center">
          <Server className="mx-auto h-16 w-16 text-muted-foreground" />
          <CardTitle className="mt-4">No Virtual Machines</CardTitle>
          <CardDescription>
            You don&apos;t have any virtual machines yet. Create your first VM to get
            started.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex justify-center">
            <Button onClick={handleCreateVM}>
              <Plus className="mr-2 h-4 w-4" />
              Create VM
            </Button>
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>Virtual Machines</CardTitle>
            <CardDescription>
              Manage and monitor your virtual machines
            </CardDescription>
          </div>
          <Button onClick={handleCreateVM}>
            <Plus className="mr-2 h-4 w-4" />
            Create VM
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>IP Address</TableHead>
              <TableHead>Resources</TableHead>
              <TableHead className="text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {vms.map((vm) => (
              <TableRow key={vm.id}>
                <TableCell className="font-medium">
                  <div>
                    <div>{vm.name}</div>
                    <div className="text-xs text-muted-foreground">
                      {vm.hostname}
                    </div>
                  </div>
                </TableCell>
                <TableCell>
                  <Badge variant={getStatusBadgeVariant(vm.status)}>
                    {getStatusLabel(vm.status)}
                  </Badge>
                </TableCell>
                <TableCell className="font-mono text-sm">{vm.ipv4}</TableCell>
                <TableCell>
                  <div className="text-sm">
                    <div>{vm.vcpu} vCPU</div>
                    <div>{formatMemory(vm.memory_mb)}</div>
                    <div className="text-muted-foreground">{vm.disk_gb} GB</div>
                  </div>
                </TableCell>
                <TableCell className="text-right">
                  <div className="flex justify-end gap-2">
                    {vm.status === "stopped" && (
                      <Button
                        variant="outline"
                        size="icon"
                        onClick={() => handleStart(vm.id)}
                        title="Start VM"
                      >
                        <Play className="h-4 w-4" />
                      </Button>
                    )}
                    {vm.status === "running" && (
                      <>
                        <Button
                          variant="outline"
                          size="icon"
                          onClick={() => handleStop(vm.id)}
                          title="Stop VM"
                        >
                          <Square className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="outline"
                          size="icon"
                          onClick={() => handleRestart(vm.id)}
                          title="Restart VM"
                        >
                          <RotateCw className="h-4 w-4" />
                        </Button>
                      </>
                    )}
                    {vm.status === "error" && (
                      <Button
                        variant="outline"
                        size="icon"
                        onClick={() => handleRestart(vm.id)}
                        title="Restart VM"
                      >
                        <RotateCw className="h-4 w-4" />
                      </Button>
                    )}
                    {vm.status === "provisioning" && (
                      <Button variant="outline" size="icon" disabled>
                        <RotateCw className="h-4 w-4 animate-spin" />
                      </Button>
                    )}
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}
