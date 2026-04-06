import assert from "node:assert/strict";
import { test } from "node:test";

interface AuthBootstrapModule {
  advanceAuthVersion: (currentVersion: number) => number;
  canApplyBootstrapResult: (
    bootstrapVersion: number,
    currentVersion: number,
  ) => boolean;
  applyAuthenticatedUserIfCurrent: <TUser>(
    user: TUser,
    expectedVersion: number,
    currentVersion: number,
    applyAuthenticatedUser: (user: TUser) => void,
  ) => boolean;
  getCancelled2FAState: () => {
    requires2FA: boolean;
    isLoading: boolean;
  };
  getProfileBootstrapErrorState: () => {
    user: null;
    isAuthenticated: boolean;
    isLoading: boolean;
    requires2FA: boolean;
    hasBootstrapError: boolean;
  };
  shouldRedirectToLogin: (state: {
    isAuthenticated: boolean;
    isLoading: boolean;
    hasBootstrapError: boolean;
  }) => boolean;
  getProtectedRouteView: (state: {
    isAuthenticated: boolean;
    isLoading: boolean;
    hasBootstrapError: boolean;
  }) =>
    | { kind: "loading" }
    | { kind: "content" }
    | { kind: "redirect"; path: "/login" }
      | { kind: "retryable-error"; fallbackPath: "/login"; allowRetry: true };
  shouldRevalidateSession: (state: {
    isAuthenticated: boolean;
    isLoading: boolean;
    requires2FA: boolean;
    hasBootstrapError: boolean;
    lastRevalidatedAtMs: number;
    nowMs: number;
    minIntervalMs?: number;
    force?: boolean;
  }) => boolean;
  getAuthSyncAction: (rawEvent: string | null) => "clear-auth" | null;
  getLoginRedirectMethod: () => string;
  applyRevalidationResultIfCurrent: <TUser>(
    result: TUser,
    expectedRequestId: number,
    latestRequestId: number,
    applyResult: (result: TUser) => void,
  ) => boolean;
  shouldPublishSessionInvalidated: (state: {
    isAuthenticated: boolean;
  }) => boolean;
}

async function loadAuthBootstrapModule(): Promise<AuthBootstrapModule | null> {
  try {
    return (await import(
      new URL("./auth-bootstrap.ts", import.meta.url).href
    )) as AuthBootstrapModule;
  } catch {
    return null;
  }
}

test("canApplyBootstrapResult rejects stale admin bootstrap results", async () => {
  const authBootstrapModule = await loadAuthBootstrapModule();
  assert.ok(authBootstrapModule, "auth-bootstrap module should load");

  assert.equal(authBootstrapModule.canApplyBootstrapResult(2, 2), true);
  assert.equal(authBootstrapModule.canApplyBootstrapResult(2, 3), false);
});

test("advanceAuthVersion invalidates bootstrap results after entering admin 2FA", async () => {
  const authBootstrapModule = await loadAuthBootstrapModule();
  assert.ok(authBootstrapModule, "auth-bootstrap module should load");

  const nextVersion = authBootstrapModule.advanceAuthVersion(10);

  assert.equal(nextVersion, 11);
  assert.equal(authBootstrapModule.canApplyBootstrapResult(10, nextVersion), false);
});

test("getCancelled2FAState leaves the admin login form interactive after cancel", async () => {
  const authBootstrapModule = await loadAuthBootstrapModule();
  assert.ok(authBootstrapModule, "auth-bootstrap module should load");

  assert.deepEqual(authBootstrapModule.getCancelled2FAState(), {
    requires2FA: false,
    isLoading: false,
  });
});

test("getProfileBootstrapErrorState preserves retryable admin bootstrap failure state", async () => {
  const authBootstrapModule = await loadAuthBootstrapModule();
  assert.ok(authBootstrapModule, "auth-bootstrap module should load");

  assert.deepEqual(authBootstrapModule.getProfileBootstrapErrorState(), {
    user: null,
    isAuthenticated: false,
    isLoading: false,
    requires2FA: false,
    hasBootstrapError: true,
  });
});

