"use client";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Cpu, MemoryStick, HardDrive, Network } from "lucide-react";
import type { VM } from "@/lib/api-client";
import { formatMemory } from "@/lib/vm-utils";

function parseBooleanEnv(value: string | undefined, defaultValue: boolean): boolean {
  if (value === undefined) return defaultValue;
  return value.toLowerCase() === "true";
}

const FEATURE_FLAGS = {
  enableResourceConfig: parseBooleanEnv(process.env.NEXT_PUBLIC_ENABLE_RESOURCE_CONFIG, true),
  enableNetworkConfig: parseBooleanEnv(process.env.NEXT_PUBLIC_ENABLE_NETWORK_CONFIG, true),
};

interface VMSettingsTabProps {
  vm: VM;
}

export function VMSettingsTab({ vm }: VMSettingsTabProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg">VM Settings</CardTitle>
        <CardDescription>
          View virtual machine parameters
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        {/* Basic Settings */}
        <div className="space-y-4">
          <h3 className="text-sm font-medium">Basic Information</h3>
          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="vm-name">VM Name</Label>
              <p className="text-sm">{vm.name}</p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="vm-hostname">Hostname</Label>
              <p className="text-sm text-muted-foreground">{vm.hostname}</p>
            </div>
          </div>
        </div>

        {/* Resource Configuration */}
        {FEATURE_FLAGS.enableResourceConfig && (
          <div className="space-y-4 border-t pt-6">
            <h3 className="text-sm font-medium">Resource Configuration</h3>
            <div className="grid gap-4 sm:grid-cols-3">
              <div className="rounded-lg border p-4">
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Cpu className="h-4 w-4" />
                  <span>CPU Cores</span>
                </div>
                <p className="mt-1 text-2xl font-bold">{vm.vcpu}</p>
              </div>
              <div className="rounded-lg border p-4">
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <MemoryStick className="h-4 w-4" />
                  <span>Memory</span>
                </div>
                <p className="mt-1 text-2xl font-bold">{formatMemory(vm.memory_mb)}</p>
              </div>
              <div className="rounded-lg border p-4">
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <HardDrive className="h-4 w-4" />
                  <span>Disk</span>
                </div>
                <p className="mt-1 text-2xl font-bold">{vm.disk_gb} GB</p>
              </div>
            </div>
          </div>
        )}

        {/* Network Configuration */}
        {FEATURE_FLAGS.enableNetworkConfig && (
          <div className="space-y-4 border-t pt-6">
            <h3 className="text-sm font-medium">Network Configuration</h3>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="rounded-lg border p-4">
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Network className="h-4 w-4" />
                  <span>Primary IP</span>
                </div>
                <p className="mt-1 font-mono text-lg font-semibold">{vm.ipv4 || "Not assigned"}</p>
              </div>
              <div className="rounded-lg border p-4">
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Network className="h-4 w-4" />
                  <span>Hostname</span>
                </div>
                <p className="mt-1 font-mono text-lg font-semibold">{vm.hostname || vm.name}</p>
              </div>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}