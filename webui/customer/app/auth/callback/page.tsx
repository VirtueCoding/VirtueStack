"use client";

import { useEffect, useMemo, useState, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Loader2, AlertCircle } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@virtuestack/ui";
import { Button } from "@virtuestack/ui";
import { useQuery } from "@tanstack/react-query";
import { customerAuthApi, oauthApi } from "@/lib/api-client";
import { useAuth } from "@/lib/auth-context";
import { retrieveOAuthState } from "@/lib/utils/oauth";
import { fetchCustomerProfileAfter2FA } from "@/lib/auth-utils";
import { finalizeOAuthSession } from "@/lib/oauth-callback-session";

function OAuthCallbackContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { setAuthenticatedUser } = useAuth();
  const [hasMounted, setHasMounted] = useState(false);

  useEffect(() => {
    setHasMounted(true);
  }, []);

  // Validate only URL parameters during render. Browser-only PKCE state is
  // consumed after mount so SSR does not touch sessionStorage.
  const callbackInput = useMemo(() => {
    const errorParam = searchParams.get("error");
    if (errorParam) {
      return {
        error: `OAuth provider returned an error: ${searchParams.get("error_description") || errorParam}`,
      };
    }
    const code = searchParams.get("code");
    const state = searchParams.get("state");
    if (!code || !state) {
      return { error: "Missing authorization code or state parameter." };
    }
    return {
      code,
      state,
    };
  }, [searchParams]);

  const hasError = "error" in callbackInput;

  // useQuery to handle the async callback (runs once, no retries).
  const { error: queryError, isLoading } = useQuery({
    queryKey: ["oauth-callback", callbackInput],
    queryFn: async () => {
      if (hasError) throw new Error(callbackInput.error);
      const input = callbackInput as { code: string; state: string };
      const stored = retrieveOAuthState(input.state);
      if (!stored) {
        throw new Error("OAuth state mismatch or expired. Please try again.");
      }

      await oauthApi.callback(stored.provider, {
        code: input.code,
        code_verifier: stored.codeVerifier,
        redirect_uri: stored.redirectURI,
        state: input.state,
      });

      await finalizeOAuthSession({
        loadUser: fetchCustomerProfileAfter2FA,
        setAuthenticatedUser,
        logout: customerAuthApi.logout,
      });

      router.push("/vms");
      return true;
    },
    enabled: hasMounted && !hasError,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnMount: false,
    staleTime: Infinity,
  });

  const displayError = hasError
    ? callbackInput.error
    : queryError instanceof Error
      ? queryError.message
      : queryError
        ? "OAuth authentication failed."
        : null;

  if (displayError) {
    return (
      <div className="flex min-h-screen items-center justify-center p-4 bg-gradient-to-br from-blue-50 to-indigo-100 dark:from-gray-900 dark:to-blue-950">
        <Card className="w-full max-w-md shadow-xl">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-destructive">
              <AlertCircle className="h-5 w-5" />
              Authentication Failed
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <p className="text-sm text-muted-foreground">{displayError}</p>
            <Button
              onClick={() => router.push("/login")}
              className="w-full"
            >
              Back to Login
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  if (!hasMounted || isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center p-4 bg-gradient-to-br from-blue-50 to-indigo-100 dark:from-gray-900 dark:to-blue-950">
        <Card className="w-full max-w-md shadow-xl">
          <CardContent className="flex flex-col items-center justify-center py-12">
            <Loader2 className="h-8 w-8 animate-spin text-primary mb-4" />
            <p className="text-muted-foreground">
              Completing authentication...
            </p>
          </CardContent>
        </Card>
      </div>
    );
  }

  return null;
}

export default function OAuthCallbackPage() {
  return (
    <Suspense
      fallback={
        <div className="flex min-h-screen items-center justify-center p-4 bg-gradient-to-br from-blue-50 to-indigo-100 dark:from-gray-900 dark:to-blue-950">
          <Card className="w-full max-w-md shadow-xl">
            <CardContent className="flex flex-col items-center justify-center py-12">
              <Loader2 className="h-8 w-8 animate-spin text-primary mb-4" />
              <p className="text-muted-foreground">Loading...</p>
            </CardContent>
          </Card>
        </div>
      }
    >
      <OAuthCallbackContent />
    </Suspense>
  );
}
