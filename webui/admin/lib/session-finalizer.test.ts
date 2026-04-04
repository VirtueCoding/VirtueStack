import assert from "node:assert/strict";
import { test } from "node:test";

interface TestAdminUser {
  id: string;
  email: string;
  role: string;
}

interface SessionFinalizerModule {
  ADMIN_PROFILE_LOAD_ERROR: string;
  ADMIN_SESSION_STATE_UNKNOWN_ERROR: string;
  AdminProfileLoadError: new (cause?: unknown) => Error;
  AdminSessionStateUnknownError: new (cause?: unknown) => Error;
  finalizeAuthenticatedSession: <TUser extends TestAdminUser>(options: {
    user: TUser | null;
    sessionCleanupToken?: string;
    setAuthenticatedUser: (user: TUser) => boolean | void;
    invalidateSession: (sessionCleanupToken?: string) => Promise<void>;
  }) => Promise<{ user: TUser; didApplyAuthenticatedUser: true }>;
}

async function loadSessionFinalizerModule(): Promise<SessionFinalizerModule | null> {
  try {
    return (await import(
      new URL("./session-finalizer.ts", import.meta.url).href
    )) as SessionFinalizerModule;
  } catch {
    return null;
  }
}

test("finalizeAuthenticatedSession revokes the session when the auth response is missing a user", async () => {
  const sessionFinalizer = await loadSessionFinalizerModule();
  assert.ok(
    sessionFinalizer?.finalizeAuthenticatedSession,
    "finalizeAuthenticatedSession should exist",
  );

  let invalidationCalls = 0;
  let invalidatedCleanupToken: string | undefined;
  let authenticatedUser: TestAdminUser | null = null;

  await assert.rejects(
    sessionFinalizer.finalizeAuthenticatedSession<TestAdminUser>({
      user: null,
      sessionCleanupToken: "cleanup-token-missing-user",
      invalidateSession: async (sessionCleanupToken) => {
        invalidationCalls += 1;
        invalidatedCleanupToken = sessionCleanupToken;
      },
      setAuthenticatedUser: (user) => {
        authenticatedUser = user;
      },
    }),
    (error: unknown) => {
      assert.ok(error instanceof Error);
      assert.ok(error instanceof sessionFinalizer.AdminProfileLoadError);
      assert.equal(
        error.message,
        sessionFinalizer.ADMIN_PROFILE_LOAD_ERROR,
      );
      return true;
    },
  );

  assert.equal(invalidationCalls, 1);
  assert.equal(invalidatedCleanupToken, "cleanup-token-missing-user");
  assert.equal(authenticatedUser, null);
});

test("finalizeAuthenticatedSession reports unknown session state when response-user cleanup fails", async () => {
  const sessionFinalizer = await loadSessionFinalizerModule();
  assert.ok(
    sessionFinalizer?.finalizeAuthenticatedSession,
    "finalizeAuthenticatedSession should exist",
  );

  const invalidationError = new Error("logout request failed");
  let invalidationCalls = 0;

  await assert.rejects(
    sessionFinalizer.finalizeAuthenticatedSession<TestAdminUser>({
      user: null,
      sessionCleanupToken: "cleanup-token-missing-user",
      invalidateSession: async () => {
        invalidationCalls += 1;
        throw invalidationError;
      },
      setAuthenticatedUser: () => {
        throw new Error("setAuthenticatedUser should not run");
      },
    }),
    (error: unknown) => {
      assert.ok(error instanceof Error);
      assert.ok(error instanceof sessionFinalizer.AdminSessionStateUnknownError);
      assert.equal(
        error.message,
        sessionFinalizer.ADMIN_SESSION_STATE_UNKNOWN_ERROR,
      );
      assert.deepEqual(error.cause, {
        invalidationError,
        profileError: null,
      });
      return true;
    },
  );

  assert.equal(invalidationCalls, 1);
});

test("finalizeAuthenticatedSession sets the authenticated user when profile loading succeeds", async () => {
  const sessionFinalizer = await loadSessionFinalizerModule();
  assert.ok(
    sessionFinalizer?.finalizeAuthenticatedSession,
    "finalizeAuthenticatedSession should exist",
  );

  const user: TestAdminUser = {
    id: "admin-123",
    email: "admin@example.com",
    role: "admin",
  };
  let logoutCalls = 0;
  let authenticatedUser: TestAdminUser | null = null;

  const result = await sessionFinalizer.finalizeAuthenticatedSession<TestAdminUser>({
    user,
    sessionCleanupToken: "cleanup-token-admin-123",
    invalidateSession: async (_sessionCleanupToken) => {
      logoutCalls += 1;
    },
    setAuthenticatedUser: (nextUser) => {
      authenticatedUser = nextUser;
    },
  });

  assert.deepEqual(result, {
    user,
    didApplyAuthenticatedUser: true,
  });
  assert.deepEqual(authenticatedUser, user);
  assert.equal(logoutCalls, 0);
});

test("finalizeAuthenticatedSession fails closed when guarded auth application was skipped", async () => {
  const sessionFinalizer = await loadSessionFinalizerModule();
  assert.ok(
    sessionFinalizer?.finalizeAuthenticatedSession,
    "finalizeAuthenticatedSession should exist",
  );

  const user: TestAdminUser = {
    id: "admin-456",
    email: "stale@example.com",
    role: "admin",
  };
  let authenticatedUser: TestAdminUser | null = null;
  let invalidationCalls = 0;

  await assert.rejects(
    sessionFinalizer.finalizeAuthenticatedSession<TestAdminUser>({
      user,
      sessionCleanupToken: "cleanup-token-stale",
      invalidateSession: async (sessionCleanupToken) => {
        invalidationCalls += 1;
        assert.equal(sessionCleanupToken, "cleanup-token-stale");
      },
      setAuthenticatedUser: () => false,
    }),
    (error: unknown) => {
      assert.ok(error instanceof Error);
      assert.ok(error instanceof sessionFinalizer.AdminProfileLoadError);
      assert.equal(
        error.message,
        sessionFinalizer.ADMIN_PROFILE_LOAD_ERROR,
      );
      return true;
    },
  );

  assert.equal(authenticatedUser, null);
  assert.equal(invalidationCalls, 1);
});

test("finalizeAuthenticatedSession wraps stale cleanup failures as session-state-unknown errors", async () => {
  const sessionFinalizer = await loadSessionFinalizerModule();
  assert.ok(
    sessionFinalizer?.finalizeAuthenticatedSession,
    "finalizeAuthenticatedSession should exist",
  );

  const user: TestAdminUser = {
    id: "admin-stale-failure",
    email: "admin-stale-failure@example.com",
    role: "admin",
  };
  const invalidationError = new Error("logout request failed");

  await assert.rejects(
    sessionFinalizer.finalizeAuthenticatedSession<TestAdminUser>({
      user,
      sessionCleanupToken: "cleanup-token-stale",
      invalidateSession: async (sessionCleanupToken) => {
        assert.equal(sessionCleanupToken, "cleanup-token-stale");
        throw invalidationError;
      },
      setAuthenticatedUser: () => false,
    }),
    (error: unknown) => {
      assert.ok(error instanceof Error);
      assert.ok(error instanceof sessionFinalizer.AdminSessionStateUnknownError);
      assert.equal(
        error.message,
        sessionFinalizer.ADMIN_SESSION_STATE_UNKNOWN_ERROR,
      );
      assert.deepEqual(error.cause, {
        invalidationError,
        profileError: null,
      });
      return true;
    },
  );
});
