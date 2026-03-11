"use client";

import { useParams, useRouter } from "next/navigation";
import { ArrowLeft, Server, Cpu, HardDrive, MemoryStick, Network, Play, Square, RotateCw, Zap } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

interface VMDetail {
  id: string;
  name: string;
  hostname: string;
  status: "running" | "stopped" | "error";
  ipv4: string;
  ipv6: string;
  vcpu: number;
  memory_mb: number;
  disk_gb: number;
  os_template: string;
  created_at: string;
}

// Mock data - will be replaced with API calls later
const mockVMs: Record<string, VMDetail> = {
  "vm-001": {
    id: "vm-001",
    name: "web-server-prod",
    hostname: "web-prod-01.internal",
    status: "running",
    ipv4: "10.0.1.15",
    ipv6: "fd00::1:15",
    vcpu: 4,
    memory_mb: 8192,
    disk_gb: 100,
    os_template: "Ubuntu 22.04 LTS",
    created_at: "2025-11-15T10:30:00Z",
  },
  "vm-002": {
    id: "vm-002",
    name: "db-server-prod",
    hostname: "db-prod-01.internal",
    status: "running",
    ipv4: "10.0.1.20",
    ipv6: "fd00::1:20",
    vcpu: 8,
    memory_mb: 16384,
    disk_gb: 500,
    os_template: "Ubuntu 22.04 LTS",
    created_at: "2025-10-20T14:15:00Z",
  },
  "vm-003": {
    id: "vm-003",
    name: "api-staging",
    hostname: "api-staging-01.internal",
    status: "stopped",
    ipv4: "10.0.2.10",
    ipv6: "fd00::2:10",
    vcpu: 2,
    memory_mb: 4096,
    disk_gb: 50,
    os_template: "Debian 12",
    created_at: "2025-12-01T09:00:00Z",
  },
  "vm-004": {
    id: "vm-004",
    name: "test-environment",
    hostname: "test-01.internal",
    status: "error",
    ipv4: "10.0.3.5",
    ipv6: "fd00::3:5",
    vcpu: 2,
    memory_mb: 2048,
    disk_gb: 30,
    os_template: "CentOS 9",
    created_at: "2025-11-28T16:45:00Z",
  },
};

function getStatusBadgeVariant(
  status: VMDetail["status"]
): "success" | "secondary" | "destructive" {
  switch (status) {
    case "running":
      return "success";
    case "stopped":
      return "secondary";
    case "error":
      return "destructive";
  }
}

function getStatusLabel(status: VMDetail["status"]): string {
  return status.charAt(0).toUpperCase() + status.slice(1);
}

function formatMemory(mb: number): string {
  if (mb >= 1024) {
    return `${(mb / 1024).toFixed(1)} GB`;
  }
  return `${mb} MB`;
}

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

