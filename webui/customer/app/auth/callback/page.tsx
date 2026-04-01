"use client";

import { useMemo, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Loader2, AlertCircle } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@virtuestack/ui";
import { Button } from "@virtuestack/ui";
import { useQuery } from "@tanstack/react-query";
import { oauthApi } from "@/lib/api-client";
import { useAuth } from "@/lib/auth-context";
import { retrieveOAuthState } from "@/lib/utils/oauth";
import { fetchCustomerProfileAfter2FA } from "@/lib/auth-utils";

function OAuthCallbackContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { setAuthenticatedUser } = useAuth();

  // Derive validation synchronously from URL params.
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
    const stored = retrieveOAuthState(state);
    if (!stored) {
      return { error: "OAuth state mismatch or expired. Please try again." };
    }
    return {
      provider: stored.provider,
      code,
      codeVerifier: stored.codeVerifier,
      redirectURI: stored.redirectURI,
      state,
    };
  }, [searchParams]);

  const hasError = "error" in callbackInput;

  // useQuery to handle the async callback (runs once, no retries).
  const { error: queryError, isLoading } = useQuery({
    queryKey: ["oauth-callback", callbackInput],
    queryFn: async () => {
      if (hasError) throw new Error(callbackInput.error);
      const input = callbackInput as {
        provider: string; code: string; codeVerifier: string;
        redirectURI: string; state: string;
      };

      await oauthApi.callback(input.provider, {
        code: input.code,
        code_verifier: input.codeVerifier,
        redirect_uri: input.redirectURI,
        state: input.state,
      });

      const user = await fetchCustomerProfileAfter2FA();
      setAuthenticatedUser(user);

      router.push("/vms");
      return true;
    },
    enabled: !hasError,
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

  if (isLoading) {
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
