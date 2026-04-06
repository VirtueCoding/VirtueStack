"use client";

import { useState } from "react";
import { Button } from "@virtuestack/ui";
import { Input } from "@virtuestack/ui";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@virtuestack/ui";
import { Upload, Loader2 } from "lucide-react";
import { useToast } from "@virtuestack/ui";
import { apiClient } from "@/lib/api-client";
import { extractValidImportAddresses } from "./validation";
// Import IPSet type for the id/name fields used in the dropdown; cidr is a display-only field
// provided by the caller (IPSetDisplay from IPSetList).
import type { IPSet } from "@/lib/api-client";

type IPSetForImport = Pick<IPSet, "id" | "name"> & { cidr?: string };

interface IPSetImportDialogProps {
  ipSets: IPSetForImport[];
  onImportComplete: () => void;
}

export function IPSetImportDialog({ ipSets, onImportComplete }: IPSetImportDialogProps) {
  const [open, setOpen] = useState(false);
  const [importFile, setImportFile] = useState<File | null>(null);
  const [importTargetPool, setImportTargetPool] = useState("");
  const [isImporting, setIsImporting] = useState(false);
  const { toast } = useToast();

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0] || null;
    setImportFile(file);
  };

  const handleImport = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!importFile) {
      toast({
        title: "No File Selected",
        description: "Please select a CSV or text file to import.",
        variant: "destructive",
      });
      return;
    }

    if (importFile.size > 1 * 1024 * 1024) {
      toast({
        title: "File Too Large",
        description: "Import file must be under 1MB.",
        variant: "destructive",
      });
      return;
    }

    if (!importTargetPool) {
      toast({
        title: "No Pool Selected",
        description: "Please select a target pool for the imported IPs.",
        variant: "destructive",
      });
      return;
    }

    setIsImporting(true);

    try {
      const text = await importFile.text();
      const lines = text.split(/[\r\n]+/).map((line) => line.trim()).filter(Boolean);

      const ips = extractValidImportAddresses(lines);

      if (ips.length === 0) {
        toast({
          title: "No Valid IPs Found",
          description: "The file does not contain any valid IP addresses. Ensure one IP per line.",
          variant: "destructive",
        });
        setIsImporting(false);
        return;
      }

      // Call API to import IPs into the target pool
      await apiClient.postVoid(`/admin/ip-sets/${importTargetPool}/import`, { addresses: ips });

      toast({
        title: "Import Successful",
        description: `${ips.length} IP address${ips.length !== 1 ? "es" : ""} imported successfully.`,
      });

      setOpen(false);
      setImportFile(null);
      setImportTargetPool("");
      onImportComplete();
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : "Failed to import IP addresses";
      toast({
        title: "Import Failed",
        description: errorMessage,
        variant: "destructive",
      });
    } finally {
      setIsImporting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline" size="default">
          <Upload className="mr-2 h-4 w-4" />
          Import IPs
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-[525px]">
        <DialogHeader>
          <DialogTitle>Import IP Addresses</DialogTitle>
          <DialogDescription>
            Upload a CSV or text file containing IP addresses to add to a pool.
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleImport}>
          <div className="grid gap-4 py-4">
            <div className="grid gap-2">
              <label className="text-sm font-medium" htmlFor="file-upload">
                Select File
              </label>
              <div className="flex items-center gap-4">
                <Input
                  id="file-upload"
                  type="file"
                  accept=".csv,.txt"
                  onChange={handleFileChange}
                  className="flex-1"
                />
              </div>
              <p className="text-xs text-muted-foreground">
                Supported formats: CSV, TXT (one IP per line)
              </p>
            </div>
            <div className="grid gap-2">
              <label className="text-sm font-medium" htmlFor="target-pool">
                Target Pool
              </label>
              <select
                id="target-pool"
                value={importTargetPool}
                onChange={(e) => setImportTargetPool(e.target.value)}
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
              >
                <option value="">Select a pool...</option>
                {ipSets.map((set) => (
                  <option key={set.id} value={set.id}>
                    {set.name} ({set.cidr})
                  </option>
                ))}
              </select>
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setOpen(false)} disabled={isImporting}>
              Cancel
            </Button>
            <Button type="submit" disabled={isImporting}>
              {isImporting ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <Upload className="mr-2 h-4 w-4" />
              )}
              {isImporting ? "Importing..." : "Import"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}