"use client";

import { useParams, useRouter } from "next/navigation";
import { useState, useEffect, useCallback } from "react";
import {
  ArrowLeft,
  Server,
  Cpu,
  HardDrive,
  MemoryStick,
  Network,
  Monitor,
  BarChart3,
} from "lucide-react";
import { Badge } from "@virtuestack/ui";
import { Button } from "@virtuestack/ui";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@virtuestack/ui";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useToast } from "@virtuestack/ui";
import { ResourceCharts } from "@/components/charts/resource-charts";
import { SerialConsole } from "@/components/serial-console/serial-console";
import { vmApi, backupApi, snapshotApi, VM, Backup, Snapshot, ApiClientError } from "@/lib/api-client";
import { getStatusBadgeVariant, getStatusLabel, formatMemory } from "@/lib/vm-utils";
import { useVMAction } from "@/lib/hooks/useVMAction";
import { VMControls } from "@/components/vm/VMControls";
import { VMBackupsTab } from "@/components/vm/VMBackupsTab";
import { VMSnapshotsTab } from "@/components/vm/VMSnapshotsTab";
import { VMSettingsTab } from "@/components/vm/VMSettingsTab";
import { VMConsoleTab } from "@/components/vm/VMConsoleTab";
import { VMISOTab } from "@/components/vm/VMISOTab";
import { VMRDNSTab } from "@/components/vm/VMRDNSTab";

function SerialConsoleWithToken({ vmId, vmName }: { vmId: string; vmName: string }) {
  const [token, setToken] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    async function fetchToken() {
      try {
        const response = await vmApi.getSerialToken(vmId);
        if (!cancelled) {
          setToken(response.token);
          setIsLoading(false);
        }
      } catch (err) {
        if (!cancelled) {
          const message = err instanceof ApiClientError
            ? err.message
            : "Failed to get serial console access token.";
          setError(message);
          setIsLoading(false);
        }
      }
    }
    fetchToken();
    return () => { cancelled = true; };
  }, [vmId]);

  if (isLoading) {
    return (
      <div className="flex min-h-[400px] items-center justify-center rounded-lg border border-dashed bg-muted/50">
        <Server className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex min-h-[400px] flex-col items-center justify-center rounded-lg border border-dashed bg-muted/50">
        <p className="text-sm text-destructive">{error}</p>
      </div>
    );
  }

  return <SerialConsole vmId={vmId} vmName={vmName} token={token || undefined} />;
}