test("shouldRedirectToLogin skips redirects while admin bootstrap verification is retryable", async () => {
  const authBootstrapModule = await loadAuthBootstrapModule();
  assert.ok(authBootstrapModule, "auth-bootstrap module should load");

  assert.equal(
    authBootstrapModule.shouldRedirectToLogin({
      isAuthenticated: false,
      isLoading: false,
      hasBootstrapError: true,
    }),
    false,
  );
  assert.equal(
    authBootstrapModule.shouldRedirectToLogin({
      isAuthenticated: false,
      isLoading: false,
      hasBootstrapError: false,
    }),
    true,
  );
});

test("getProtectedRouteView exposes admin recovery actions for retryable bootstrap failures", async () => {
  const authBootstrapModule = await loadAuthBootstrapModule();
  assert.ok(authBootstrapModule, "auth-bootstrap module should load");

  assert.deepEqual(
    authBootstrapModule.getProtectedRouteView({
      isAuthenticated: false,
      isLoading: true,
      hasBootstrapError: false,
    }),
    { kind: "loading" },
  );
  assert.deepEqual(
    authBootstrapModule.getProtectedRouteView({
      isAuthenticated: true,
      isLoading: false,
      hasBootstrapError: false,
    }),
    { kind: "content" },
  );
  assert.deepEqual(
    authBootstrapModule.getProtectedRouteView({
      isAuthenticated: false,
      isLoading: false,
      hasBootstrapError: false,
    }),
    { kind: "redirect", path: "/login" },
  );
  assert.deepEqual(
    authBootstrapModule.getProtectedRouteView({
      isAuthenticated: false,
      isLoading: false,
      hasBootstrapError: true,
    }),
    { kind: "retryable-error", fallbackPath: "/login", allowRetry: true },
  );
});

test("shouldRevalidateSession only retries admin session checks when state is stale and security-relevant", async () => {
  const authBootstrapModule = await loadAuthBootstrapModule();
  assert.ok(authBootstrapModule, "auth-bootstrap module should load");

  assert.equal(
    authBootstrapModule.shouldRevalidateSession({
      isAuthenticated: true,
      isLoading: false,
      requires2FA: false,
      hasBootstrapError: false,
      lastRevalidatedAtMs: 0,
      nowMs: 20_000,
      minIntervalMs: 10_000,
    }),
    true,
  );
  assert.equal(
    authBootstrapModule.shouldRevalidateSession({
      isAuthenticated: false,
      isLoading: false,
      requires2FA: false,
      hasBootstrapError: true,
      lastRevalidatedAtMs: 0,
      nowMs: 20_000,
      minIntervalMs: 10_000,
    }),
    true,
  );
  assert.equal(
    authBootstrapModule.shouldRevalidateSession({
      isAuthenticated: true,
      isLoading: true,
      requires2FA: false,
      hasBootstrapError: false,
      lastRevalidatedAtMs: 0,
      nowMs: 20_000,
      minIntervalMs: 10_000,
    }),
    false,
  );
  assert.equal(
    authBootstrapModule.shouldRevalidateSession({
      isAuthenticated: true,
      isLoading: false,
      requires2FA: true,
      hasBootstrapError: false,
      lastRevalidatedAtMs: 0,
      nowMs: 20_000,
      minIntervalMs: 10_000,
    }),
    false,
  );
  assert.equal(
    authBootstrapModule.shouldRevalidateSession({
      isAuthenticated: false,
      isLoading: false,
      requires2FA: false,
      hasBootstrapError: false,
      lastRevalidatedAtMs: 0,
      nowMs: 20_000,
      minIntervalMs: 10_000,
    }),
    false,
  );
  assert.equal(
    authBootstrapModule.shouldRevalidateSession({
      isAuthenticated: true,
      isLoading: false,
      requires2FA: false,
      hasBootstrapError: false,
      lastRevalidatedAtMs: 15_500,
      nowMs: 20_000,
      minIntervalMs: 10_000,
    }),
    false,
  );
  assert.equal(
    authBootstrapModule.shouldRevalidateSession({
      isAuthenticated: true,
      isLoading: false,
      requires2FA: false,
      hasBootstrapError: false,
      lastRevalidatedAtMs: 19_500,
      nowMs: 20_000,
      minIntervalMs: 10_000,
      force: true,
    }),
    true,
  );
});