export default function VMDetailPage() {
  const params = useParams();
  const router = useRouter();
  const vmId = params.id as string;
  const vm = mockVMs[vmId];

  const handleBack = () => {
    router.push("/vms");
  };

  const handleStart = () => {
    console.log(`Starting VM: ${vmId}`);
    // TODO: Implement API call
  };

  const handleStop = () => {
    console.log(`Stopping VM: ${vmId}`);
    // TODO: Implement API call
  };

  const handleForceStop = () => {
    console.log(`Force stopping VM: ${vmId}`);
    // TODO: Implement API call
  };

  const handleRestart = () => {
    console.log(`Restarting VM: ${vmId}`);
    // TODO: Implement API call
  };

  if (!vm) {
    return (
      <div className="flex min-h-[400px] flex-col items-center justify-center">
        <Server className="h-16 w-16 text-muted-foreground" />
        <h2 className="mt-4 text-xl font-semibold">VM Not Found</h2>
        <p className="mt-2 text-muted-foreground">
          The virtual machine you&apos;re looking for doesn&apos;t exist.
        </p>
        <Button onClick={handleBack} className="mt-4">
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back to VMs
        </Button>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Breadcrumb & Header */}
      <div className="flex items-center gap-4">
        <Button variant="ghost" onClick={handleBack} className="h-8 px-2">
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back
        </Button>
        <div className="flex-1">
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-bold tracking-tight">{vm.name}</h1>
            <Badge variant={getStatusBadgeVariant(vm.status)}>
              {getStatusLabel(vm.status)}
            </Badge>
          </div>
          <p className="text-sm text-muted-foreground">{vm.hostname}</p>
        </div>
      </div>

      {/* Control Panel */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">VM Controls</CardTitle>
          <CardDescription>
            Manage the power state of your virtual machine
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap gap-3">
            {vm.status === "stopped" && (
              <Button onClick={handleStart}>
                <Play className="mr-2 h-4 w-4" />
                Start
              </Button>
            )}
            {vm.status === "running" && (
              <>
                <Button variant="outline" onClick={handleStop}>
                  <Square className="mr-2 h-4 w-4" />
                  Stop
                </Button>
                <Button variant="outline" onClick={handleForceStop}>
                  <Zap className="mr-2 h-4 w-4" />
                  Force Stop
                </Button>
                <Button variant="outline" onClick={handleRestart}>
                  <RotateCw className="mr-2 h-4 w-4" />
                  Restart
                </Button>
              </>
            )}
            {vm.status === "error" && (
              <>
                <Button variant="outline" onClick={handleRestart}>
                  <RotateCw className="mr-2 h-4 w-4" />
                  Restart
                </Button>
                <Button variant="outline" onClick={handleForceStop}>
                  <Zap className="mr-2 h-4 w-4" />
                  Force Stop
                </Button>
              </>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Resource Cards */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">vCPU</CardTitle>
            <Cpu className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{vm.vcpu}</div>
            <p className="text-xs text-muted-foreground">Virtual CPUs</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Memory</CardTitle>
            <MemoryStick className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{formatMemory(vm.memory_mb)}</div>
            <p className="text-xs text-muted-foreground">RAM allocated</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Disk</CardTitle>
            <HardDrive className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{vm.disk_gb} GB</div>
            <p className="text-xs text-muted-foreground">Storage</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">OS</CardTitle>
            <Server className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-sm font-semibold">{vm.os_template}</div>
            <p className="text-xs text-muted-foreground">
              Created {formatDate(vm.created_at)}
            </p>
          </CardContent>
        </Card>
      </div>

      {/* Network Information */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <Network className="h-5 w-5" />
            <CardTitle className="text-lg">Network</CardTitle>
          </div>
          <CardDescription>IP addresses assigned to this VM</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid gap-4 md:grid-cols-2">
            <div>
              <h4 className="text-sm font-medium text-muted-foreground">IPv4 Address</h4>
              <p className="mt-1 font-mono text-lg">{vm.ipv4}</p>
            </div>
            <div>
              <h4 className="text-sm font-medium text-muted-foreground">IPv6 Address</h4>
              <p className="mt-1 font-mono text-lg">{vm.ipv6}</p>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Tabs for Console, Backups, Snapshots, Settings */}
      <Tabs defaultValue="console" className="w-full">
        <TabsList className="grid w-full grid-cols-4">
          <TabsTrigger value="console">Console</TabsTrigger>
          <TabsTrigger value="backups">Backups</TabsTrigger>
          <TabsTrigger value="snapshots">Snapshots</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>
        <TabsContent value="console">
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">VM Console</CardTitle>
              <CardDescription>
                Access the virtual machine console
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex min-h-[300px] flex-col items-center justify-center rounded-lg border border-dashed bg-muted/50">
                <Server className="h-12 w-12 text-muted-foreground" />
                <p className="mt-4 text-sm text-muted-foreground">
                  Console access placeholder
                </p>
                <p className="text-xs text-muted-foreground">
                  Full console implementation coming in Phase 5.4.1
                </p>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="backups">
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">Backups</CardTitle>
              <CardDescription>
                Manage automated and manual backups
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex min-h-[200px] flex-col items-center justify-center rounded-lg border border-dashed bg-muted/50">
                <HardDrive className="h-12 w-12 text-muted-foreground" />
                <p className="mt-4 text-sm text-muted-foreground">
                  Backup management placeholder
                </p>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="snapshots">
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">Snapshots</CardTitle>
              <CardDescription>
                Create and restore VM snapshots
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex min-h-[200px] flex-col items-center justify-center rounded-lg border border-dashed bg-muted/50">
                <Server className="h-12 w-12 text-muted-foreground" />
                <p className="mt-4 text-sm text-muted-foreground">
                  Snapshot management placeholder
                </p>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="settings">
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">VM Settings</CardTitle>
              <CardDescription>
                Configure virtual machine parameters
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex min-h-[200px] flex-col items-center justify-center rounded-lg border border-dashed bg-muted/50">
                <Server className="h-12 w-12 text-muted-foreground" />
                <p className="mt-4 text-sm text-muted-foreground">
                  Settings configuration placeholder
                </p>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
