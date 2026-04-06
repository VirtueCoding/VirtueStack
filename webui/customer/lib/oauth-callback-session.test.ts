import assert from "node:assert/strict";
import { test } from "node:test";

interface TestCustomerUser {
  id: string;
  email: string;
  role: string;
}

interface OAuthCallbackSessionModule {
  OAUTH_SESSION_STATE_UNKNOWN_ERROR: string;
  finalizeOAuthSession: <TUser extends TestCustomerUser>(options: {
    user: TUser | null;
    sessionCleanupToken?: string;
    setAuthenticatedUser: (user: TUser) => boolean | void;
    invalidateSession: (sessionCleanupToken?: string) => Promise<void>;
  }) => Promise<{ user: TUser; didApplyAuthenticatedUser: true }>;
}

async function loadOAuthCallbackSessionModule(): Promise<OAuthCallbackSessionModule | null> {
  try {
    return (await import(
      new URL("./oauth-callback-session.ts", import.meta.url).href
    )) as OAuthCallbackSessionModule;
  } catch {
    return null;
  }
}

test("finalizeOAuthSession sets the authenticated user when the result is current", async () => {
  const oauthCallbackSessionModule = await loadOAuthCallbackSessionModule();
  assert.ok(
    oauthCallbackSessionModule?.finalizeOAuthSession,
    "finalizeOAuthSession should exist",
  );

  const user: TestCustomerUser = {
    id: "oauth-1",
    email: "oauth@example.com",
    role: "customer",
  };
  let authenticatedUser: TestCustomerUser | null = null;

  const result = await oauthCallbackSessionModule.finalizeOAuthSession({
    user,
    sessionCleanupToken: "oauth-cleanup-token",
    invalidateSession: async () => {},
    setAuthenticatedUser: (nextUser) => {
      authenticatedUser = nextUser;
    },
  });

  assert.deepEqual(result, {
    user,
    didApplyAuthenticatedUser: true,
  });
  assert.deepEqual(authenticatedUser, user);
});

test("finalizeOAuthSession fails closed when guarded auth application is skipped", async () => {
  const oauthCallbackSessionModule = await loadOAuthCallbackSessionModule();
  assert.ok(
    oauthCallbackSessionModule?.finalizeOAuthSession,
    "finalizeOAuthSession should exist",
  );

  const user: TestCustomerUser = {
    id: "oauth-2",
    email: "oauth-stale@example.com",
    role: "customer",
  };
  let invalidationCalls = 0;

  await assert.rejects(
    oauthCallbackSessionModule.finalizeOAuthSession({
      user,
      sessionCleanupToken: "oauth-cleanup-token-stale",
      invalidateSession: async (sessionCleanupToken) => {
        invalidationCalls += 1;
        assert.equal(sessionCleanupToken, "oauth-cleanup-token-stale");
      },
      setAuthenticatedUser: () => false,
    }),
    (error: unknown) => {
      assert.ok(error instanceof Error);
      assert.equal(
        error.message,
        "Unable to load your profile after authentication. Please log in again.",
      );
      return true;
    },
  );
  assert.equal(invalidationCalls, 1);
});

test("finalizeOAuthSession wraps stale cleanup failures as session-state-unknown errors", async () => {
  const oauthCallbackSessionModule = await loadOAuthCallbackSessionModule();
  assert.ok(
    oauthCallbackSessionModule?.finalizeOAuthSession,
    "finalizeOAuthSession should exist",
  );

  const user: TestCustomerUser = {
    id: "oauth-3",
    email: "oauth-stale-failure@example.com",
    role: "customer",
  };
  const invalidationError = new Error("logout request failed");

  await assert.rejects(
    oauthCallbackSessionModule.finalizeOAuthSession({
      user,
      sessionCleanupToken: "oauth-cleanup-token-stale",
      invalidateSession: async (sessionCleanupToken) => {
        assert.equal(sessionCleanupToken, "oauth-cleanup-token-stale");
        throw invalidationError;
      },
      setAuthenticatedUser: () => false,
    }),
    (error: unknown) => {
      assert.ok(error instanceof Error);
      assert.equal(
        error.message,
        oauthCallbackSessionModule.OAUTH_SESSION_STATE_UNKNOWN_ERROR,
      );
      assert.deepEqual(error.cause, {
        invalidationError,
        profileError: null,
      });
      return true;
    },
  );
});
