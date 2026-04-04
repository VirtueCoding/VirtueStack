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
  getProfileBootstrapErrorState: (
    stored: {
      user: { id: string; email: string; role: string };
      isAuthenticated: boolean;
    } | null,
  ) => {
    user: { id: string; email: string; role: string } | null;
    isAuthenticated: boolean;
    isLoading: boolean;
    requires2FA: boolean;
  };
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

test("getProfileBootstrapErrorState fails closed even when cached auth exists", async () => {
  const authBootstrapModule = await loadAuthBootstrapModule();
  assert.ok(authBootstrapModule, "auth-bootstrap module should load");

  assert.deepEqual(
    authBootstrapModule.getProfileBootstrapErrorState({
      user: {
        id: "customer-1",
        email: "user@example.com",
        role: "customer",
      },
      isAuthenticated: true,
    }),
    {
      user: null,
      isAuthenticated: false,
      isLoading: false,
      requires2FA: false,
    },
  );
});

test("canApplyBootstrapResult rejects stale bootstrap results", async () => {
  const authBootstrapModule = await loadAuthBootstrapModule();
  assert.ok(authBootstrapModule, "auth-bootstrap module should load");

  assert.equal(authBootstrapModule.canApplyBootstrapResult(3, 3), true);
  assert.equal(authBootstrapModule.canApplyBootstrapResult(3, 4), false);
});

test("advanceAuthVersion invalidates in-flight bootstrap results for newer auth transitions", async () => {
  const authBootstrapModule = await loadAuthBootstrapModule();
  assert.ok(authBootstrapModule, "auth-bootstrap module should load");

  const nextVersion = authBootstrapModule.advanceAuthVersion(7);

  assert.equal(nextVersion, 8);
  assert.equal(authBootstrapModule.canApplyBootstrapResult(7, nextVersion), false);
});

test("getCancelled2FAState leaves the customer login form interactive after cancel", async () => {
  const authBootstrapModule = await loadAuthBootstrapModule();
  assert.ok(authBootstrapModule, "auth-bootstrap module should load");

  assert.deepEqual(authBootstrapModule.getCancelled2FAState(), {
    requires2FA: false,
    isLoading: false,
  });
});

test("applyAuthenticatedUserIfCurrent skips stale customer 2FA completions", async () => {
  const authBootstrapModule = await loadAuthBootstrapModule();
  assert.ok(authBootstrapModule, "auth-bootstrap module should load");

  let appliedUser: { id: string } | null = null;

  const didApply = authBootstrapModule.applyAuthenticatedUserIfCurrent(
    { id: "customer-1" },
    9,
    10,
    (user) => {
      appliedUser = user;
    },
  );

  assert.equal(didApply, false);
  assert.equal(appliedUser, null);
});

test("applyAuthenticatedUserIfCurrent applies current customer 2FA completions", async () => {
  const authBootstrapModule = await loadAuthBootstrapModule();
  assert.ok(authBootstrapModule, "auth-bootstrap module should load");

  let appliedUser: { id: string } | null = null;

  const didApply = authBootstrapModule.applyAuthenticatedUserIfCurrent(
    { id: "customer-2" },
    10,
    10,
    (user) => {
      appliedUser = user;
    },
  );

  assert.equal(didApply, true);
  assert.deepEqual(appliedUser, { id: "customer-2" });
});

test("getProfileBootstrapErrorState falls back to signed-out state without stored auth", async () => {
  const authBootstrapModule = await loadAuthBootstrapModule();
  assert.ok(authBootstrapModule, "auth-bootstrap module should load");

  assert.deepEqual(
    authBootstrapModule.getProfileBootstrapErrorState(null),
    {
      user: null,
      isAuthenticated: false,
      isLoading: false,
      requires2FA: false,
    },
  );
});
