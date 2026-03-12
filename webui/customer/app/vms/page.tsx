"use client";

import { useState, useEffect, useCallback } from "react";
import { Play, Square, RotateCw, Plus, Server, Loader2 } from "lucide-react";
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useToast } from "@/components/ui/use-toast";
import { vmApi, VM, ApiClientError } from "@/lib/api-client";

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
  const [vms, setVms] = useState<VM[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [loadingVMs, setLoadingVMs] = useState<Record<string, boolean>>({});
  const [vmToStop, setVmToStop] = useState<string | null>(null);
  const { toast } = useToast();

  const fetchVMs = useCallback(async () => {
    try {
      const data = await vmApi.getVMs();
      setVms(data);
    } catch (error) {
      console.error("Failed to fetch VMs:", error);
      toast({
        title: "Error",
        description: "Failed to load virtual machines. Please try again.",
        variant: "destructive",
      });
    } finally {
      setIsLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    fetchVMs();
  }, [fetchVMs]);

  const setVMLoading = (id: string, loading: boolean) => {
    setLoadingVMs((prev) => ({ ...prev, [id]: loading }));
  };

  const handleStart = async (id: string) => {
    setVMLoading(id, true);
    try {
      await vmApi.startVM(id);
      toast({
        title: "VM Started",
        description: "Virtual machine started successfully.",
      });
      await fetchVMs();
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
      setVMLoading(id, false);
    }
  };

  const handleStop = async (id: string) => {
    setVmToStop(null);
    setVMLoading(id, true);
    try {
      await vmApi.stopVM(id);
      toast({
        title: "VM Stopped",
        description: "Virtual machine stopped successfully.",
      });
      await fetchVMs();
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
      setVMLoading(id, false);
    }
  };

  const handleRestart = async (id: string) => {
    setVMLoading(id, true);
    try {
      await vmApi.restartVM(id);
      toast({
        title: "VM Restarted",
        description: "Virtual machine restarted successfully.",
      });
      await fetchVMs();
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
      setVMLoading(id, false);
    }
  };

  const handleCreateVM = () => {
    toast({
      title: "Coming Soon",
      description: "VM creation will be available in a future update.",
    });
  };

  const confirmStop = (id: string) => {
    setVmToStop(id);
  };

  const getVMName = (id: string) => {
    const vm = vms.find((v) => v.id === id);
    return vm?.name || id;
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
    <>
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
                          disabled={loadingVMs[vm.id]}
                          title="Start VM"
                        >
                          {loadingVMs[vm.id] ? (
                            <Loader2 className="h-4 w-4 animate-spin" />
                          ) : (
                            <Play className="h-4 w-4" />
                          )}
                        </Button>
                      )}
                      {vm.status === "running" && (
                        <>
                          <Button
                            variant="outline"
                            size="icon"
                            onClick={() => confirmStop(vm.id)}
                            disabled={loadingVMs[vm.id]}
                            title="Stop VM"
                          >
                            {loadingVMs[vm.id] ? (
                              <Loader2 className="h-4 w-4 animate-spin" />
                            ) : (
                              <Square className="h-4 w-4" />
                            )}
                          </Button>
                          <Button
                            variant="outline"
                            size="icon"
                            onClick={() => handleRestart(vm.id)}
                            disabled={loadingVMs[vm.id]}
                            title="Restart VM"
                          >
                            {loadingVMs[vm.id] ? (
                              <Loader2 className="h-4 w-4 animate-spin" />
                            ) : (
                              <RotateCw className="h-4 w-4" />
                            )}
                          </Button>
                        </>
                      )}
                      {vm.status === "error" && (
                        <Button
                          variant="outline"
                          size="icon"
                          onClick={() => handleRestart(vm.id)}
                          disabled={loadingVMs[vm.id]}
                          title="Restart VM"
                        >
                          {loadingVMs[vm.id] ? (
                            <Loader2 className="h-4 w-4 animate-spin" />
                          ) : (
                            <RotateCw className="h-4 w-4" />
                          )}
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

      <Dialog open={!!vmToStop} onOpenChange={() => setVmToStop(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Stop Virtual Machine</DialogTitle>
            <DialogDescription>
              Are you sure you want to stop <strong>{vmToStop ? getVMName(vmToStop) : ""}</strong>?
              This will perform a graceful shutdown of the VM.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setVmToStop(null)}>
              Cancel
            </Button>
            <Button 
              variant="default" 
              onClick={() => vmToStop && handleStop(vmToStop)}
              disabled={vmToStop ? loadingVMs[vmToStop] : false}
            >
              {vmToStop && loadingVMs[vmToStop] ? (
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
    </>
  );
}
