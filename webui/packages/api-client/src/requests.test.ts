import assert from "node:assert/strict";
import { setTimeout as delay } from "node:timers/promises";
import { test } from "node:test";

interface APIClientModule {
  apiRequest: <T>(
    apiBaseURL: string,
    endpoint: string,
    options?: RequestInit,
  ) => Promise<T>;
}

interface RecordedFetchCall {
  input: string;
  init?: RequestInit;
}

async function loadAPIClientModule(): Promise<APIClientModule | null> {
  try {
    return (await import(
      new URL("./index.ts", import.meta.url).href
    )) as APIClientModule;
  } catch {
    return null;
  }
}

test("apiRequest bootstraps CSRF before customer and admin write requests when the cookie is missing", async (t) => {
  const apiClientModule = await loadAPIClientModule();
  assert.ok(apiClientModule, "api client module should load");

  const originalFetch = globalThis.fetch;
  const originalDocument = globalThis.document;

  await t.test("customer writes fetch the customer CSRF endpoint first", async () => {
    const fetchCalls: RecordedFetchCall[] = [];

    Object.defineProperty(globalThis, "document", {
      configurable: true,
      value: { cookie: "" } as Document,
    });

    globalThis.fetch = async (input, init) => {
      const url = typeof input === "string" ? input : String(input);
      fetchCalls.push({ input: url, init });
      if (url === "/api/v1/customer/auth/csrf") {
        globalThis.document.cookie = "csrf_token=customer-bootstrapped";
        return new Response(null, { status: 204 });
      }

      return new Response(JSON.stringify({ data: { ok: true } }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    };

    const response = await apiClientModule.apiRequest<{ ok: boolean }>(
      "/api/v1",
      "/customer/vms/vm-123/start",
      { method: "POST", body: "{}" },
    );

    assert.deepEqual(response, { ok: true });
    assert.equal(fetchCalls.length, 2);
    assert.equal(fetchCalls[0]?.input, "/api/v1/customer/auth/csrf");
    assert.equal(fetchCalls[0]?.init?.method, "GET");
    assert.equal(fetchCalls[0]?.init?.credentials, "include");
    assert.ok(fetchCalls[0]?.init?.signal instanceof AbortSignal);
    assert.equal(fetchCalls[1]?.input, "/api/v1/customer/vms/vm-123/start");
    assert.equal(fetchCalls[1]?.init?.method, "POST");
    assert.equal(fetchCalls[1]?.init?.credentials, "include");
    assert.equal(
      (fetchCalls[1]?.init?.headers as Record<string, string> | undefined)?.["X-CSRF-Token"],
      "customer-bootstrapped",
    );
  });

  await t.test("admin writes fetch the admin CSRF endpoint first", async () => {
    const fetchCalls: RecordedFetchCall[] = [];

    Object.defineProperty(globalThis, "document", {
      configurable: true,
      value: { cookie: "" } as Document,
    });

    globalThis.fetch = async (input, init) => {
      const url = typeof input === "string" ? input : String(input);
      fetchCalls.push({ input: url, init });
      if (url === "/api/v1/admin/auth/csrf") {
        globalThis.document.cookie = "csrf_token=admin-bootstrapped";
        return new Response(null, { status: 204 });
      }

      return new Response(JSON.stringify({ data: { ok: true } }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    };

    const response = await apiClientModule.apiRequest<{ ok: boolean }>(
      "/api/v1",
      "/admin/auth/verify-2fa",
      { method: "POST", body: JSON.stringify({ temp_token: "temp", totp_code: "123456" }) },
    );

    assert.deepEqual(response, { ok: true });
    assert.equal(fetchCalls.length, 2);
    assert.equal(fetchCalls[0]?.input, "/api/v1/admin/auth/csrf");
    assert.equal(fetchCalls[0]?.init?.method, "GET");
    assert.equal(fetchCalls[0]?.init?.credentials, "include");
    assert.ok(fetchCalls[0]?.init?.signal instanceof AbortSignal);
    assert.equal(fetchCalls[1]?.input, "/api/v1/admin/auth/verify-2fa");
    assert.equal(fetchCalls[1]?.init?.method, "POST");
    assert.equal(fetchCalls[1]?.init?.credentials, "include");
    assert.equal(
      (fetchCalls[1]?.init?.headers as Record<string, string> | undefined)?.["X-CSRF-Token"],
      "admin-bootstrapped",
    );
  });

  globalThis.fetch = originalFetch;
  Object.defineProperty(globalThis, "document", {
    configurable: true,
    value: originalDocument,
  });
});

test("apiRequest times out when CSRF bootstrap hangs", async () => {
  const apiClientModule = await loadAPIClientModule();
  assert.ok(apiClientModule, "api client module should load");

  const originalFetch = globalThis.fetch;
  const originalDocument = globalThis.document;
  const originalSetTimeout = globalThis.setTimeout;
  const originalClearTimeout = globalThis.clearTimeout;

  Object.defineProperty(globalThis, "document", {
    configurable: true,
    value: { cookie: "" } as Document,
  });

  globalThis.fetch = async (input, init) => {
    const url = typeof input === "string" ? input : String(input);
    if (url === "/api/v1/customer/auth/csrf") {
      return new Promise<Response>((_resolve, reject) => {
        init?.signal?.addEventListener("abort", () => {
          reject(new DOMException("The operation was aborted.", "AbortError"));
        }, { once: true });
      });
    }

    return new Response(JSON.stringify({ data: { ok: true } }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  };

  globalThis.setTimeout = ((handler: TimerHandler, _timeout?: number, ...args: unknown[]) => {
    return originalSetTimeout(handler, 0, ...args);
  }) as typeof globalThis.setTimeout;
  globalThis.clearTimeout = originalClearTimeout;

  try {
    await assert.rejects(
      apiClientModule.apiRequest("/api/v1", "/customer/vms/vm-123/start", {
        method: "POST",
        body: "{}",
      }),
      (error: unknown) =>
        error instanceof Error &&
        "code" in error &&
        "status" in error &&
        error.code === "REQUEST_TIMEOUT" &&
        error.status === 0 &&
        error.message === "Request timed out",
    );
    await delay(0);
  } finally {
    globalThis.fetch = originalFetch;
    globalThis.setTimeout = originalSetTimeout;
    globalThis.clearTimeout = originalClearTimeout;
    Object.defineProperty(globalThis, "document", {
      configurable: true,
      value: originalDocument,
    });
  }
});

test("apiRequest preserves a caller-aborted signal for the primary request fetch", async () => {
  const apiClientModule = await loadAPIClientModule();
  assert.ok(apiClientModule, "api client module should load");

  const originalFetch = globalThis.fetch;
  const abortController = new AbortController();
  const fetchCalls: RecordedFetchCall[] = [];
  abortController.abort();

  globalThis.fetch = async (input, init) => {
    const url = typeof input === "string" ? input : String(input);
    fetchCalls.push({ input: url, init });

    if (init?.signal?.aborted) {
      throw new DOMException("The operation was aborted.", "AbortError");
    }

    return new Response(JSON.stringify({ data: { ok: true } }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  };

  try {
    await assert.rejects(
      apiClientModule.apiRequest("/api/v1", "/customer/vms", {
        signal: abortController.signal,
      }),
      (error: unknown) =>
        error instanceof DOMException &&
        error.name === "AbortError" &&
        error.message === "The operation was aborted.",
    );

    assert.equal(fetchCalls.length, 1);
    assert.equal(fetchCalls[0]?.input, "/api/v1/customer/vms");
    assert.ok(fetchCalls[0]?.init?.signal instanceof AbortSignal);
    assert.equal(fetchCalls[0]?.init?.signal.aborted, true);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("apiRequest preserves a caller-aborted signal for CSRF bootstrap fetches", async () => {
  const apiClientModule = await loadAPIClientModule();
  assert.ok(apiClientModule, "api client module should load");

  const originalFetch = globalThis.fetch;
  const originalDocument = globalThis.document;
  const abortController = new AbortController();
  const fetchCalls: RecordedFetchCall[] = [];
  abortController.abort();

  Object.defineProperty(globalThis, "document", {
    configurable: true,
    value: { cookie: "" } as Document,
  });

  globalThis.fetch = async (input, init) => {
    const url = typeof input === "string" ? input : String(input);
    fetchCalls.push({ input: url, init });

    if (init?.signal?.aborted) {
      throw new DOMException("The operation was aborted.", "AbortError");
    }

    if (url === "/api/v1/customer/auth/csrf") {
      globalThis.document.cookie = "csrf_token=bootstrapped";
      return new Response(null, { status: 204 });
    }

    return new Response(JSON.stringify({ data: { ok: true } }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  };

  try {
    await assert.rejects(
      apiClientModule.apiRequest("/api/v1", "/customer/vms/vm-123/start", {
        method: "POST",
        body: "{}",
        signal: abortController.signal,
      }),
      (error: unknown) =>
        error instanceof DOMException &&
        error.name === "AbortError" &&
        error.message === "The operation was aborted.",
    );

    assert.equal(fetchCalls.length, 1);
    assert.equal(fetchCalls[0]?.input, "/api/v1/customer/auth/csrf");
    assert.ok(fetchCalls[0]?.init?.signal instanceof AbortSignal);
    assert.equal(fetchCalls[0]?.init?.signal.aborted, true);
  } finally {
    globalThis.fetch = originalFetch;
    Object.defineProperty(globalThis, "document", {
      configurable: true,
      value: originalDocument,
    });
  }
});

test("apiRequest keeps caller cancellation for the primary request even if the timeout signal aborts during handling", async () => {
  const apiClientModule = await loadAPIClientModule();
  assert.ok(apiClientModule, "api client module should load");

  const originalFetch = globalThis.fetch;
  const originalSetTimeout = globalThis.setTimeout;
  const originalClearTimeout = globalThis.clearTimeout;
  const abortController = new AbortController();
  const fetchCalls: RecordedFetchCall[] = [];
  let timeoutHandler: (() => void) | null = null;

  globalThis.setTimeout = ((handler: TimerHandler, _timeout?: number, ...args: unknown[]) => {
    timeoutHandler = () => {
      if (typeof handler === "function") {
        handler(...args);
      }
    };
    return 1 as ReturnType<typeof globalThis.setTimeout>;
  }) as typeof globalThis.setTimeout;
  globalThis.clearTimeout = (() => {}) as typeof globalThis.clearTimeout;

  globalThis.fetch = async (input, init) => {
    const url = typeof input === "string" ? input : String(input);
    fetchCalls.push({ input: url, init });

    return await new Promise<Response>((_resolve, reject) => {
      init?.signal?.addEventListener("abort", () => {
        timeoutHandler?.();
        reject(new DOMException("The operation was aborted.", "AbortError"));
      }, { once: true });

      queueMicrotask(() => abortController.abort());
    });
  };

  try {
    await assert.rejects(
      apiClientModule.apiRequest("/api/v1", "/customer/vms", {
        signal: abortController.signal,
      }),
      (error: unknown) =>
        error instanceof DOMException &&
        error.name === "AbortError" &&
        error.message === "The operation was aborted.",
    );

    assert.equal(fetchCalls.length, 1);
    assert.equal(fetchCalls[0]?.input, "/api/v1/customer/vms");
  } finally {
    globalThis.fetch = originalFetch;
    globalThis.setTimeout = originalSetTimeout;
    globalThis.clearTimeout = originalClearTimeout;
  }
});

test("apiRequest keeps caller cancellation for CSRF bootstrap even if the timeout signal aborts during handling", async () => {
  const apiClientModule = await loadAPIClientModule();
  assert.ok(apiClientModule, "api client module should load");

  const originalFetch = globalThis.fetch;
  const originalDocument = globalThis.document;
  const originalSetTimeout = globalThis.setTimeout;
  const originalClearTimeout = globalThis.clearTimeout;
  const abortController = new AbortController();
  const fetchCalls: RecordedFetchCall[] = [];
  let timeoutHandler: (() => void) | null = null;

  Object.defineProperty(globalThis, "document", {
    configurable: true,
    value: { cookie: "" } as Document,
  });

  globalThis.setTimeout = ((handler: TimerHandler, _timeout?: number, ...args: unknown[]) => {
    timeoutHandler = () => {
      if (typeof handler === "function") {
        handler(...args);
      }
    };
    return 1 as ReturnType<typeof globalThis.setTimeout>;
  }) as typeof globalThis.setTimeout;
  globalThis.clearTimeout = (() => {}) as typeof globalThis.clearTimeout;

  globalThis.fetch = async (input, init) => {
    const url = typeof input === "string" ? input : String(input);
    fetchCalls.push({ input: url, init });

    return await new Promise<Response>((_resolve, reject) => {
      init?.signal?.addEventListener("abort", () => {
        timeoutHandler?.();
        reject(new DOMException("The operation was aborted.", "AbortError"));
      }, { once: true });

      queueMicrotask(() => abortController.abort());
    });
  };

  try {
    await assert.rejects(
      apiClientModule.apiRequest("/api/v1", "/customer/vms/vm-123/start", {
        method: "POST",
        body: "{}",
        signal: abortController.signal,
      }),
      (error: unknown) =>
        error instanceof DOMException &&
        error.name === "AbortError" &&
        error.message === "The operation was aborted.",
    );

    assert.equal(fetchCalls.length, 1);
    assert.equal(fetchCalls[0]?.input, "/api/v1/customer/auth/csrf");
  } finally {
    globalThis.fetch = originalFetch;
    globalThis.setTimeout = originalSetTimeout;
    globalThis.clearTimeout = originalClearTimeout;
    Object.defineProperty(globalThis, "document", {
      configurable: true,
      value: originalDocument,
    });
  }
});
