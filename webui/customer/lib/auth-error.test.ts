import assert from "node:assert/strict";
import { test } from "node:test";

interface AuthErrorModule {
  shouldTreatProfileErrorAsUnauthenticated: (error: unknown) => boolean;
}

class FakeApiClientError extends Error {
  public readonly code: string;
  public readonly status: number;

  constructor(message: string, code: string, status: number) {
    super(message);
    this.name = "ApiClientError";
    this.code = code;
    this.status = status;
  }
}

async function loadAuthErrorModule(): Promise<AuthErrorModule | null> {
  try {
    return (await import(
      new URL("./auth-error.ts", import.meta.url).href
    )) as AuthErrorModule;
  } catch {
    return null;
  }
}

test("shouldTreatProfileErrorAsUnauthenticated returns true for 401 and 403 responses", async () => {
  const authErrorModule = await loadAuthErrorModule();
  assert.ok(authErrorModule, "auth-error module should load");

  assert.equal(
    authErrorModule.shouldTreatProfileErrorAsUnauthenticated(
      new FakeApiClientError("Unauthorized", "UNAUTHORIZED", 401),
    ),
    true,
  );
  assert.equal(
    authErrorModule.shouldTreatProfileErrorAsUnauthenticated(
      new FakeApiClientError("Forbidden", "FORBIDDEN", 403),
    ),
    true,
  );
});

test("shouldTreatProfileErrorAsUnauthenticated returns false for transient failures", async () => {
  const authErrorModule = await loadAuthErrorModule();
  assert.ok(authErrorModule, "auth-error module should load");

  assert.equal(
    authErrorModule.shouldTreatProfileErrorAsUnauthenticated(
      new FakeApiClientError("Internal error", "INTERNAL_ERROR", 500),
    ),
    false,
  );
  assert.equal(
    authErrorModule.shouldTreatProfileErrorAsUnauthenticated(new Error("network")),
    false,
  );
});