test("getAuthSyncAction clears admin auth for logout-style cross-tab events only", async () => {
  const authBootstrapModule = await loadAuthBootstrapModule();
  assert.ok(authBootstrapModule, "auth-bootstrap module should load");

  assert.equal(
    authBootstrapModule.getAuthSyncAction(JSON.stringify({ type: "logout", at: 1 })),
    "clear-auth",
  );
  assert.equal(
    authBootstrapModule.getAuthSyncAction(
      JSON.stringify({ type: "session-invalidated", at: 2 }),
    ),
    "clear-auth",
  );
  assert.equal(
    authBootstrapModule.getAuthSyncAction(JSON.stringify({ type: "login", at: 3 })),
    null,
  );
  assert.equal(authBootstrapModule.getAuthSyncAction("not-json"), null);
  assert.equal(authBootstrapModule.getAuthSyncAction(null), null);
});

test("applyAuthenticatedUserIfCurrent skips stale admin 2FA completions", async () => {
  const authBootstrapModule = await loadAuthBootstrapModule();
  assert.ok(authBootstrapModule, "auth-bootstrap module should load");

  let appliedUser: { id: string } | null = null;

  const didApply = authBootstrapModule.applyAuthenticatedUserIfCurrent(
    { id: "admin-1" },
    4,
    5,
    (user) => {
      appliedUser = user;
    },
  );

  assert.equal(didApply, false);
  assert.equal(appliedUser, null);
});

test("applyAuthenticatedUserIfCurrent applies current admin 2FA completions", async () => {
  const authBootstrapModule = await loadAuthBootstrapModule();
  assert.ok(authBootstrapModule, "auth-bootstrap module should load");

  let appliedUser: { id: string } | null = null;

  const didApply = authBootstrapModule.applyAuthenticatedUserIfCurrent(
    { id: "admin-2" },
    5,
    5,
    (user) => {
      appliedUser = user;
    },
  );

  assert.equal(didApply, true);
  assert.deepEqual(appliedUser, { id: "admin-2" });
});

test("applyRevalidationResultIfCurrent ignores stale admin revalidation responses", async () => {
  const authBootstrapModule = await loadAuthBootstrapModule();
  assert.ok(authBootstrapModule, "auth-bootstrap module should load");

  let appliedResult: { id: string } | null = null;

  const didApply = authBootstrapModule.applyRevalidationResultIfCurrent(
    { id: "admin-stale" },
    2,
    3,
    (result) => {
      appliedResult = result;
    },
  );

  assert.equal(didApply, false);
  assert.equal(appliedResult, null);
});

test("applyRevalidationResultIfCurrent applies only the latest admin revalidation response", async () => {
  const authBootstrapModule = await loadAuthBootstrapModule();
  assert.ok(authBootstrapModule, "auth-bootstrap module should load");

  let appliedResult: { id: string } | null = null;

  const didApply = authBootstrapModule.applyRevalidationResultIfCurrent(
    { id: "admin-latest" },
    4,
    4,
    (result) => {
      appliedResult = result;
    },
  );

  assert.equal(didApply, true);
  assert.deepEqual(appliedResult, { id: "admin-latest" });
});

test("shouldPublishSessionInvalidated only broadcasts for authenticated admin sessions", async () => {
  const authBootstrapModule = await loadAuthBootstrapModule();
  assert.ok(authBootstrapModule, "auth-bootstrap module should load");

  assert.equal(
    authBootstrapModule.shouldPublishSessionInvalidated({ isAuthenticated: true }),
    true,
  );
  assert.equal(
    authBootstrapModule.shouldPublishSessionInvalidated({ isAuthenticated: false }),
    false,
  );
});

test("getLoginRedirectMethod replaces history for admin auth guard redirects", async () => {
  const authBootstrapModule = await loadAuthBootstrapModule();
  assert.ok(authBootstrapModule, "auth-bootstrap module should load");

  assert.equal(authBootstrapModule.getLoginRedirectMethod(), "replace");
});
