import assert from "node:assert/strict";
import { test } from "node:test";

interface TestCustomerAuthSessionResponse {
  token_type: string;
  expires_in: number;
  session_cleanup_token?: string;
}

interface TestOAuthCallbackRequest {
  code: string;
  code_verifier: string;
  redirect_uri: string;
  state: string;
}

interface TestOAuthStoredState {
  codeVerifier: string;
  state: string;
  provider: string;
  redirectURI: string;
  timestamp: number;
  mode?: "login" | "link";
  returnTo?: string;
}

interface OAuthFlowModule {
  startOAuthFlow: (options: {
    provider: string;
    origin: string;
    mode?: "login" | "link";
    returnTo?: string;
    apiBaseURL?: string;
    generateCodeVerifier?: () => string;
    generateCodeChallenge?: (verifier: string) => Promise<string>;
    generateState?: () => string;
    storeState?: (params: Omit<TestOAuthStoredState, "timestamp">) => void;
  }) => Promise<string>;
  completeOAuthFlow: (
    stored: TestOAuthStoredState,
    request: TestOAuthCallbackRequest,
    handlers: {
      callback: (
        provider: string,
        request: TestOAuthCallbackRequest,
      ) => Promise<TestCustomerAuthSessionResponse>;
      link: (
        provider: string,
        request: TestOAuthCallbackRequest,
      ) => Promise<{ message: string }>;
    },
  ) => Promise<
    | {
        mode: "login";
        returnTo: string;
        tokens: TestCustomerAuthSessionResponse;
      }
    | {
        mode: "link";
        returnTo: string;
      }
  >;
}

async function loadOAuthFlowModule(): Promise<OAuthFlowModule | null> {
  try {
    return (await import(
      new URL("./oauth-flow.ts", import.meta.url).href
    )) as OAuthFlowModule;
  } catch {
    return null;
  }
}

test("startOAuthFlow stores link state with a settings return target", async () => {
  const oauthFlowModule = await loadOAuthFlowModule();
  assert.ok(oauthFlowModule?.startOAuthFlow, "startOAuthFlow should exist");

  const storedStates: Array<Omit<TestOAuthStoredState, "timestamp">> = [];
  const authorizeURL = await oauthFlowModule.startOAuthFlow({
    provider: "google",
    origin: "https://customer.example.com",
    mode: "link",
    apiBaseURL: "/api/v1",
    generateCodeVerifier: () => "verifier-123",
    generateCodeChallenge: async () => "challenge-456",
    generateState: () => "state-789",
    storeState: (params) => {
      storedStates.push(params);
    },
  });

  assert.equal(
    authorizeURL,
    "/api/v1/customer/auth/oauth/google/authorize?code_challenge=challenge-456&state=state-789&redirect_uri=https%3A%2F%2Fcustomer.example.com%2Fauth%2Fcallback",
  );
  assert.deepEqual(storedStates, [
    {
      codeVerifier: "verifier-123",
      state: "state-789",
      provider: "google",
      redirectURI: "https://customer.example.com/auth/callback",
      mode: "link",
      returnTo: "/settings",
    },
  ]);
});

test("completeOAuthFlow routes link callbacks to the account-link endpoint", async () => {
  const oauthFlowModule = await loadOAuthFlowModule();
  assert.ok(oauthFlowModule?.completeOAuthFlow, "completeOAuthFlow should exist");

  const request: TestOAuthCallbackRequest = {
    code: "oauth-code",
    code_verifier: "oauth-verifier",
    redirect_uri: "https://customer.example.com/auth/callback",
    state: "oauth-state",
  };
  const stored: TestOAuthStoredState = {
    codeVerifier: "oauth-verifier",
    state: "oauth-state",
    provider: "github",
    redirectURI: "https://customer.example.com/auth/callback",
    timestamp: Date.now(),
    mode: "link",
    returnTo: "/settings",
  };

  const calls = {
    callback: 0,
    link: 0,
  };

  const result = await oauthFlowModule.completeOAuthFlow(stored, request, {
    callback: async () => {
      calls.callback += 1;
      return {
        token_type: "Bearer",
        expires_in: 900,
      };
    },
    link: async (provider, nextRequest) => {
      calls.link += 1;
      assert.equal(provider, "github");
      assert.deepEqual(nextRequest, request);
      return { message: "linked" };
    },
  });

  assert.deepEqual(result, {
    mode: "link",
    returnTo: "/settings",
  });
  assert.deepEqual(calls, {
    callback: 0,
    link: 1,
  });
});

test("completeOAuthFlow defaults login callbacks to the auth session exchange", async () => {
  const oauthFlowModule = await loadOAuthFlowModule();
  assert.ok(oauthFlowModule?.completeOAuthFlow, "completeOAuthFlow should exist");

  const request: TestOAuthCallbackRequest = {
    code: "oauth-code",
    code_verifier: "oauth-verifier",
    redirect_uri: "https://customer.example.com/auth/callback",
    state: "oauth-state",
  };
  const stored: TestOAuthStoredState = {
    codeVerifier: "oauth-verifier",
    state: "oauth-state",
    provider: "google",
    redirectURI: "https://customer.example.com/auth/callback",
    timestamp: Date.now(),
  };

  const expectedTokens: TestCustomerAuthSessionResponse = {
    token_type: "Bearer",
    expires_in: 900,
    session_cleanup_token: "cleanup-token",
  };

  const result = await oauthFlowModule.completeOAuthFlow(stored, request, {
    callback: async (provider, nextRequest) => {
      assert.equal(provider, "google");
      assert.deepEqual(nextRequest, request);
      return expectedTokens;
    },
    link: async () => {
      assert.fail("link flow should not be used for login callbacks");
    },
  });

  assert.deepEqual(result, {
    mode: "login",
    returnTo: "/vms",
    tokens: expectedTokens,
  });
});
