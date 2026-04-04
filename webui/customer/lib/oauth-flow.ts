import type {
  CustomerAuthSessionResponse,
  OAuthCallbackRequest,
} from "./api-client";
import type { OAuthFlowMode, OAuthStoredState } from "./utils/oauth";

const DEFAULT_API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || "/api/v1";

const DEFAULT_RETURN_TO_BY_MODE: Record<OAuthFlowMode, string> = {
  login: "/vms",
  link: "/settings",
};

export interface StartOAuthFlowOptions {
  provider: string;
  origin: string;
  mode?: OAuthFlowMode;
  returnTo?: string;
  apiBaseURL?: string;
  generateCodeVerifier: () => string;
  generateCodeChallenge: (verifier: string) => Promise<string>;
  generateState: () => string;
  storeState: (params: Omit<OAuthStoredState, "timestamp">) => void;
}

export interface OAuthFlowHandlers {
  callback: (
    provider: string,
    request: OAuthCallbackRequest,
  ) => Promise<CustomerAuthSessionResponse>;
  link: (
    provider: string,
    request: OAuthCallbackRequest,
  ) => Promise<{ message: string }>;
}

type OAuthFlowCompletionResult =
  | {
      mode: "login";
      returnTo: string;
      tokens: CustomerAuthSessionResponse;
    }
  | {
      mode: "link";
      returnTo: string;
    };

export async function startOAuthFlow(
  options: StartOAuthFlowOptions,
): Promise<string> {
  const mode = options.mode ?? "login";
  const returnTo = options.returnTo ?? DEFAULT_RETURN_TO_BY_MODE[mode];
  const codeVerifier = options.generateCodeVerifier();
  const codeChallenge = await options.generateCodeChallenge(codeVerifier);
  const state = options.generateState();
  const redirectURI = `${options.origin}/auth/callback`;

  options.storeState({
    codeVerifier,
    state,
    provider: options.provider,
    redirectURI,
    mode,
    returnTo,
  });

  const params = new URLSearchParams({
    code_challenge: codeChallenge,
    state,
    redirect_uri: redirectURI,
  });

  return `${options.apiBaseURL ?? DEFAULT_API_BASE_URL}/customer/auth/oauth/${options.provider}/authorize?${params.toString()}`;
}

export async function completeOAuthFlow(
  stored: OAuthStoredState,
  request: OAuthCallbackRequest,
  handlers: OAuthFlowHandlers,
): Promise<OAuthFlowCompletionResult> {
  const mode = stored.mode ?? "login";
  const returnTo = stored.returnTo ?? DEFAULT_RETURN_TO_BY_MODE[mode];

  if (mode === "link") {
    await handlers.link(stored.provider, request);
    return { mode, returnTo };
  }

  const tokens = await handlers.callback(stored.provider, request);
  return { mode, returnTo, tokens };
}
