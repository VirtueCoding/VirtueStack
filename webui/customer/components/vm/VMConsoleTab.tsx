"use client";

import { Badge } from "@virtuestack/ui";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@virtuestack/ui";
import { Monitor } from "lucide-react";
import { VNCConsole } from "@/components/novnc-console/vnc-console";

interface VMConsoleTabProps {
  vmId: string;
  vmStatus: string;
}

export function VMConsoleTab({ vmId, vmStatus }: VMConsoleTabProps) {
  if (vmStatus !== "running") {
    return (
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
              <div className="flex h-16 w-16 items-center justify-center rounded-full bg-muted">
                <Monitor className="h-8 w-8 text-muted-foreground" />
              </div>
              <div>
                <h3 className="text-lg font-semibold">Console Unavailable</h3>
                <p className="mt-1 text-sm text-muted-foreground">
                  VM must be running to access the console
                </p>
              </div>
              <Badge variant="secondary" className="gap-1">
                <div className="h-2 w-2 rounded-full bg-yellow-500" />
                VM {vmStatus || "Unknown"}
              </Badge>
            </div>
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg">VM Console</CardTitle>
        <CardDescription>
          Access the virtual machine console via noVNC
        </CardDescription>
      </CardHeader>
      <CardContent>
        <VNCConsole vmId={vmId} className="min-h-[500px]" />
      </CardContent>
    </Card>
  );
}
