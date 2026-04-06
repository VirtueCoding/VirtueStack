"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@virtuestack/ui";
import { useAuth } from "@/lib/auth-context";
import { getHomeRedirectPath } from "@/lib/auth-bootstrap";

export default function Home() {
  const { isAuthenticated, isLoading, hasBootstrapError, error } = useAuth();
  const router = useRouter();
  const redirectPath = getHomeRedirectPath({
    isAuthenticated,
    isLoading,
    hasBootstrapError,
  });

  useEffect(() => {
    if (redirectPath) {
      router.replace(redirectPath);
    }
  }, [redirectPath, router]);

  if (hasBootstrapError) {
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
            <Button type="button" variant="outline" onClick={() => router.replace("/login")}>
              Back to login
            </Button>
          </div>
        </div>
      </div>
    );
  }

  return null;
}
