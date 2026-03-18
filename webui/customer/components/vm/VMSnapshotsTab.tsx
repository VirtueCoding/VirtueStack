"use client";

import { useState } from "react";
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
import { Camera, Loader2, Trash2, RefreshCcw, Clock } from "lucide-react";
import type { Snapshot } from "@/lib/api-client";
import { formatBytes } from "@/lib/vm-utils";

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

interface VMSnapshotsTabProps {
  vmId: string;
  vmName: string;
  snapshots: Snapshot[];
  isLoading: boolean;
  isActionLoading: boolean;
  onRefresh: () => void;
  onCreateSnapshot: (name: string) => Promise<void>;
  onDeleteSnapshot: (snapshotId: string) => Promise<void>;
  onRevertSnapshot: (snapshotId: string) => Promise<void>;
}

export function VMSnapshotsTab({
  vmName,
  snapshots,
  isLoading,
  isActionLoading,
  onCreateSnapshot,
  onDeleteSnapshot,
  onRevertSnapshot,
}: VMSnapshotsTabProps) {
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const [showRevertDialog, setShowRevertDialog] = useState(false);
  const [selectedSnapshot, setSelectedSnapshot] = useState<Snapshot | null>(null);
  const [snapshotName, setSnapshotName] = useState("");

  const handleCreate = async () => {
    if (!snapshotName.trim()) return;
    await onCreateSnapshot(snapshotName.trim());
    setShowCreateDialog(false);
    setSnapshotName("");
  };

  const handleDelete = async () => {
    if (!selectedSnapshot) return;
    await onDeleteSnapshot(selectedSnapshot.id);
    setShowDeleteDialog(false);
    setSelectedSnapshot(null);
  };

  const handleRevert = async () => {
    if (!selectedSnapshot) return;
    await onRevertSnapshot(selectedSnapshot.id);
    setShowRevertDialog(false);
    setSelectedSnapshot(null);
  };

  return (
    <>
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
              setShowCreateDialog(true);
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
          {isLoading ? (
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
                        setShowRevertDialog(true);
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

      {/* Create Snapshot Dialog */}
      <Dialog open={showCreateDialog} onOpenChange={setShowCreateDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create Snapshot</DialogTitle>
            <DialogDescription>
              Create a new snapshot of <strong>{vmName}</strong>.
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
      <Dialog open={showDeleteDialog} onOpenChange={setShowDeleteDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Snapshot</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete the snapshot &quot;{selectedSnapshot?.name}&quot;?
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
                "Delete Snapshot"
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Revert Snapshot Dialog */}
      <Dialog open={showRevertDialog} onOpenChange={setShowRevertDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Revert to Snapshot</DialogTitle>
            <DialogDescription>
              Are you sure you want to revert <strong>{vmName}</strong> to the snapshot &quot;{selectedSnapshot?.name}&quot;?
              This will discard all changes made since the snapshot was created.
              The VM will be temporarily unavailable during reversion.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowRevertDialog(false)}>
              Cancel
            </Button>
            <Button
              onClick={handleRevert}
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
    </>
  );
}