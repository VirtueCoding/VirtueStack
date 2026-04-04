import assert from "node:assert/strict";
import { test } from "node:test";

interface TestCustomerUser {
  id: string;
  email: string;
  role: string;
}

interface SessionFinalizerModule {
  CUSTOMER_PROFILE_LOAD_ERROR: string;
  CUSTOMER_SESSION_STATE_UNKNOWN_ERROR: string;
  CustomerProfileLoadError: new (cause?: unknown) => Error;
  CustomerSessionStateUnknownError: new (cause?: unknown) => Error;
  finalizeAuthenticatedSession: <TUser extends TestCustomerUser>(options: {
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
  let authenticatedUser: TestCustomerUser | null = null;

  await assert.rejects(
    sessionFinalizer.finalizeAuthenticatedSession<TestCustomerUser>({
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
      assert.ok(error instanceof sessionFinalizer.CustomerProfileLoadError);
      assert.equal(
        error.message,
        sessionFinalizer.CUSTOMER_PROFILE_LOAD_ERROR,
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
    sessionFinalizer.finalizeAuthenticatedSession<TestCustomerUser>({
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
      assert.ok(error instanceof sessionFinalizer.CustomerSessionStateUnknownError);
      assert.equal(
        error.message,
        sessionFinalizer.CUSTOMER_SESSION_STATE_UNKNOWN_ERROR,
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

  const user: TestCustomerUser = {
    id: "customer-123",
    email: "customer@example.com",
    role: "customer",
  };
  let invalidationCalls = 0;
  let authenticatedUser: TestCustomerUser | null = null;

  const result = await sessionFinalizer.finalizeAuthenticatedSession<TestCustomerUser>({
    user,
    sessionCleanupToken: "cleanup-token-customer-123",
    invalidateSession: async (_sessionCleanupToken) => {
      invalidationCalls += 1;
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
  assert.equal(invalidationCalls, 0);
});

test("finalizeAuthenticatedSession fails closed when guarded auth application was skipped", async () => {
  const sessionFinalizer = await loadSessionFinalizerModule();
  assert.ok(
    sessionFinalizer?.finalizeAuthenticatedSession,
    "finalizeAuthenticatedSession should exist",
  );

  const user: TestCustomerUser = {
    id: "customer-456",
    email: "stale@example.com",
    role: "customer",
  };
  let invalidationCalls = 0;

  await assert.rejects(
    sessionFinalizer.finalizeAuthenticatedSession<TestCustomerUser>({
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
      assert.ok(error instanceof sessionFinalizer.CustomerProfileLoadError);
      assert.equal(
        error.message,
        sessionFinalizer.CUSTOMER_PROFILE_LOAD_ERROR,
      );
      return true;
    },
  );
  assert.equal(invalidationCalls, 1);
});

test("finalizeAuthenticatedSession wraps stale cleanup failures as session-state-unknown errors", async () => {
  const sessionFinalizer = await loadSessionFinalizerModule();
  assert.ok(
    sessionFinalizer?.finalizeAuthenticatedSession,
    "finalizeAuthenticatedSession should exist",
  );

  const user: TestCustomerUser = {
    id: "customer-stale-failure",
    email: "customer-stale-failure@example.com",
    role: "customer",
  };
  const invalidationError = new Error("logout request failed");

  await assert.rejects(
    sessionFinalizer.finalizeAuthenticatedSession<TestCustomerUser>({
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
      assert.ok(error instanceof sessionFinalizer.CustomerSessionStateUnknownError);
      assert.equal(
        error.message,
        sessionFinalizer.CUSTOMER_SESSION_STATE_UNKNOWN_ERROR,
      );
      assert.deepEqual(error.cause, {
        invalidationError,
        profileError: null,
      });
      return true;
    },
  );
});
