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
