"use client";

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
import { Play, Square, RotateCw, Zap, Loader2 } from "lucide-react";
import type { VM } from "@/lib/api-client";

interface VMControlsProps {
  vm: VM | null;
  isActionLoading: boolean;
  onStart: () => void;
  onStop: () => void;
  onForceStop: () => void;
  onRestart: () => void;
  showStopDialog: boolean;
  setShowStopDialog: (show: boolean) => void;
  showForceStopDialog: boolean;
  setShowForceStopDialog: (show: boolean) => void;
}

export function VMControls({
  vm,
  isActionLoading,
  onStart,
  onStop,
  onForceStop,
  onRestart,
  showStopDialog,
  setShowStopDialog,
  showForceStopDialog,
  setShowForceStopDialog,
}: VMControlsProps) {
  if (!vm) return null;

  return (
    <>
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
              <Button onClick={onStart} disabled={isActionLoading}>
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
                  onClick={onRestart}
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
                  onClick={onRestart}
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
            <Button onClick={onStop} disabled={isActionLoading}>
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
              onClick={onForceStop}
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
    </>
  );
}