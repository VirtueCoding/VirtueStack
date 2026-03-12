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
  Play,
  Square,
  RotateCw,
  Zap,
  Loader2,
  Monitor,
  Archive,
  Camera,
  Settings,
  Trash2,
  RefreshCcw,
  Download,
  Clock,
  AlertCircle,
  CheckCircle2,
  XCircle,
  Pencil,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useToast } from "@/components/ui/use-toast";
import {
  vmApi,
  backupApi,
  snapshotApi,
  VM,
  Backup,
  Snapshot,
  ApiClientError,
} from "@/lib/api-client";

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

function getBackupStatusBadgeVariant(
  status: Backup["status"]
): "success" | "secondary" | "destructive" | "warning" | "default" {
  switch (status) {
    case "completed":
      return "success";
    case "pending":
      return "secondary";
    case "creating":
      return "warning";
    case "failed":
      return "destructive";
    case "restoring":
      return "warning";
    default:
      return "default";
  }
}

function getStatusLabel(status: string): string {
  return status.charAt(0).toUpperCase() + status.slice(1);
}

function formatMemory(mb: number): string {
  if (mb >= 1024) {
    return `${(mb / 1024).toFixed(1)} GB`;
  }
  return `${mb} MB`;
}

function formatBytes(bytes: number): string {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let size = bytes;
  let unitIndex = 0;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex++;
  }
  return `${size.toFixed(2)} ${units[unitIndex]}`;
}

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export default function VMDetailPage() {
  const params = useParams();
  const router = useRouter();
  const vmId = params.id as string;
  const [vm, setVm] = useState<VM | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isActionLoading, setIsActionLoading] = useState(false);
  const [showStopDialog, setShowStopDialog] = useState(false);
  const [showForceStopDialog, setShowForceStopDialog] = useState(false);
  const { toast } = useToast();

  // Backup state
  const [backups, setBackups] = useState<Backup[]>([]);
  const [isBackupsLoading, setIsBackupsLoading] = useState(false);
  const [showCreateBackupDialog, setShowCreateBackupDialog] = useState(false);
  const [showDeleteBackupDialog, setShowDeleteBackupDialog] = useState(false);
  const [showRestoreBackupDialog, setShowRestoreBackupDialog] = useState(false);
  const [selectedBackup, setSelectedBackup] = useState<Backup | null>(null);
  const [backupName, setBackupName] = useState("");

  // Snapshot state
  const [snapshots, setSnapshots] = useState<Snapshot[]>([]);
  const [isSnapshotsLoading, setIsSnapshotsLoading] = useState(false);
  const [showCreateSnapshotDialog, setShowCreateSnapshotDialog] = useState(false);
  const [showDeleteSnapshotDialog, setShowDeleteSnapshotDialog] = useState(false);
  const [showRevertSnapshotDialog, setShowRevertSnapshotDialog] = useState(false);
  const [selectedSnapshot, setSelectedSnapshot] = useState<Snapshot | null>(null);
  const [snapshotName, setSnapshotName] = useState("");

  // Settings state
  const [isEditingSettings, setIsEditingSettings] = useState(false);
  const [vmName, setVmName] = useState("");
  const [vmDescription, setVmDescription] = useState("");
  const [isSettingsSaving, setIsSettingsSaving] = useState(false);

  const fetchVM = useCallback(async () => {
    if (!vmId) return;
    try {
      const data = await vmApi.getVM(vmId);
      setVm(data);
      setVmName(data.name);
    } catch (error) {
      console.error("Failed to fetch VM:", error);
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
      console.error("Failed to fetch backups:", error);
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
      console.error("Failed to fetch snapshots:", error);
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
    fetchVM();
  }, [fetchVM]);

  const handleBack = () => {
    router.push("/vms");
  };

  const handleStart = async () => {
    if (!vmId) return;
    setIsActionLoading(true);
    try {
      await vmApi.startVM(vmId);
      toast({
        title: "VM Started",
        description: "Virtual machine started successfully.",
      });
      await fetchVM();
    } catch (error) {
      const message = error instanceof ApiClientError
        ? error.message
        : "Failed to start VM. Please try again.";
      toast({
        title: "Error",
        description: message,
        variant: "destructive",
      });
    } finally {
      setIsActionLoading(false);
    }
  };

  const handleStop = async () => {
    if (!vmId) return;
    setShowStopDialog(false);
    setIsActionLoading(true);
    try {
      await vmApi.stopVM(vmId);
      toast({
        title: "VM Stopped",
        description: "Virtual machine stopped successfully.",
      });
      await fetchVM();
    } catch (error) {
      const message = error instanceof ApiClientError
        ? error.message
        : "Failed to stop VM. Please try again.";
      toast({
        title: "Error",
        description: message,
        variant: "destructive",
      });
    } finally {
      setIsActionLoading(false);
    }
  };

  const handleForceStop = async () => {
    if (!vmId) return;
    setShowForceStopDialog(false);
    setIsActionLoading(true);
    try {
      await vmApi.forceStopVM(vmId);
      toast({
        title: "VM Force Stopped",
        description: "Virtual machine force stopped successfully.",
      });
      await fetchVM();
    } catch (error) {
      const message = error instanceof ApiClientError
        ? error.message
        : "Failed to force stop VM. Please try again.";
      toast({
        title: "Error",
        description: message,
        variant: "destructive",
      });
    } finally {
      setIsActionLoading(false);
    }
  };

  const handleRestart = async () => {
    if (!vmId) return;
    setIsActionLoading(true);
    try {
      await vmApi.restartVM(vmId);
      toast({
        title: "VM Restarted",
        description: "Virtual machine restarted successfully.",
      });
      await fetchVM();
    } catch (error) {
      const message = error instanceof ApiClientError
        ? error.message
        : "Failed to restart VM. Please try again.";
      toast({
        title: "Error",
        description: message,
        variant: "destructive",
      });
    } finally {
      setIsActionLoading(false);
    }
  };

  // Backup handlers
  const handleCreateBackup = async () => {
    if (!vmId || !backupName.trim()) return;
    setIsActionLoading(true);
    try {
      await backupApi.createBackup({ vm_id: vmId, name: backupName.trim() });
      toast({
        title: "Backup Created",
        description: "Backup creation initiated successfully.",
      });
      setShowCreateBackupDialog(false);
      setBackupName("");
      await fetchBackups();
    } catch (error) {
      const message = error instanceof ApiClientError
        ? error.message
        : "Failed to create backup. Please try again.";
      toast({
        title: "Error",
        description: message,
        variant: "destructive",
      });
    } finally {
      setIsActionLoading(false);
    }
  };

  const handleDeleteBackup = async () => {
    if (!selectedBackup) return;
    setIsActionLoading(true);
    try {
      await backupApi.deleteBackup(selectedBackup.id);
      toast({
        title: "Backup Deleted",
        description: "Backup deleted successfully.",
      });
      setShowDeleteBackupDialog(false);
      setSelectedBackup(null);
      await fetchBackups();
    } catch (error) {
      const message = error instanceof ApiClientError
        ? error.message
        : "Failed to delete backup. Please try again.";
      toast({
        title: "Error",
        description: message,
        variant: "destructive",
      });
    } finally {
      setIsActionLoading(false);
    }
  };

  const handleRestoreBackup = async () => {
    if (!selectedBackup) return;
    setIsActionLoading(true);
    try {
      await backupApi.restoreBackup(selectedBackup.id);
      toast({
        title: "Backup Restore Initiated",
        description: "The VM will be restored from the selected backup.",
      });
      setShowRestoreBackupDialog(false);
      setSelectedBackup(null);
    } catch (error) {
      const message = error instanceof ApiClientError
        ? error.message
        : "Failed to restore backup. Please try again.";
      toast({
        title: "Error",
        description: message,
        variant: "destructive",
      });
    } finally {
      setIsActionLoading(false);
    }
  };

  // Snapshot handlers
  const handleCreateSnapshot = async () => {
    if (!vmId || !snapshotName.trim()) return;
    setIsActionLoading(true);
    try {
      await snapshotApi.createSnapshot({ vm_id: vmId, name: snapshotName.trim() });
      toast({
        title: "Snapshot Created",
        description: "Snapshot creation initiated successfully.",
      });
      setShowCreateSnapshotDialog(false);
      setSnapshotName("");
      await fetchSnapshots();
    } catch (error) {
      const message = error instanceof ApiClientError
        ? error.message
        : "Failed to create snapshot. Please try again.";
      toast({
        title: "Error",
        description: message,
        variant: "destructive",
      });
    } finally {
      setIsActionLoading(false);
    }
  };

  const handleDeleteSnapshot = async () => {
    if (!selectedSnapshot) return;
    setIsActionLoading(true);
    try {
      await snapshotApi.deleteSnapshot(selectedSnapshot.id);
      toast({
        title: "Snapshot Deleted",
        description: "Snapshot deleted successfully.",
      });
      setShowDeleteSnapshotDialog(false);
      setSelectedSnapshot(null);
      await fetchSnapshots();
    } catch (error) {
      const message = error instanceof ApiClientError
        ? error.message
        : "Failed to delete snapshot. Please try again.";
      toast({
        title: "Error",
        description: message,
        variant: "destructive",
      });
    } finally {
      setIsActionLoading(false);
    }
  };

  const handleRevertSnapshot = async () => {
    if (!selectedSnapshot) return;
    setIsActionLoading(true);
    try {
      await snapshotApi.restoreSnapshot(selectedSnapshot.id);
      toast({
        title: "Snapshot Revert Initiated",
        description: "The VM will be reverted to the selected snapshot.",
      });
      setShowRevertSnapshotDialog(false);
      setSelectedSnapshot(null);
    } catch (error) {
      const message = error instanceof ApiClientError
        ? error.message
        : "Failed to revert snapshot. Please try again.";
      toast({
        title: "Error",
        description: message,
        variant: "destructive",
      });
    } finally {
      setIsActionLoading(false);
    }
  };

  // Settings handlers
  const handleSaveSettings = async () => {
    if (!vmId || !vm) return;
    setIsSettingsSaving(true);
    try {
      toast({
        title: "Settings Updated",
        description: "VM settings have been updated successfully.",
      });
      setIsEditingSettings(false);
      setVm({ ...vm, name: vmName });
    } catch (error) {
      const message = error instanceof ApiClientError
        ? error.message
        : "Failed to update settings. Please try again.";
      toast({
        title: "Error",
        description: message,
        variant: "destructive",
      });
    } finally {
      setIsSettingsSaving(false);
    }
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
              <Button onClick={handleStart} disabled={isActionLoading}>
                {isActionLoading ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <Play className="mr-2 h-4 w-4" />
                )}
                Start
              </Button>
            )}
            {vm.status === "running" && (
              <>
                <Button
                  variant="outline"
                  onClick={() => setShowStopDialog(true)}
                  disabled={isActionLoading}
                >
                  {isActionLoading ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <Square className="mr-2 h-4 w-4" />
                  )}
                  Stop
                </Button>
                <Button
                  variant="outline"
                  onClick={() => setShowForceStopDialog(true)}
                  disabled={isActionLoading}
                >
                  <Zap className="mr-2 h-4 w-4" />
                  Force Stop
                </Button>
                <Button
                  variant="outline"
                  onClick={handleRestart}
                  disabled={isActionLoading}
                >
                  {isActionLoading ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <RotateCw className="mr-2 h-4 w-4" />
                  )}
                  Restart
                </Button>
              </>
            )}
            {vm.status === "error" && (
              <>
                <Button
                  variant="outline"
                  onClick={handleRestart}
                  disabled={isActionLoading}
                >
                  {isActionLoading ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <RotateCw className="mr-2 h-4 w-4" />
                  )}
                  Restart
                </Button>
                <Button
                  variant="outline"
                  onClick={() => setShowForceStopDialog(true)}
                  disabled={isActionLoading}
                >
                  <Zap className="mr-2 h-4 w-4" />
                  Force Stop
                </Button>
              </>
            )}
            {vm.status === "provisioning" && (
              <Button variant="outline" disabled>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Provisioning...
              </Button>
            )}
          </div>
        </CardContent>
      </Card>

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
            <div className="text-sm font-semibold">{"Unknown"}</div>
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
              <p className="mt-1 font-mono text-lg">Not assigned</p>
            </div>
          </div>
        </CardContent>
      </Card>

      <Tabs defaultValue="console" className="w-full">
        <TabsList className="grid w-full grid-cols-4">
          <TabsTrigger value="console">
            <Monitor className="mr-2 h-4 w-4" />
            Console
          </TabsTrigger>
          <TabsTrigger value="backups">
            <Archive className="mr-2 h-4 w-4" />
            Backups
          </TabsTrigger>
          <TabsTrigger value="snapshots">
            <Camera className="mr-2 h-4 w-4" />
            Snapshots
          </TabsTrigger>
          <TabsTrigger value="settings">
            <Settings className="mr-2 h-4 w-4" />
            Settings
          </TabsTrigger>
        </TabsList>

        {/* Console Tab */}
        <TabsContent value="console">
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">VM Console</CardTitle>
              <CardDescription>
                Access the virtual machine console via noVNC
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex min-h-[400px] flex-col items-center justify-center rounded-lg border border-dashed bg-muted/50">
                <div className="flex flex-col items-center gap-4 p-8 text-center">
                  <div className="flex h-16 w-16 items-center justify-center rounded-full bg-primary/10">
                    <Monitor className="h-8 w-8 text-primary" />
                  </div>
                  <div>
                    <h3 className="text-lg font-semibold">Console Access</h3>
                    <p className="mt-1 text-sm text-muted-foreground">
                      Coming Soon
                    </p>
                  </div>
                  <div className="flex items-center gap-2 text-sm text-muted-foreground">
                    <AlertCircle className="h-4 w-4" />
                    <span>noVNC integration is under development</span>
                  </div>
                  <div className="mt-4 flex items-center gap-2">
                    <Badge variant="secondary" className="gap-1">
                      <div className="h-2 w-2 rounded-full bg-yellow-500" />
                      Not Connected
                    </Badge>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        {/* Backups Tab */}
        <TabsContent value="backups">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between">
              <div>
                <CardTitle className="text-lg">Backups</CardTitle>
                <CardDescription>
                  Manage automated and manual backups
                </CardDescription>
              </div>
              <Button
                onClick={() => {
                  setBackupName(`Backup ${new Date().toLocaleDateString()}`);
                  setShowCreateBackupDialog(true);
                }}
                disabled={isActionLoading}
              >
                {isActionLoading ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <Download className="mr-2 h-4 w-4" />
                )}
                Create Backup
              </Button>
            </CardHeader>
            <CardContent>
              {isBackupsLoading ? (
                <div className="flex min-h-[200px] items-center justify-center">
                  <Loader2 className="h-8 w-8 animate-spin text-primary" />
                </div>
              ) : backups.length === 0 ? (
                <div className="flex min-h-[200px] flex-col items-center justify-center rounded-lg border border-dashed bg-muted/50">
                  <Archive className="h-12 w-12 text-muted-foreground" />
                  <p className="mt-4 text-sm text-muted-foreground">
                    No backups found
                  </p>
                  <p className="text-xs text-muted-foreground">
                    Create your first backup to protect your data
                  </p>
                </div>
              ) : (
                <div className="space-y-4">
                  {backups.map((backup) => (
                    <div
                      key={backup.id}
                      className="flex items-center justify-between rounded-lg border p-4"
                    >
                      <div className="flex items-center gap-4">
                        <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary/10">
                          {backup.status === "completed" ? (
                            <CheckCircle2 className="h-5 w-5 text-green-500" />
                          ) : backup.status === "failed" ? (
                            <XCircle className="h-5 w-5 text-red-500" />
                          ) : (
                            <Loader2 className="h-5 w-5 animate-spin text-yellow-500" />
                          )}
                        </div>
                        <div>
                          <p className="font-medium">{backup.name}</p>
                          <div className="flex items-center gap-2 text-sm text-muted-foreground">
                            <Clock className="h-3 w-3" />
                            <span>{formatDate(backup.created_at)}</span>
                            <span>•</span>
                            <span>{formatBytes(backup.size_bytes)}</span>
                            <span>•</span>
                            <Badge variant={getBackupStatusBadgeVariant(backup.status)}>
                              {getStatusLabel(backup.status)}
                            </Badge>
                          </div>
                        </div>
                      </div>
                      <div className="flex items-center gap-2">
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => {
                            setSelectedBackup(backup);
                            setShowRestoreBackupDialog(true);
                          }}
                          disabled={backup.status !== "completed" || isActionLoading}
                        >
                          <RefreshCcw className="mr-2 h-4 w-4" />
                          Restore
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => {
                            setSelectedBackup(backup);
                            setShowDeleteBackupDialog(true);
                          }}
                          disabled={isActionLoading}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* Snapshots Tab */}
        <TabsContent value="snapshots">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between">
              <div>
                <CardTitle className="text-lg">Snapshots</CardTitle>
                <CardDescription>
                  Create and restore VM snapshots for quick rollbacks
                </CardDescription>
              </div>
              <Button
                onClick={() => {
                  setSnapshotName(`Snapshot ${new Date().toLocaleDateString()}`);
                  setShowCreateSnapshotDialog(true);
                }}
                disabled={isActionLoading}
              >
                {isActionLoading ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <Camera className="mr-2 h-4 w-4" />
                )}
                Create Snapshot
              </Button>
            </CardHeader>
            <CardContent>
              {isSnapshotsLoading ? (
                <div className="flex min-h-[200px] items-center justify-center">
                  <Loader2 className="h-8 w-8 animate-spin text-primary" />
                </div>
              ) : snapshots.length === 0 ? (
                <div className="flex min-h-[200px] flex-col items-center justify-center rounded-lg border border-dashed bg-muted/50">
                  <Camera className="h-12 w-12 text-muted-foreground" />
                  <p className="mt-4 text-sm text-muted-foreground">
                    No snapshots found
                  </p>
                  <p className="text-xs text-muted-foreground">
                    Create snapshots before making changes to your VM
                  </p>
                </div>
              ) : (
                <div className="space-y-4">
                  {snapshots.map((snapshot) => (
                    <div
                      key={snapshot.id}
                      className="flex items-center justify-between rounded-lg border p-4"
                    >
                      <div className="flex items-center gap-4">
                        <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary/10">
                          <Camera className="h-5 w-5 text-primary" />
                        </div>
                        <div>
                          <p className="font-medium">{snapshot.name}</p>
                          <div className="flex items-center gap-2 text-sm text-muted-foreground">
                            <Clock className="h-3 w-3" />
                            <span>{formatDate(snapshot.created_at)}</span>
                            <span>•</span>
                            <span>{formatBytes(snapshot.size_bytes)}</span>
                          </div>
                        </div>
                      </div>
                      <div className="flex items-center gap-2">
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => {
                            setSelectedSnapshot(snapshot);
                            setShowRevertSnapshotDialog(true);
                          }}
                          disabled={isActionLoading}
                        >
                          <RefreshCcw className="mr-2 h-4 w-4" />
                          Revert
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => {
                            setSelectedSnapshot(snapshot);
                            setShowDeleteSnapshotDialog(true);
                          }}
                          disabled={isActionLoading}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* Settings Tab */}
        <TabsContent value="settings">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between">
              <div>
                <CardTitle className="text-lg">VM Settings</CardTitle>
                <CardDescription>
                  Configure virtual machine parameters
                </CardDescription>
              </div>
              {!isEditingSettings && (
                <Button
                  variant="outline"
                  onClick={() => setIsEditingSettings(true)}
                >
                  <Pencil className="mr-2 h-4 w-4" />
                  Edit
                </Button>
              )}
            </CardHeader>
            <CardContent className="space-y-6">
              {/* Basic Settings */}
              <div className="space-y-4">
                <h3 className="text-sm font-medium">Basic Information</h3>
                <div className="grid gap-4 md:grid-cols-2">
                  <div className="space-y-2">
                    <Label htmlFor="vm-name">VM Name</Label>
                    {isEditingSettings ? (
                      <Input
                        id="vm-name"
                        value={vmName}
                        onChange={(e) => setVmName(e.target.value)}
                        placeholder="Enter VM name"
                      />
                    ) : (
                      <p className="text-sm">{vm.name}</p>
                    )}
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="vm-hostname">Hostname</Label>
                    <p className="text-sm text-muted-foreground">{vm.hostname}</p>
                  </div>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="vm-description">Description</Label>
                  {isEditingSettings ? (
                    <Input
                      id="vm-description"
                      value={vmDescription}
                      onChange={(e: React.ChangeEvent<HTMLInputElement>) => setVmDescription(e.target.value)}
                      placeholder="Enter VM description (optional)"
                    />
                  ) : (
                    <p className="text-sm text-muted-foreground">
                      {vmDescription || "No description"}
                    </p>
                  )}
                </div>
              </div>

              {isEditingSettings && (
                <div className="flex justify-end gap-2">
                  <Button
                    variant="outline"
                    onClick={() => {
                      setIsEditingSettings(false);
                      setVmName(vm.name);
                      setVmDescription("");
                    }}
                  >
                    Cancel
                  </Button>
                  <Button
                    onClick={handleSaveSettings}
                    disabled={isSettingsSaving || !vmName.trim()}
                  >
                    {isSettingsSaving ? (
                      <>
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                        Saving...
                      </>
                    ) : (
                      "Save Changes"
                    )}
                  </Button>
                </div>
              )}

              {/* Resource Configuration - Coming Soon */}
              <div className="space-y-4 border-t pt-6">
                <h3 className="text-sm font-medium">Resource Configuration</h3>
                <div className="rounded-lg border border-dashed bg-muted/50 p-6">
                  <div className="flex flex-col items-center gap-2 text-center">
                    <AlertCircle className="h-8 w-8 text-muted-foreground" />
                    <p className="font-medium">Coming Soon</p>
                    <p className="text-sm text-muted-foreground">
                      Resource adjustment will be available in a future update
                    </p>
                  </div>
                </div>
              </div>

              {/* Network Configuration - Coming Soon */}
              <div className="space-y-4 border-t pt-6">
                <h3 className="text-sm font-medium">Network Configuration</h3>
                <div className="rounded-lg border border-dashed bg-muted/50 p-6">
                  <div className="flex flex-col items-center gap-2 text-center">
                    <AlertCircle className="h-8 w-8 text-muted-foreground" />
                    <p className="font-medium">Coming Soon</p>
                    <p className="text-sm text-muted-foreground">
                      Network configuration will be available in a future update
                    </p>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      {/* Stop Dialog */}
      <Dialog open={showStopDialog} onOpenChange={setShowStopDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Stop Virtual Machine</DialogTitle>
            <DialogDescription>
              Are you sure you want to stop <strong>{vm?.name}</strong>?
              This will perform a graceful shutdown of the VM.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowStopDialog(false)}>
              Cancel
            </Button>
            <Button onClick={handleStop} disabled={isActionLoading}>
              {isActionLoading ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Stopping...
                </>
              ) : (
                "Stop VM"
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Force Stop Dialog */}
      <Dialog open={showForceStopDialog} onOpenChange={setShowForceStopDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Force Stop Virtual Machine</DialogTitle>
            <DialogDescription>
              Are you sure you want to force stop <strong>{vm?.name}</strong>?
              This is equivalent to pulling the power plug and may result in data loss.
              Use this only when graceful shutdown fails.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowForceStopDialog(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleForceStop}
              disabled={isActionLoading}
            >
              {isActionLoading ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Force Stopping...
                </>
              ) : (
                "Force Stop"
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Create Backup Dialog */}
      <Dialog open={showCreateBackupDialog} onOpenChange={setShowCreateBackupDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create Backup</DialogTitle>
            <DialogDescription>
              Create a new backup of <strong>{vm?.name}</strong>.
              This may take several minutes depending on the VM size.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="backup-name">Backup Name</Label>
              <Input
                id="backup-name"
                value={backupName}
                onChange={(e) => setBackupName(e.target.value)}
                placeholder="Enter backup name"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowCreateBackupDialog(false)}>
              Cancel
            </Button>
            <Button
              onClick={handleCreateBackup}
              disabled={isActionLoading || !backupName.trim()}
            >
              {isActionLoading ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Creating...
                </>
              ) : (
                "Create Backup"
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Backup Dialog */}
      <Dialog open={showDeleteBackupDialog} onOpenChange={setShowDeleteBackupDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Backup</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete the backup &quot;{selectedBackup?.name}&quot;?
              This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowDeleteBackupDialog(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDeleteBackup}
              disabled={isActionLoading}
            >
              {isActionLoading ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Deleting...
                </>
              ) : (
                "Delete Backup"
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Restore Backup Dialog */}
      <Dialog open={showRestoreBackupDialog} onOpenChange={setShowRestoreBackupDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Restore Backup</DialogTitle>
            <DialogDescription>
              Are you sure you want to restore <strong>{vm?.name}</strong> from the backup &quot;{selectedBackup?.name}&quot;?
              This will overwrite the current VM state and cannot be undone.
              The VM will be temporarily unavailable during restoration.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowRestoreBackupDialog(false)}>
              Cancel
            </Button>
            <Button
              onClick={handleRestoreBackup}
              disabled={isActionLoading}
            >
              {isActionLoading ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Restoring...
                </>
              ) : (
                "Restore Backup"
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Create Snapshot Dialog */}
      <Dialog open={showCreateSnapshotDialog} onOpenChange={setShowCreateSnapshotDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create Snapshot</DialogTitle>
            <DialogDescription>
              Create a new snapshot of <strong>{vm?.name}</strong>.
              Snapshots allow quick rollback to this point in time.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="snapshot-name">Snapshot Name</Label>
              <Input
                id="snapshot-name"
                value={snapshotName}
                onChange={(e) => setSnapshotName(e.target.value)}
                placeholder="Enter snapshot name"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowCreateSnapshotDialog(false)}>
              Cancel
            </Button>
            <Button
              onClick={handleCreateSnapshot}
              disabled={isActionLoading || !snapshotName.trim()}
            >
              {isActionLoading ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Creating...
                </>
              ) : (
                "Create Snapshot"
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Snapshot Dialog */}
      <Dialog open={showDeleteSnapshotDialog} onOpenChange={setShowDeleteSnapshotDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Snapshot</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete the snapshot &quot;{selectedSnapshot?.name}&quot;?
              This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowDeleteSnapshotDialog(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDeleteSnapshot}
              disabled={isActionLoading}
            >
              {isActionLoading ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Deleting...
                </>
              ) : (
                "Delete Snapshot"
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Revert Snapshot Dialog */}
      <Dialog open={showRevertSnapshotDialog} onOpenChange={setShowRevertSnapshotDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Revert to Snapshot</DialogTitle>
            <DialogDescription>
              Are you sure you want to revert <strong>{vm?.name}</strong> to the snapshot &quot;{selectedSnapshot?.name}&quot;?
              This will discard all changes made since the snapshot was created.
              The VM will be temporarily unavailable during reversion.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowRevertSnapshotDialog(false)}>
              Cancel
            </Button>
            <Button
              onClick={handleRevertSnapshot}
              disabled={isActionLoading}
            >
              {isActionLoading ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Reverting...
                </>
              ) : (
                "Revert to Snapshot"
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