export default function VMDetailPage() {
  const params = useParams();
  const router = useRouter();
  const rawId = params.id;
  const vmId = Array.isArray(rawId) ? rawId[0] : (rawId as string);
  const [vm, setVm] = useState<VM | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [showStopDialog, setShowStopDialog] = useState(false);
  const [showForceStopDialog, setShowForceStopDialog] = useState(false);
  const { toast } = useToast();
  const { executeAction: executeVMAction, isLoading: isVMActionLoading } = useVMAction();

  // Backup state
  const [backups, setBackups] = useState<Backup[]>([]);
  const [isBackupsLoading, setIsBackupsLoading] = useState(false);
  const [backupMethodFilter, setBackupMethodFilter] = useState<"all" | "full" | "snapshot">("all");

  // Snapshot state
  const [snapshots, setSnapshots] = useState<Snapshot[]>([]);
  const [isSnapshotsLoading, setIsSnapshotsLoading] = useState(false);

  const fetchVM = useCallback(async () => {
    if (!vmId) return;
    try {
      const data = await vmApi.getVM(vmId);
      setVm(data);
    } catch (error) {
      toast({
        title: "Error",
        description: "Failed to load virtual machine details.",
        variant: "destructive",
      });
    } finally {
      setIsLoading(false);
    }
  }, [vmId, toast]);

  const fetchBackups = useCallback(async () => {
    if (!vmId) return;
    setIsBackupsLoading(true);
    try {
      const data = await backupApi.listBackups(vmId);
      setBackups(data);
    } catch (error) {
      toast({
        title: "Error",
        description: "Failed to load backups.",
        variant: "destructive",
      });
    } finally {
      setIsBackupsLoading(false);
    }
  }, [vmId, toast]);

  const fetchSnapshots = useCallback(async () => {
    if (!vmId) return;
    setIsSnapshotsLoading(true);
    try {
      const data = await snapshotApi.listSnapshots(vmId);
      setSnapshots(data);
    } catch (error) {
      toast({
        title: "Error",
        description: "Failed to load snapshots.",
        variant: "destructive",
      });
    } finally {
      setIsSnapshotsLoading(false);
    }
  }, [vmId, toast]);

  useEffect(() => {
    if (!vmId) {
      router.push('/vms');
      return;
    }
    fetchVM();
    fetchBackups();
    fetchSnapshots();
  // eslint-disable-next-line react-hooks/exhaustive-deps -- Intentionally only depend on vmId
  }, [vmId]);

  const handleBack = () => {
    router.push("/vms");
  };

  const handleStart = () => {
    if (!vmId) return;
    executeVMAction({
      action: "start",
      vmId,
      onSuccess: fetchVM,
    });
  };

  const handleStop = () => {
    if (!vmId) return;
    setShowStopDialog(false);
    executeVMAction({
      action: "stop",
      vmId,
      onSuccess: fetchVM,
    });
  };

  const handleForceStop = () => {
    if (!vmId) return;
    setShowForceStopDialog(false);
    executeVMAction({
      action: "forceStop",
      vmId,
      onSuccess: fetchVM,
    });
  };

  const handleRestart = () => {
    if (!vmId) return;
    executeVMAction({
      action: "restart",
      vmId,
      onSuccess: fetchVM,
    });
  };

  // Backup handlers
  const handleCreateBackup = async (name: string) => {
    if (!vmId) return;
    await backupApi.createBackup({ vm_id: vmId, name });
    toast({
      title: "Backup Created",
      description: "Backup creation initiated successfully.",
    });
    await fetchBackups();
  };

  const handleDeleteBackup = async (backupId: string) => {
    await backupApi.deleteBackup(backupId);
    toast({
      title: "Backup Deleted",
      description: "Backup deleted successfully.",
    });
    await fetchBackups();
  };

  const handleRestoreBackup = async (backupId: string) => {
    await backupApi.restoreBackup(backupId);
    toast({
      title: "Backup Restore Initiated",
      description: "The VM will be restored from the selected backup.",
    });
  };

  // Snapshot handlers
  const handleCreateSnapshot = async (name: string) => {
    if (!vmId) return;
    await snapshotApi.createSnapshot({ vm_id: vmId, name });
    toast({
      title: "Snapshot Created",
      description: "Snapshot creation initiated successfully.",
    });
    await fetchSnapshots();
  };

  const handleDeleteSnapshot = async (snapshotId: string) => {
    await snapshotApi.deleteSnapshot(snapshotId);
    toast({
      title: "Snapshot Deleted",
      description: "Snapshot deleted successfully.",
    });
    await fetchSnapshots();
  };

  const handleRevertSnapshot = async (snapshotId: string) => {
    await snapshotApi.restoreSnapshot(snapshotId);
    toast({
      title: "Snapshot Revert Initiated",
      description: "The VM will be reverted to the selected snapshot.",
    });
  };

  if (isLoading) {
    return (
      <div className="flex min-h-[400px] items-center justify-center">
        <div className="text-center">
          <Server className="mx-auto h-12 w-12 animate-pulse text-primary" />
          <p className="mt-4 text-muted-foreground">Loading VM details...</p>
        </div>
      </div>
    );
  }

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

      <VMControls
        vm={vm}
        isActionLoading={isVMActionLoading}
        onStart={handleStart}
        onStop={handleStop}
        onForceStop={handleForceStop}
        onRestart={handleRestart}
        showStopDialog={showStopDialog}
        setShowStopDialog={setShowStopDialog}
        showForceStopDialog={showForceStopDialog}
        setShowForceStopDialog={setShowForceStopDialog}
      />

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
            <div className="text-sm font-semibold">{vm.template_name ?? "Unknown"}</div>
            <p className="text-xs text-muted-foreground">
              VM ID: {vm.id}
            </p>
          </CardContent>
        </Card>
      </div>

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
              <p className="mt-1 font-mono text-lg">
                {vm.ip_addresses?.find((ip) => ip.ip_version === 6)?.address ?? "Not assigned"}
              </p>
            </div>
          </div>
        </CardContent>
      </Card>

      <Tabs defaultValue="console" className="w-full">
        <TabsList className="grid w-full grid-cols-9">
          <TabsTrigger value="console">
            <Monitor className="mr-2 h-4 w-4" />
            VNC
          </TabsTrigger>
          <TabsTrigger value="serial">
            <Monitor className="mr-2 h-4 w-4" />
            Serial
          </TabsTrigger>
          <TabsTrigger value="network">
            <Network className="mr-2 h-4 w-4" />
            Network
          </TabsTrigger>
          <TabsTrigger value="rdns">
            <Network className="mr-2 h-4 w-4" />
            RDNS
          </TabsTrigger>
          <TabsTrigger value="iso">
            <HardDrive className="mr-2 h-4 w-4" />
            ISO
          </TabsTrigger>
          <TabsTrigger value="backups">
            <Server className="mr-2 h-4 w-4" />
            Backups & Snapshots
          </TabsTrigger>
          <TabsTrigger value="metrics">
            <BarChart3 className="mr-2 h-4 w-4" />
            Metrics
          </TabsTrigger>
          <TabsTrigger value="settings">
            <Server className="mr-2 h-4 w-4" />
            Settings
          </TabsTrigger>
        </TabsList>

        {/* Console Tab */}
        <TabsContent value="console">
          <VMConsoleTab vmId={vmId} vmStatus={vm.status} />
        </TabsContent>

        <TabsContent value="serial">
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">Serial Console</CardTitle>
              <CardDescription>
                Access the virtual machine serial console via terminal
              </CardDescription>
            </CardHeader>
            <CardContent>
              {vm?.status !== "running" ? (
                <div className="flex min-h-[400px] flex-col items-center justify-center rounded-lg border border-dashed bg-muted/50">
                  <p className="text-sm text-muted-foreground">
                    VM must be running to access the serial console
                  </p>
                </div>
              ) : (
                <SerialConsoleWithToken vmId={vmId} vmName={vm.name} />
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="network">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Network className="h-5 w-5" />
                Network Information
              </CardTitle>
              <CardDescription>
                IP addresses and network configuration for this VM
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {vm.ipv4 && (
                  <div className="rounded-lg border p-4">
                    <div className="flex items-center gap-2 text-sm text-muted-foreground">
                      <Network className="h-4 w-4" />
                      <span>IPv4 Address</span>
                    </div>
                    <p className="mt-1 font-mono text-lg font-semibold">{vm.ipv4}</p>
                  </div>
                )}
                {vm.ip_addresses && vm.ip_addresses.length > 0 ? (
                  vm.ip_addresses.map((ip) => (
                    <div key={ip.id} className="rounded-lg border p-4">
                      <div className="flex items-center gap-2 text-sm text-muted-foreground">
                        <Network className="h-4 w-4" />
                        <span>IPv{ip.ip_version} {ip.is_primary ? "(Primary)" : ""}</span>
                      </div>
                      <p className="mt-1 font-mono text-lg font-semibold">{ip.address}</p>
                    </div>
                  ))
                ) : (
                  !vm.ipv4 && (
                    <p className="text-sm text-muted-foreground">No IP addresses assigned.</p>
                  )
                )}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        {/* RDNS Tab */}
        <TabsContent value="rdns">
          <VMRDNSTab vmId={vmId} />
        </TabsContent>

        {/* ISO Tab */}
        <TabsContent value="iso">
          <VMISOTab
            vmId={vmId}
            vmStatus={vm.status}
            attachedISOId={vm.attached_iso}
            maxISOSizeBytes={vm.max_iso_size_bytes}
          />
        </TabsContent>

        {/* Backups Tab */}
        <TabsContent value="backups">
          <VMBackupsTab
            vmId={vmId}
            vmName={vm.name}
            backups={backups}
            isLoading={isBackupsLoading}
            isActionLoading={isVMActionLoading}
            onRefresh={fetchBackups}
            onCreateBackup={handleCreateBackup}
            onDeleteBackup={handleDeleteBackup}
            onRestoreBackup={handleRestoreBackup}
            methodFilter={backupMethodFilter}
            onMethodFilterChange={setBackupMethodFilter}
          />
        </TabsContent>

        {/* Metrics Tab */}
        <TabsContent value="metrics">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <BarChart3 className="h-5 w-5" />
                Resource Metrics
              </CardTitle>
              <CardDescription>
                CPU, memory, network, and disk usage over time
              </CardDescription>
            </CardHeader>
            <CardContent>
              {vm?.status !== "running" ? (
                <div className="flex min-h-[300px] flex-col items-center justify-center rounded-lg border border-dashed bg-muted/50">
                  <p className="text-sm text-muted-foreground">
                    VM must be running to view metrics
                  </p>
                </div>
              ) : (
                <ResourceCharts vmId={vmId} />
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* Settings Tab */}
        <TabsContent value="settings">
          <VMSettingsTab vm={vm} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
