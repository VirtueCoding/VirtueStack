"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
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
import { Archive, Loader2, Trash2, RefreshCcw, Download, CheckCircle2, XCircle, Clock } from "lucide-react";
import type { Backup } from "@/lib/api-client";
import { getStatusLabel, formatBytes } from "@/lib/vm-utils";

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

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

interface VMBackupsTabProps {
  vmId: string;
  vmName: string;
  backups: Backup[];
  isLoading: boolean;
  isActionLoading: boolean;
  onRefresh: () => void;
  onCreateBackup: (name: string) => Promise<void>;
  onDeleteBackup: (backupId: string) => Promise<void>;
  onRestoreBackup: (backupId: string) => Promise<void>;
}

export function VMBackupsTab({
  vmName,
  backups,
  isLoading,
  isActionLoading,
  onRefresh,
  onCreateBackup,
  onDeleteBackup,
  onRestoreBackup,
}: VMBackupsTabProps) {
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const [showRestoreDialog, setShowRestoreDialog] = useState(false);
  const [selectedBackup, setSelectedBackup] = useState<Backup | null>(null);
  const [backupName, setBackupName] = useState("");

  const handleCreate = async () => {
    if (!backupName.trim()) return;
    await onCreateBackup(backupName.trim());
    setShowCreateDialog(false);
    setBackupName("");
  };

  const handleDelete = async () => {
    if (!selectedBackup) return;
    await onDeleteBackup(selectedBackup.id);
    setShowDeleteDialog(false);
    setSelectedBackup(null);
  };

  const handleRestore = async () => {
    if (!selectedBackup) return;
    await onRestoreBackup(selectedBackup.id);
    setShowRestoreDialog(false);
    setSelectedBackup(null);
  };

  return (
    <>
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
              setShowCreateDialog(true);
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
          {isLoading ? (
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
                        setShowRestoreDialog(true);
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
                        setShowDeleteDialog(true);
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

      {/* Create Backup Dialog */}
      <Dialog open={showCreateDialog} onOpenChange={setShowCreateDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create Backup</DialogTitle>
            <DialogDescription>
              Create a new backup of <strong>{vmName}</strong>.
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
                maxLength={128}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowCreateDialog(false)}>
              Cancel
            </Button>
            <Button
              onClick={handleCreate}
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
      <Dialog open={showDeleteDialog} onOpenChange={setShowDeleteDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Backup</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete the backup &quot;{selectedBackup?.name}&quot;?
              This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowDeleteDialog(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDelete}
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
      <Dialog open={showRestoreDialog} onOpenChange={setShowRestoreDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Restore Backup</DialogTitle>
            <DialogDescription>
              Are you sure you want to restore <strong>{vmName}</strong> from the backup &quot;{selectedBackup?.name}&quot;?
              This will overwrite the current VM state and cannot be undone.
              The VM will be temporarily unavailable during restoration.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowRestoreDialog(false)}>
              Cancel
            </Button>
            <Button
              onClick={handleRestore}
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
    </>
  );
}