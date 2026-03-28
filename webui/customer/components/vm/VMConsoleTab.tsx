"use client";

import { useState } from "react";
import { Button } from "@virtuestack/ui";
import { Badge } from "@virtuestack/ui";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@virtuestack/ui";
import { Monitor, Loader2 } from "lucide-react";
import { VNCConsole } from "@/components/novnc-console/vnc-console";
import { vmApi, ApiClientError } from "@/lib/api-client";
import { useToast } from "@virtuestack/ui";

interface VMConsoleTabProps {
  vmId: string;
  vmStatus: string;
}

export function VMConsoleTab({ vmId, vmStatus }: VMConsoleTabProps) {
  const [consoleUrl, setConsoleUrl] = useState<string | null>(null);
  const [isConsoleLoading, setIsConsoleLoading] = useState(false);
  const { toast } = useToast();

  const handleOpenConsole = async () => {
    setIsConsoleLoading(true);
    try {
      const response = await vmApi.getConsoleToken(vmId);
      setConsoleUrl(response.url);
    } catch (error) {
      const message = error instanceof ApiClientError
        ? error.message
        : "Failed to get console access token.";
      toast({
        title: "Console Error",
        description: message,
        variant: "destructive",
      });
    } finally {
      setIsConsoleLoading(false);
    }
  };

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

  if (consoleUrl) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">VM Console</CardTitle>
          <CardDescription>
            Access the virtual machine console via noVNC
          </CardDescription>
        </CardHeader>
        <CardContent>
          <VNCConsole wsUrl={consoleUrl} vmId={vmId} className="min-h-[500px]" />
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
        <div className="flex min-h-[400px] flex-col items-center justify-center rounded-lg border border-dashed bg-muted/50">
          <div className="flex flex-col items-center gap-4 p-8 text-center">
            <div className="flex h-16 w-16 items-center justify-center rounded-full bg-primary/10">
              <Monitor className="h-8 w-8 text-primary" />
            </div>
            <div>
              <h3 className="text-lg font-semibold">Console Access</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                Connect to your VM&apos;s graphical console
              </p>
            </div>
            <Button onClick={handleOpenConsole} disabled={isConsoleLoading}>
              {isConsoleLoading ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <Monitor className="mr-2 h-4 w-4" />
              )}
              {isConsoleLoading ? "Connecting..." : "Connect to Console"}
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}