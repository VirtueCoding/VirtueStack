"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Loader2, Link2, Unlink, ExternalLink } from "lucide-react";
import { Button } from "@virtuestack/ui";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@virtuestack/ui";
import { oauthApi, type OAuthLink } from "@/lib/api-client";

const OAUTH_GOOGLE_ENABLED =
  process.env.NEXT_PUBLIC_OAUTH_GOOGLE_ENABLED === "true";
const OAUTH_GITHUB_ENABLED =
  process.env.NEXT_PUBLIC_OAUTH_GITHUB_ENABLED === "true";

const providerLabels: Record<string, string> = {
  google: "Google",
  github: "GitHub",
};

export function OAuthLinksSection() {
  const queryClient = useQueryClient();
  const [unlinking, setUnlinking] = useState<string | null>(null);

  const { data: links, isLoading } = useQuery({
    queryKey: ["oauth-links"],
    queryFn: () => oauthApi.listLinks(),
    enabled: OAUTH_GOOGLE_ENABLED || OAUTH_GITHUB_ENABLED,
  });

  const unlinkMutation = useMutation({
    mutationFn: (provider: string) => oauthApi.unlink(provider),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["oauth-links"] });
      setUnlinking(null);
    },
    onError: () => {
      setUnlinking(null);
    },
  });

  if (!OAUTH_GOOGLE_ENABLED && !OAUTH_GITHUB_ENABLED) return null;

  const linkedProviders = new Set(
    (links || []).map((l: OAuthLink) => l.provider)
  );

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Link2 className="h-5 w-5" />
          Linked Accounts
        </CardTitle>
        <CardDescription>
          Manage your linked OAuth accounts for single sign-on
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {isLoading ? (
          <div className="flex items-center justify-center py-4">
            <Loader2 className="h-5 w-5 animate-spin" />
          </div>
        ) : (
          <div className="space-y-3">
            {OAUTH_GOOGLE_ENABLED && (
              <ProviderRow
                provider="google"
                link={links?.find(
                  (l: OAuthLink) => l.provider === "google"
                )}
                isLinked={linkedProviders.has("google")}
                isUnlinking={unlinking === "google"}
                onUnlink={() => {
                  setUnlinking("google");
                  unlinkMutation.mutate("google");
                }}
              />
            )}
            {OAUTH_GITHUB_ENABLED && (
              <ProviderRow
                provider="github"
                link={links?.find(
                  (l: OAuthLink) => l.provider === "github"
                )}
                isLinked={linkedProviders.has("github")}
                isUnlinking={unlinking === "github"}
                onUnlink={() => {
                  setUnlinking("github");
                  unlinkMutation.mutate("github");
                }}
              />
            )}
          </div>
        )}

        {unlinkMutation.isError && (
          <p className="text-sm text-destructive mt-2">
            Failed to unlink account. You may need to set a password first.
          </p>
        )}
      </CardContent>
    </Card>
  );
}

interface ProviderRowProps {
  provider: string;
  link?: OAuthLink;
  isLinked: boolean;
  isUnlinking: boolean;
  onUnlink: () => void;
}

function ProviderRow({
  provider,
  link,
  isLinked,
  isUnlinking,
  onUnlink,
}: ProviderRowProps) {
  const label = providerLabels[provider] || provider;

  return (
    <div className="flex items-center justify-between rounded-lg border p-4">
      <div className="flex items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-full bg-muted">
          {provider === "google" ? (
            <ExternalLink className="h-5 w-5" />
          ) : (
            <ExternalLink className="h-5 w-5" />
          )}
        </div>
        <div>
          <p className="font-medium">{label}</p>
          {isLinked && link?.email ? (
            <p className="text-sm text-muted-foreground">{link.email}</p>
          ) : (
            <p className="text-sm text-muted-foreground">Not connected</p>
          )}
        </div>
      </div>

      {isLinked ? (
        <Button
          variant="outline"
          size="sm"
          onClick={onUnlink}
          disabled={isUnlinking}
          className="text-destructive hover:text-destructive"
        >
          {isUnlinking ? (
            <Loader2 className="mr-1 h-3 w-3 animate-spin" />
          ) : (
            <Unlink className="mr-1 h-3 w-3" />
          )}
          Unlink
        </Button>
      ) : (
        <p className="text-sm text-muted-foreground">
          Sign in with {label} to link
        </p>
      )}
    </div>
  );
}
