"use client";

import { useState, useEffect, useCallback } from "react";
import { useSearchParams } from "next/navigation";
import { Play, Square, RotateCw, Server, Loader2, Search } from "lucide-react";
import { Badge } from "@virtuestack/ui";
import { Button } from "@virtuestack/ui";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@virtuestack/ui";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@virtuestack/ui";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@virtuestack/ui";
import { useToast } from "@virtuestack/ui";
import { Input } from "@virtuestack/ui";
import { vmApi, VM } from "@/lib/api-client";
import { getStatusBadgeVariant, getStatusLabel, formatMemory } from "@/lib/vm-utils";
import { useVMAction } from "@/lib/hooks/useVMAction";

export default function VMsPage() {
  const [vms, setVms] = useState<VM[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [vmToStop, setVmToStop] = useState<string | null>(null);
  const { toast } = useToast();
  const searchParams = useSearchParams();
  const searchFromUrl = searchParams.get("search") || "";
  const [searchTerm, setSearchTerm] = useState(searchFromUrl);
  const { executeAction, isVMLoading } = useVMAction();

  const fetchVMs = useCallback(async () => {
    try {
      const response = await vmApi.getVMs();
      setVms(response.data || []);
    } catch (error) {
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

  useEffect(() => {
    const interval = setInterval(() => {
      fetchVMs();
    }, 30000);
    return () => clearInterval(interval);
  }, [fetchVMs]);

  const handleStart = (id: string) => {
    executeAction({
      action: "start",
      vmId: id,
      onSuccess: fetchVMs,
    });
  };

  const handleStop = (id: string) => {
    setVmToStop(null);
    executeAction({
      action: "stop",
      vmId: id,
      onSuccess: fetchVMs,
    });
  };

  const handleRestart = (id: string) => {
    executeAction({
      action: "restart",
      vmId: id,
      onSuccess: fetchVMs,
    });
  };

  const confirmStop = (id: string) => {
    setVmToStop(id);
  };

  const getVMName = (id: string) => {
    const vm = vms.find((v) => v.id === id);
    return vm?.name || id;
  };

  const filteredVMs = searchTerm.trim()
    ? vms.filter(
        (vm) =>
          vm.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
          vm.hostname.toLowerCase().includes(searchTerm.toLowerCase()) ||
          vm.ipv4?.toLowerCase().includes(searchTerm.toLowerCase())
      )
    : vms;

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
            You don&apos;t have any virtual machines assigned to your account yet.
            Contact your hosting provider if you expected to see one here.
          </CardDescription>
        </CardHeader>
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
          </div>
          <div className="relative mt-2 max-w-sm">
            <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
            <Input
              type="search"
              placeholder="Search by name, hostname or IP..."
              className="pl-8"
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
            />
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
              {filteredVMs.map((vm) => (
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
                          disabled={isVMLoading(vm.id)}
                          title="Start VM"
                        >
                          {isVMLoading(vm.id) ? (
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
                            disabled={isVMLoading(vm.id)}
                            title="Stop VM"
                          >
                            {isVMLoading(vm.id) ? (
                              <Loader2 className="h-4 w-4 animate-spin" />
                            ) : (
                              <Square className="h-4 w-4" />
                            )}
                          </Button>
                          <Button
                            variant="outline"
                            size="icon"
                            onClick={() => handleRestart(vm.id)}
                            disabled={isVMLoading(vm.id)}
                            title="Restart VM"
                          >
                            {isVMLoading(vm.id) ? (
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
                          disabled={isVMLoading(vm.id)}
                          title="Restart VM"
                        >
                          {isVMLoading(vm.id) ? (
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
              disabled={vmToStop ? isVMLoading(vmToStop) : false}
            >
              {vmToStop && isVMLoading(vmToStop) ? (
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
