import { useState, useCallback } from "react";
import { useToast } from "@virtuestack/ui";
import { vmApi, ApiClientError } from "@/lib/api-client";

export type VMAction = "start" | "stop" | "forceStop" | "restart";

interface VMActionConfig {
  action: VMAction;
  vmId: string;
  onSuccess?: () => void;
  successMessage?: string;
  errorMessage?: string;
}

const DEFAULT_MESSAGES: Record<VMAction, { success: string; error: string }> = {
  start: {
    success: "Virtual machine started successfully.",
    error: "Failed to start VM. Please try again.",
  },
  stop: {
    success: "Virtual machine stopped successfully.",
    error: "Failed to stop VM. Please try again.",
  },
  forceStop: {
    success: "Virtual machine force stopped successfully.",
    error: "Failed to force stop VM. Please try again.",
  },
  restart: {
    success: "Virtual machine restarted successfully.",
    error: "Failed to restart VM. Please try again.",
  },
};

const ACTION_TITLES: Record<VMAction, string> = {
  start: "VM Started",
  stop: "VM Stopped",
  forceStop: "VM Force Stopped",
  restart: "VM Restarted",
};

/**
 * Hook for executing VM actions with consistent loading state, toast notifications,
 * and error handling. Reduces boilerplate for start/stop/restart/forceStop operations.
 *
 * @example
 * const { executeAction, isLoading, loadingVMId } = useVMAction();
 *
 * // In a click handler:
 * await executeAction({
 *   action: "start",
 *   vmId: vm.id,
 *   onSuccess: () => refetch(),
 * });
 */
export function useVMAction() {
  const [isLoading, setIsLoading] = useState(false);
  const [loadingVMId, setLoadingVMId] = useState<string | null>(null);
  const { toast } = useToast();

  const executeAction = useCallback(
    async (config: VMActionConfig): Promise<boolean> => {
      const {
        action,
        vmId,
        onSuccess,
        successMessage,
        errorMessage,
      } = config;

      const messages = DEFAULT_MESSAGES[action];

      setIsLoading(true);
      setLoadingVMId(vmId);

      try {
        switch (action) {
          case "start":
            await vmApi.startVM(vmId);
            break;
          case "stop":
            await vmApi.stopVM(vmId);
            break;
          case "forceStop":
            await vmApi.forceStopVM(vmId);
            break;
          case "restart":
            await vmApi.restartVM(vmId);
            break;
        }

        toast({
          title: ACTION_TITLES[action],
          description: successMessage || messages.success,
        });

        onSuccess?.();
        return true;
      } catch (error) {
        const message = error instanceof ApiClientError
          ? error.message
          : errorMessage || messages.error;

        toast({
          title: "Error",
          description: message,
          variant: "destructive",
        });
        return false;
      } finally {
        setIsLoading(false);
        setLoadingVMId(null);
      }
    },
    [toast]
  );

  return {
    executeAction,
    isLoading,
    loadingVMId,
    isVMLoading: (vmId: string) => loadingVMId === vmId,
  };
}
