"use client";

import { useState, useEffect } from "react";
import { HardDrive, Upload, Trash2, Link, Unlink, Loader2 } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@virtuestack/ui";
import { Button } from "@virtuestack/ui";
import { Badge } from "@virtuestack/ui";
import { useToast } from "@virtuestack/ui";
import { isoApi, ISORecord, ApiClientError } from "@/lib/api-client";
import { ISOUpload } from "@/components/file-upload/iso-upload";
import { formatBytes } from "@/lib/vm-utils";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@virtuestack/ui";

interface VMISOTabProps {
  vmId: string;
  vmStatus: string;
  attachedISOId?: string | null;
}

export function VMISOTab({ vmId, vmStatus, attachedISOId }: VMISOTabProps) {
  const { toast } = useToast();
  const [isos, setISOs] = useState<ISORecord[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [attachDialogOpen, setAttachDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [selectedISO, setSelectedISO] = useState<ISORecord | null>(null);
  const [isAttaching, setIsAttaching] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);

  const fetchISOs = async () => {
    try {
      setIsLoading(true);
      const data = await isoApi.listISOs(vmId);
      setISOs(data || []);
    } catch (err) {
      toast({
        title: "Error",
        description: err instanceof ApiClientError ? err.message : "Failed to load ISOs",
        variant: "destructive",
      });
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchISOs();
  }, [vmId]);

  const handleUploadComplete = (_isoId: string, fileName: string) => {
    toast({
      title: "ISO Uploaded",
      description: `${fileName} has been uploaded successfully.`,
    });
    fetchISOs();
  };

  const handleAttach = async (iso: ISORecord) => {
    setSelectedISO(iso);
    setAttachDialogOpen(true);
  };

  const confirmAttach = async () => {
    if (!selectedISO) return;
    setIsAttaching(true);
    try {
      await isoApi.attachISO(vmId, selectedISO.id);
      toast({
        title: "ISO Attached",
        description: `${selectedISO.file_name} has been attached to the VM.`,
      });
      setAttachDialogOpen(false);
      fetchISOs();
    } catch (err) {
      toast({
        title: "Error",
        description: err instanceof ApiClientError ? err.message : "Failed to attach ISO",
        variant: "destructive",
      });
    } finally {
      setIsAttaching(false);
    }
  };

  const handleDetach = async (iso: ISORecord) => {
    try {
      await isoApi.detachISO(vmId, iso.id);
      toast({
        title: "ISO Detached",
        description: `${iso.file_name} has been detached from the VM.`,
      });
      fetchISOs();
    } catch (err) {
      toast({
        title: "Error",
        description: err instanceof ApiClientError ? err.message : "Failed to detach ISO",
        variant: "destructive",
      });
    }
  };

  const handleDelete = (iso: ISORecord) => {
    setSelectedISO(iso);
    setDeleteDialogOpen(true);
  };

  const confirmDelete = async () => {
    if (!selectedISO) return;
    setIsDeleting(true);
    try {
      await isoApi.deleteISO(vmId, selectedISO.id);
      toast({
        title: "ISO Deleted",
        description: `${selectedISO.file_name} has been deleted.`,
      });
      setDeleteDialogOpen(false);
      fetchISOs();
    } catch (err) {
      toast({
        title: "Error",
        description: err instanceof ApiClientError ? err.message : "Failed to delete ISO",
        variant: "destructive",
      });
    } finally {
      setIsDeleting(false);
    }
  };

  const canAttach = vmStatus === "running" || vmStatus === "stopped";

  if (isLoading) {
    return (
      <div className="flex justify-center p-8">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <>
      <div className="space-y-6">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Upload className="h-5 w-5" />
              Upload ISO
            </CardTitle>
            <CardDescription>
              Upload an ISO image to attach to this VM. Maximum file size is 10 GB.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <ISOUpload vmId={vmId} onUploadComplete={handleUploadComplete} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <HardDrive className="h-5 w-5" />
              Uploaded ISOs
            </CardTitle>
            <CardDescription>
              Manage ISO images for this VM. Attach an ISO to use it for installation or recovery.
            </CardDescription>
          </CardHeader>
          <CardContent>
            {isos.length === 0 ? (
              <div className="text-center p-8 text-muted-foreground">
                No ISOs uploaded yet. Upload one above to get started.
              </div>
            ) : (
              <div className="space-y-4">
                {isos.map((iso) => {
                  const isAttached = iso.id === attachedISOId;
                  return (
                    <div
                      key={iso.id}
                      className="flex items-center justify-between rounded-lg border p-4"
                    >
                      <div className="flex items-center gap-4">
                        <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary/10">
                          <HardDrive className="h-5 w-5 text-primary" />
                        </div>
                        <div>
                          <div className="flex items-center gap-2">
                            <span className="font-medium">{iso.file_name}</span>
                            {isAttached && (
                              <Badge variant="default">Attached</Badge>
                            )}
                          </div>
                          <div className="flex items-center gap-4 text-sm text-muted-foreground">
                            <span>{formatBytes(iso.file_size)}</span>
                            <span>Uploaded: {new Date(iso.created_at).toLocaleDateString()}</span>
                          </div>
                        </div>
                      </div>
                      <div className="flex gap-2">
                        {isAttached ? (
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => handleDetach(iso)}
                            disabled={!canAttach}
                          >
                            <Unlink className="mr-2 h-4 w-4" />
                            Detach
                          </Button>
                        ) : (
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => handleAttach(iso)}
                            disabled={!canAttach}
                          >
                            <Link className="mr-2 h-4 w-4" />
                            Attach
                          </Button>
                        )}
                        <Button
                          variant="outline"
                          size="sm"
                          className="text-destructive hover:bg-destructive hover:text-destructive-foreground"
                          onClick={() => handleDelete(iso)}
                          disabled={isAttached}
                        >
                          <Trash2 className="mr-2 h-4 w-4" />
                          Delete
                        </Button>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Attach ISO Dialog */}
      <Dialog open={attachDialogOpen} onOpenChange={setAttachDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Attach ISO</DialogTitle>
            <DialogDescription>
              Are you sure you want to attach &quot;{selectedISO?.file_name}&quot; to this VM?
              The VM will need to be rebooted to access the ISO.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setAttachDialogOpen(false)}>
              Cancel
            </Button>
            <Button onClick={confirmAttach} disabled={isAttaching}>
              {isAttaching && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Attach
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete ISO Dialog */}
      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete ISO</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete &quot;{selectedISO?.file_name}&quot;?
              This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteDialogOpen(false)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={confirmDelete} disabled={isDeleting}>
              {isDeleting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}