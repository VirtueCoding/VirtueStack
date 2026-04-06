import assert from "node:assert/strict";
import { test } from "node:test";

interface SessionStorageLike {
  readonly length: number;
  getItem(key: string): string | null;
  key(index: number): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
  clear(): void;
}

interface OAuthModule {
  storeOAuthState: (params: {
    codeVerifier: string;
    state: string;
    provider: string;
    redirectURI: string;
    mode?: "login" | "link";
    returnTo?: string;
  }) => void;
  retrieveOAuthState: (expectedState: string) => {
    codeVerifier: string;
    state: string;
    provider: string;
    redirectURI: string;
    timestamp: number;
    mode?: "login" | "link";
    returnTo?: string;
  } | null;
}

class MemorySessionStorage implements SessionStorageLike {
  private readonly store = new Map<string, string>();

  get length(): number {
    return this.store.size;
  }

  getItem(key: string): string | null {
    return this.store.get(key) ?? null;
  }

  key(index: number): string | null {
    return Array.from(this.store.keys())[index] ?? null;
  }

  setItem(key: string, value: string): void {
    this.store.set(key, value);
  }

  removeItem(key: string): void {
    this.store.delete(key);
  }

  clear(): void {
    this.store.clear();
  }
}

async function loadOAuthModule(): Promise<OAuthModule | null> {
  try {
    return (await import(new URL("./oauth.ts", import.meta.url).href)) as OAuthModule;
  } catch {
    return null;
  }
}

test("retrieveOAuthState keeps concurrent OAuth flows isolated until each callback consumes its own state", async () => {
  const oauthModule = await loadOAuthModule();
  assert.ok(oauthModule, "oauth utility module should load");

  const sessionStorage = new MemorySessionStorage();
  const originalSessionStorage = globalThis.sessionStorage;
  Object.defineProperty(globalThis, "sessionStorage", {
    configurable: true,
    value: sessionStorage,
  });

  try {
    oauthModule.storeOAuthState({
      codeVerifier: "verifier-one",
      state: "state-one",
      provider: "google",
      redirectURI: "https://customer.example.com/auth/callback",
      mode: "login",
      returnTo: "/vms",
    });
    oauthModule.storeOAuthState({
      codeVerifier: "verifier-two",
      state: "state-two",
      provider: "github",
      redirectURI: "https://customer.example.com/auth/callback",
      mode: "link",
      returnTo: "/settings",
    });

    const firstResult = oauthModule.retrieveOAuthState("state-one");
    assert.ok(firstResult, "first OAuth state should be returned");
    assert.equal(typeof firstResult.timestamp, "number");
    assert.deepEqual({ ...firstResult, timestamp: 0 }, {
      codeVerifier: "verifier-one",
      state: "state-one",
      provider: "google",
      redirectURI: "https://customer.example.com/auth/callback",
      timestamp: 0,
      mode: "login",
      returnTo: "/vms",
    });

    const secondResult = oauthModule.retrieveOAuthState("state-two");
    assert.ok(secondResult, "second OAuth state should remain available");
    assert.equal(typeof secondResult.timestamp, "number");
    assert.deepEqual({ ...secondResult, timestamp: 0 }, {
      codeVerifier: "verifier-two",
      state: "state-two",
      provider: "github",
      redirectURI: "https://customer.example.com/auth/callback",
      timestamp: 0,
      mode: "link",
      returnTo: "/settings",
    });
  } finally {
    Object.defineProperty(globalThis, "sessionStorage", {
      configurable: true,
      value: originalSessionStorage,
    });
  }
});
