"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@virtuestack/ui";
import { useAuth } from "./auth-context";
import { getLoginRedirectMethod, getProtectedRouteView } from "./auth-bootstrap";

export function RequireAuth({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isLoading, hasBootstrapError, error } = useAuth();
  const router = useRouter();
  const protectedRouteView = getProtectedRouteView({
    isAuthenticated,
    isLoading,
    hasBootstrapError,
  });

  useEffect(() => {
    if (protectedRouteView.kind === "redirect") {
      if (getLoginRedirectMethod() === "replace") {
        router.replace(protectedRouteView.path);
      } else {
        router.push(protectedRouteView.path);
      }
    }
  }, [protectedRouteView, router]);

  if (protectedRouteView.kind === "loading") {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-foreground" />
      </div>
    );
  }

  if (protectedRouteView.kind === "retryable-error") {
    return (
      <div className="flex min-h-screen items-center justify-center px-6">
        <div className="w-full max-w-md rounded-lg border bg-background p-6 text-center shadow-sm">
          <p className="text-sm font-medium text-destructive">
            {error ?? "Unable to verify your session right now. Please try again."}
          </p>
          <div className="mt-4 flex flex-col gap-3">
            <Button type="button" onClick={() => window.location.reload()}>
              Retry
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={() => router.replace(protectedRouteView.fallbackPath)}
            >
              Back to login
            </Button>
          </div>
        </div>
      </div>
    );
  }

  if (protectedRouteView.kind !== "content") {
    return null;
  }

  return <>{children}</>;
}
