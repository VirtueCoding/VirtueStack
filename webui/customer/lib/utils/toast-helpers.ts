import { useCallback } from "react";
import { useToast } from "@/components/ui/use-toast";
import { ApiClientError } from "@/lib/api-client";

export interface ToastOptions {
  title?: string;
  description: string;
}

/**
 * Hook providing standardized toast notification helpers.
 * Reduces repetitive toast patterns across mutation callbacks.
 *
 * @example
 * const { showSuccess, showError } = useToastHelpers();
 *
 * // In a mutation:
 * onError: (error) => showError(error.message),
 * onSuccess: () => showSuccess("Operation completed"),
 */
export function useToastHelpers() {
  const { toast } = useToast();

  const showSuccess = useCallback(
    (description: string, title = "Success") => {
      toast({ title, description });
    },
    [toast]
  );

  const showError = useCallback(
    (error: unknown, fallbackMessage = "An unexpected error occurred") => {
      const message = error instanceof ApiClientError
        ? error.message || fallbackMessage
        : error instanceof Error
          ? error.message
          : fallbackMessage;

      toast({
        title: "Error",
        description: message,
        variant: "destructive",
      });
    },
    [toast]
  );

  const showWarning = useCallback(
    (description: string, title = "Warning") => {
      toast({
        title,
        description,
        variant: "destructive",
      });
    },
    [toast]
  );

  return {
    toast,
    showSuccess,
    showError,
    showWarning,
  };
}

/**
 * Creates an onError callback for useMutation that shows a toast notification.
 * Useful for reducing boilerplate in mutation definitions.
 *
 * @example
 * const { createMutationOnError } = useMutationToast();
 *
 * const mutation = useMutation({
 *   mutationFn: settingsApi.updateProfile,
 *   onError: createMutationOnError("Failed to update profile"),
 * });
 */
export function useMutationToast() {
  const { toast } = useToast();

  const createMutationOnError = useCallback(
    (fallbackMessage: string) => (error: unknown) => {
      const message = error instanceof ApiClientError
        ? error.message || fallbackMessage
        : error instanceof Error
          ? error.message
          : fallbackMessage;

      toast({
        title: "Error",
        description: message,
        variant: "destructive",
      });
    },
    [toast]
  );

  const createMutationOnSuccess = useCallback(
    (message: string, title = "Success") => () => {
      toast({ title, description: message });
    },
    [toast]
  );

  return {
    createMutationOnError,
    createMutationOnSuccess,
  };
}

