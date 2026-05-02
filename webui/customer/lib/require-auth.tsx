"use client";

import { useCallback } from "react";
import { useRouter } from "next/navigation";
import { RequireAuthGate } from "@virtuestack/ui";

import { useAuth } from "./auth-context";

export function RequireAuth({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isLoading } = useAuth();
  const router = useRouter();
  const redirectToLogin = useCallback(() => {
    router.push("/login");
  }, [router]);

  return (
    <RequireAuthGate
      isAuthenticated={isAuthenticated}
      isLoading={isLoading}
      onUnauthenticated={redirectToLogin}
    >
      {children}
    </RequireAuthGate>
  );
}
