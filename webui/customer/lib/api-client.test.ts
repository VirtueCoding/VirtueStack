import assert from "node:assert/strict";
import { test } from "node:test";

type LogoutWithRefreshRecovery = typeof import("@virtuestack/api-client").logoutWithRefreshRecovery;

declare const logoutWithRefreshRecovery: LogoutWithRefreshRecovery;

async function assertLogoutRecoveryAcceptsAuthSessionPayload(): Promise<void> {
  await logoutWithRefreshRecovery({
    invalidateSession: async () => {},
    refreshSession: async () => ({
      token_type: "Bearer",
      expires_in: 900,
      session_id: "customer-session-456",
    }),
  });
}

void assertLogoutRecoveryAcceptsAuthSessionPayload;

interface RestoreBackupResponse {
  message: string;
  task_id?: string;
}

interface CustomerAPIClientModule {
  ApiClientError: new (
    message: string,
    code: string,
    status: number,
    correlationId?: string,
  ) => Error;
  customerAuthApi: {
    logout: () => Promise<void>;
    invalidateSession: (sessionCleanupToken?: string) => Promise<void>;
    refresh: () => Promise<unknown>;
    verify2FA: (request: {
      temp_token: string;
      totp_code: string;
    }) => Promise<unknown>;
    forgotPassword: (email: string) => Promise<unknown>;
    resetPassword: (token: string, newPassword: string) => Promise<unknown>;
  };
  oauthApi: {
    link: (
      provider: string,
      request: {
        code: string;
        code_verifier: string;
        redirect_uri: string;
        state: string;
      },
    ) => Promise<unknown>;
  };
  inAppNotificationApi: {
    getUnreadCount: () => Promise<unknown>;
  };
  isoApi: {
    uploadISO: (
      vmId: string,
      file: File,
      onProgress?: (progress: number) => void,
      signal?: AbortSignal,
    ) => Promise<unknown>;
  };
  billingApi: {
    getBalance: () => Promise<unknown>;
    getTopUpConfig: () => Promise<unknown>;
    initiateTopUp: (req: {
      gateway: string;
      amount: number;
      currency: string;
    }) => Promise<unknown>;
    capturePayPalOrder: (orderID: string) => Promise<unknown>;
  };
  invoiceApi: {
    get: (id: string) => Promise<unknown>;
  };
  settingsApi: {
    initiate2FA: () => Promise<unknown>;
    enable2FA: (req: { totp_code: string }) => Promise<unknown>;
    regenerateBackupCodes: () => Promise<unknown>;
  };
  backupApi: {
    restoreBackup: (
      backupId: string,
      onProgress?: (progress: number, message?: string) => void,
    ) => Promise<RestoreBackupResponse>;
  };
  taskApi: {
    pollTaskCompletion: (
      taskId: string,
      onProgress?: (progress: number, message?: string) => void,
    ) => Promise<unknown>;
  };
  apiClient: {
    get: (endpoint: string) => Promise<unknown>;
    post: (endpoint: string, body: unknown) => Promise<unknown>;
  };
}

async function loadCustomerAPIClientModule(): Promise<CustomerAPIClientModule | null> {
  try {
    return (await import(
      new URL("./api-client.ts", import.meta.url).href
    )) as CustomerAPIClientModule;
  } catch {
    return null;
  }
}

function assertCsrfBootstrapCall(
  fetchCalls: Array<{ input: string; init?: RequestInit }>,
  expectedPath: string,
): void {
  assert.equal(fetchCalls.length, 1);
  assert.equal(fetchCalls[0]?.input, expectedPath);
  assert.equal(fetchCalls[0]?.init?.method, "GET");
  assert.equal(fetchCalls[0]?.init?.credentials, "include");
  assert.ok(fetchCalls[0]?.init?.signal instanceof AbortSignal);
}

test("customerAuthApi exposes refresh for logout recovery", async () => {
  const apiClientModule = await loadCustomerAPIClientModule();
  assert.ok(apiClientModule?.customerAuthApi, "customerAuthApi should exist");

  assert.equal(typeof apiClientModule.customerAuthApi.refresh, "function");
});

test("customerAuthApi.logout refreshes then retries logout after 401", async () => {
  const apiClientModule = await loadCustomerAPIClientModule();
  assert.ok(apiClientModule?.customerAuthApi, "customerAuthApi should exist");

  const originalPost = apiClientModule.apiClient.post;
  const requests: string[] = [];
  const unauthorizedError = new apiClientModule.ApiClientError(
    "logout unauthorized",
    "UNAUTHORIZED",
    401,
  );

  apiClientModule.apiClient.post = async (endpoint) => {
    requests.push(endpoint);
    if (endpoint === "/customer/auth/logout" && requests.length === 1) {
      throw unauthorizedError;
    }
    return {};
  };

  try {
    await assert.doesNotReject(apiClientModule.customerAuthApi.logout());
    assert.deepEqual(requests, [
      "/customer/auth/logout",
      "/customer/auth/refresh",
      "/customer/auth/logout",
    ]);
  } finally {
    apiClientModule.apiClient.post = originalPost;
  }
});

test("customerAuthApi.logout resolves when refresh confirms there is no session left", async () => {
  const apiClientModule = await loadCustomerAPIClientModule();
  assert.ok(apiClientModule?.customerAuthApi, "customerAuthApi should exist");

  const originalPost = apiClientModule.apiClient.post;
  const requests: string[] = [];
  const unauthorizedError = new apiClientModule.ApiClientError(
    "logout unauthorized",
    "UNAUTHORIZED",
    401,
  );
  const invalidRefreshError = new apiClientModule.ApiClientError(
    "refresh token missing",
    "INVALID_REFRESH_TOKEN",
    401,
  );

  apiClientModule.apiClient.post = async (endpoint) => {
    requests.push(endpoint);
    if (endpoint === "/customer/auth/logout") {
      throw unauthorizedError;
    }
    throw invalidRefreshError;
  };

  try {
    await assert.doesNotReject(apiClientModule.customerAuthApi.logout());
    assert.deepEqual(requests, [
      "/customer/auth/logout",
      "/customer/auth/refresh",
    ]);
  } finally {
    apiClientModule.apiClient.post = originalPost;
  }
});

test("customerAuthApi.logout propagates unexpected refresh failures", async () => {
  const apiClientModule = await loadCustomerAPIClientModule();
  assert.ok(apiClientModule?.customerAuthApi, "customerAuthApi should exist");

  const originalPost = apiClientModule.apiClient.post;
  const requests: string[] = [];
  const unauthorizedError = new apiClientModule.ApiClientError(
    "logout unauthorized",
    "UNAUTHORIZED",
    401,
  );
  const refreshError = new Error("refresh backend failed");

  apiClientModule.apiClient.post = async (endpoint) => {
    requests.push(endpoint);
    if (endpoint === "/customer/auth/logout") {
      throw unauthorizedError;
    }
    throw refreshError;
  };

  try {
    await assert.rejects(
      apiClientModule.customerAuthApi.logout(),
      refreshError,
    );
    assert.deepEqual(requests, [
      "/customer/auth/logout",
      "/customer/auth/refresh",
    ]);
  } finally {
    apiClientModule.apiClient.post = originalPost;
  }
});

test("customerAuthApi.invalidateSession propagates logout request failures", async () => {
  const apiClientModule = await loadCustomerAPIClientModule();
  assert.ok(apiClientModule?.customerAuthApi, "customerAuthApi should exist");

  const originalPost = apiClientModule.apiClient.post;
  const logoutError = new Error("logout request failed");

  apiClientModule.apiClient.post = async () => {
    throw logoutError;
  };

  try {
    await assert.rejects(
      apiClientModule.customerAuthApi.invalidateSession(),
      logoutError,
    );
  } finally {
    apiClientModule.apiClient.post = originalPost;
  }
});

test("customerAuthApi.invalidateSession forwards the cleanup token body", async () => {
  const apiClientModule = await loadCustomerAPIClientModule();
  assert.ok(apiClientModule?.customerAuthApi, "customerAuthApi should exist");

  const originalPost = apiClientModule.apiClient.post;
  let capturedEndpoint: string | undefined;
  let capturedBody: unknown;

  apiClientModule.apiClient.post = async (endpoint, body) => {
    capturedEndpoint = endpoint;
    capturedBody = body;
    return {};
  };

  try {
    await assert.doesNotReject(
      apiClientModule.customerAuthApi.invalidateSession("cleanup-token-456"),
    );
    assert.equal(capturedEndpoint, "/customer/auth/logout");
    assert.deepEqual(capturedBody, {
      session_cleanup_token: "cleanup-token-456",
    });
  } finally {
    apiClientModule.apiClient.post = originalPost;
  }
});

test("oauthApi.link bootstraps CSRF before posting the link callback", async () => {
  const apiClientModule = await loadCustomerAPIClientModule();
  assert.ok(apiClientModule?.oauthApi, "oauthApi should exist");

  const originalFetch = globalThis.fetch;
  const originalPost = apiClientModule.apiClient.post;
  const fetchCalls: Array<{ input: string; init?: RequestInit }> = [];
  let capturedEndpoint: string | undefined;
  let capturedBody: unknown;

  globalThis.fetch = async (input, init) => {
    fetchCalls.push({
      input: typeof input === "string" ? input : String(input),
      init,
    });
    return new Response(null, { status: 204 });
  };
  apiClientModule.apiClient.post = async (endpoint, body) => {
    capturedEndpoint = endpoint;
    capturedBody = body;
    return { message: "linked" };
  };

  try {
    const request = {
      code: "oauth-code",
      code_verifier: "oauth-verifier",
      redirect_uri: "https://customer.example.com/auth/callback",
      state: "oauth-state",
    };

    await assert.doesNotReject(apiClientModule.oauthApi.link("google", request));
    assertCsrfBootstrapCall(fetchCalls, "/api/v1/customer/auth/csrf");
    assert.equal(capturedEndpoint, "/customer/account/oauth/google/link");
    assert.deepEqual(capturedBody, request);
  } finally {
    globalThis.fetch = originalFetch;
    apiClientModule.apiClient.post = originalPost;
  }
});

test("customerAuthApi.verify2FA bootstraps CSRF before posting the verification request", async () => {
  const apiClientModule = await loadCustomerAPIClientModule();
  assert.ok(apiClientModule?.customerAuthApi, "customerAuthApi should exist");

  const originalFetch = globalThis.fetch;
  const originalPost = apiClientModule.apiClient.post;
  const fetchCalls: Array<{ input: string; init?: RequestInit }> = [];
  let capturedEndpoint: string | undefined;
  let capturedBody: unknown;

  globalThis.fetch = async (input, init) => {
    fetchCalls.push({
      input: typeof input === "string" ? input : String(input),
      init,
    });
    return new Response(null, { status: 204 });
  };
  apiClientModule.apiClient.post = async (endpoint, body) => {
    capturedEndpoint = endpoint;
    capturedBody = body;
    return {
      token_type: "Bearer",
      expires_in: 900,
      session_id: "customer-session-2fa",
    };
  };

  try {
    const request = {
      temp_token: "temp-token-123",
      totp_code: "123456",
    };

    await assert.doesNotReject(apiClientModule.customerAuthApi.verify2FA(request));
    assertCsrfBootstrapCall(fetchCalls, "/api/v1/customer/auth/csrf");
    assert.equal(capturedEndpoint, "/customer/auth/verify-2fa");
    assert.deepEqual(capturedBody, request);
  } finally {
    globalThis.fetch = originalFetch;
    apiClientModule.apiClient.post = originalPost;
  }
});

test("customerAuthApi.forgotPassword bootstraps CSRF before posting the reset request", async () => {
  const apiClientModule = await loadCustomerAPIClientModule();
  assert.ok(apiClientModule?.customerAuthApi, "customerAuthApi should exist");

  const originalFetch = globalThis.fetch;
  const originalPost = apiClientModule.apiClient.post;
  const fetchCalls: Array<{ input: string; init?: RequestInit }> = [];
  let capturedEndpoint: string | undefined;
  let capturedBody: unknown;

  globalThis.fetch = async (input, init) => {
    fetchCalls.push({
      input: typeof input === "string" ? input : String(input),
      init,
    });
    return new Response(null, { status: 204 });
  };
  apiClientModule.apiClient.post = async (endpoint, body) => {
    capturedEndpoint = endpoint;
    capturedBody = body;
    return { message: "sent" };
  };

  try {
    await assert.doesNotReject(apiClientModule.customerAuthApi.forgotPassword("user@example.com"));
    assertCsrfBootstrapCall(fetchCalls, "/api/v1/customer/auth/csrf");
    assert.equal(capturedEndpoint, "/customer/auth/forgot-password");
    assert.deepEqual(capturedBody, { email: "user@example.com" });
  } finally {
    globalThis.fetch = originalFetch;
    apiClientModule.apiClient.post = originalPost;
  }
});

test("customerAuthApi.resetPassword bootstraps CSRF before posting the new password", async () => {
  const apiClientModule = await loadCustomerAPIClientModule();
  assert.ok(apiClientModule?.customerAuthApi, "customerAuthApi should exist");

  const originalFetch = globalThis.fetch;
  const originalPost = apiClientModule.apiClient.post;
  const fetchCalls: Array<{ input: string; init?: RequestInit }> = [];
  let capturedEndpoint: string | undefined;
  let capturedBody: unknown;

  globalThis.fetch = async (input, init) => {
    fetchCalls.push({
      input: typeof input === "string" ? input : String(input),
      init,
    });
    return new Response(null, { status: 204 });
  };
  apiClientModule.apiClient.post = async (endpoint, body) => {
    capturedEndpoint = endpoint;
    capturedBody = body;
    return { message: "reset" };
  };

  try {
    await assert.doesNotReject(apiClientModule.customerAuthApi.resetPassword("reset-token", "new-password-123"));
    assertCsrfBootstrapCall(fetchCalls, "/api/v1/customer/auth/csrf");
    assert.equal(capturedEndpoint, "/customer/auth/reset-password");
    assert.deepEqual(capturedBody, {
      token: "reset-token",
      new_password: "new-password-123",
    });
  } finally {
    globalThis.fetch = originalFetch;
    apiClientModule.apiClient.post = originalPost;
  }
});

test("isoApi.uploadISO bootstraps CSRF before starting the upload request", async () => {
  const apiClientModule = await loadCustomerAPIClientModule();
  assert.ok(apiClientModule?.isoApi, "isoApi should exist");

  const originalFetch = globalThis.fetch;
  const originalXMLHttpRequest = globalThis.XMLHttpRequest;
  const originalDocument = globalThis.document;
  const fetchCalls: Array<{ input: string; init?: RequestInit }> = [];

  class FakeXMLHttpRequest {
    static lastInstance: FakeXMLHttpRequest | null = null;

    public readonly upload = { onprogress: null as ((event: ProgressEvent<EventTarget>) => void) | null };
    public withCredentials = false;
    public timeout = 0;
    public status = 200;
    public statusText = "OK";
    public responseText = JSON.stringify({
      data: { id: "iso-1", file_name: "installer.iso", file_size: 3, sha256: "abc123" },
    });
    public onload: (() => void) | null = null;
    public onerror: (() => void) | null = null;
    public ontimeout: (() => void) | null = null;
    public method: string | null = null;
    public url: string | null = null;
    public headers = new Map<string, string>();

    constructor() {
      FakeXMLHttpRequest.lastInstance = this;
    }

    open(method: string, url: string): void {
      this.method = method;
      this.url = url;
    }

    setRequestHeader(name: string, value: string): void {
      this.headers.set(name, value);
    }

    send(_body: FormData): void {
      this.onload?.();
    }

    abort(): void {}
  }

  Object.defineProperty(globalThis, "document", {
    configurable: true,
    value: { cookie: "" } as Document,
  });
  globalThis.fetch = async (input, init) => {
    fetchCalls.push({
      input: typeof input === "string" ? input : String(input),
      init,
    });
    globalThis.document.cookie = "csrf_token=upload-token";
    return new Response(null, { status: 204 });
  };
  Object.defineProperty(globalThis, "XMLHttpRequest", {
    configurable: true,
    value: FakeXMLHttpRequest,
  });

  try {
    const result = await apiClientModule.isoApi.uploadISO(
      "vm-123",
      new File([new Uint8Array([1, 2, 3])], "installer.iso", { type: "application/octet-stream" }),
    );

    assertCsrfBootstrapCall(fetchCalls, "/api/v1/customer/auth/csrf");
    assert.deepEqual(result, {
      id: "iso-1",
      file_name: "installer.iso",
      file_size: 3,
      sha256: "abc123",
    });
    assert.equal(FakeXMLHttpRequest.lastInstance?.method, "POST");
    assert.equal(FakeXMLHttpRequest.lastInstance?.url, "/api/v1/customer/vms/vm-123/iso/upload");
    assert.equal(FakeXMLHttpRequest.lastInstance?.headers.get("X-CSRF-Token"), "upload-token");
  } finally {
    globalThis.fetch = originalFetch;
    Object.defineProperty(globalThis, "XMLHttpRequest", {
      configurable: true,
      value: originalXMLHttpRequest,
    });
    Object.defineProperty(globalThis, "document", {
      configurable: true,
      value: originalDocument,
    });
  }
});

test("backupApi.restoreBackup preserves an accepted restore when task polling fails", async () => {
  const apiClientModule = await loadCustomerAPIClientModule();
  assert.ok(apiClientModule?.backupApi, "backupApi should exist");

  const acceptedResponse: RestoreBackupResponse = {
    message: "Backup restore initiated",
    task_id: "task-123",
  };

  const originalPost = apiClientModule.apiClient.post;
  const originalPollTaskCompletion = apiClientModule.taskApi.pollTaskCompletion;
  const nodeConsole = globalThis.console;
  const originalWarn = nodeConsole.warn;

  let restoreRequestCalls = 0;
  let pollCalls = 0;
  let warningCalls = 0;
  let progressUpdates = 0;

  apiClientModule.apiClient.post = async () => {
    restoreRequestCalls += 1;
    return acceptedResponse;
  };
  apiClientModule.taskApi.pollTaskCompletion = async (_taskId, onProgress) => {
    pollCalls += 1;
    onProgress?.(50, "halfway");
    throw new Error("polling failed");
  };
  nodeConsole.warn = (..._args: unknown[]) => {
    warningCalls += 1;
  };

  try {
    const response = await apiClientModule.backupApi.restoreBackup(
      "backup-123",
      () => {
        progressUpdates += 1;
      },
    );

    assert.deepEqual(response, acceptedResponse);
    assert.equal(restoreRequestCalls, 1);
    assert.equal(pollCalls, 1);
    assert.equal(progressUpdates, 1);
    assert.equal(warningCalls, 0);
  } finally {
    apiClientModule.apiClient.post = originalPost;
    apiClientModule.taskApi.pollTaskCompletion = originalPollTaskCompletion;
    nodeConsole.warn = originalWarn;
  }
});

test("customer billing, notification, and invoice helpers accept already-unwrapped API payloads", async () => {
  const apiClientModule = await loadCustomerAPIClientModule();
  assert.ok(apiClientModule, "customer API client module should load");

  const originalFetch = globalThis.fetch;
  const originalDocument = globalThis.document;
  const originalGet = apiClientModule.apiClient.get;
  const originalPost = apiClientModule.apiClient.post;

  const unreadCount = { count: 4 };
  const balance = { balance: 4200, currency: "USD" };
  const topUpConfig = {
    min_amount_cents: 500,
    max_amount_cents: 25000,
    presets: [500, 1000, 2500],
    gateways: ["stripe", "paypal"],
    currency: "USD",
  };
  const topUpResponse = {
    payment_id: "pay_123",
    payment_url: "https://payments.example.test/checkout/pay_123",
  };
  const captureResponse = {
    capture_id: "cap_123",
    status: "COMPLETED",
    amount_cents: 2500,
    currency: "USD",
  };
  const invoice = {
    id: "inv-456",
    customer_id: "cust-456",
    invoice_number: "INV-0456",
    period_start: "2026-02-01T00:00:00Z",
    period_end: "2026-02-28T23:59:59Z",
    subtotal: 2500,
    tax_amount: 0,
    total: 2500,
    currency: "USD",
    status: "issued",
    line_items: [],
    has_pdf: true,
    created_at: "2026-02-01T00:00:00Z",
    updated_at: "2026-02-01T00:00:00Z",
  };

  Object.defineProperty(globalThis, "document", {
    configurable: true,
    value: { cookie: "" } as Document,
  });
  globalThis.fetch = async (input, init) => {
    const url = typeof input === "string" ? input : String(input);
    if (url === "/api/v1/customer/auth/csrf") {
      globalThis.document.cookie = "csrf_token=customer-test-token";
      return new Response(null, { status: 204 });
    }
    throw new Error(`unexpected fetch ${url} ${String(init?.method ?? "GET")}`);
  };

  apiClientModule.apiClient.get = async (endpoint) => {
    switch (endpoint) {
      case "/customer/notifications/unread-count":
        return unreadCount;
      case "/customer/billing/balance":
        return balance;
      case "/customer/billing/top-up/config":
        return topUpConfig;
      case "/customer/invoices/inv-456":
        return invoice;
      default:
        throw new Error(`unexpected GET ${endpoint}`);
    }
  };

  apiClientModule.apiClient.post = async (endpoint) => {
    switch (endpoint) {
      case "/customer/billing/top-up":
        return topUpResponse;
      case "/customer/billing/payments/paypal/capture":
        return captureResponse;
      default:
        throw new Error(`unexpected POST ${endpoint}`);
    }
  };

  try {
    await assert.doesNotReject(async () => {
      assert.deepEqual(
        await apiClientModule.inAppNotificationApi.getUnreadCount(),
        unreadCount,
      );
      assert.deepEqual(await apiClientModule.billingApi.getBalance(), balance);
      assert.deepEqual(
        await apiClientModule.billingApi.getTopUpConfig(),
        topUpConfig,
      );
      assert.deepEqual(
        await apiClientModule.billingApi.initiateTopUp({
          gateway: "crypto",
          amount: 2500,
          currency: "USD",
        }),
        topUpResponse,
      );
      assert.deepEqual(
        await apiClientModule.billingApi.capturePayPalOrder("order_123"),
        captureResponse,
      );
      assert.deepEqual(
        await apiClientModule.invoiceApi.get("inv-456"),
        invoice,
      );
    });
  } finally {
    globalThis.fetch = originalFetch;
    Object.defineProperty(globalThis, "document", {
      configurable: true,
      value: originalDocument,
    });
    apiClientModule.apiClient.get = originalGet;
    apiClientModule.apiClient.post = originalPost;
  }
});

test("settingsApi 2FA helpers use the customer wire contract", async () => {
  const apiClientModule = await loadCustomerAPIClientModule();
  assert.ok(apiClientModule, "customer API client module should load");

  const originalPost = apiClientModule.apiClient.post;
  const originalGet = apiClientModule.apiClient.get;

  const postCalls: Array<{ endpoint: string; body: unknown }> = [];
  const getCalls: string[] = [];

  apiClientModule.apiClient.post = async (endpoint, body) => {
    postCalls.push({ endpoint, body });
    switch (endpoint) {
      case "/customer/2fa/initiate":
        return { qr_code_url: "data:image/png;base64,abc123", secret: "SECRET123" };
      case "/customer/2fa/enable":
        return { backup_codes: ["11111111", "22222222"] };
      case "/customer/2fa/backup-codes/regenerate":
        return { backup_codes: ["33333333", "44444444"] };
      default:
        throw new Error(`unexpected POST ${endpoint}`);
    }
  };

  apiClientModule.apiClient.get = async (endpoint) => {
    getCalls.push(endpoint);
    throw new Error(`unexpected GET ${endpoint}`);
  };

  try {
    assert.deepEqual(await apiClientModule.settingsApi.initiate2FA(), {
      qr_code_url: "data:image/png;base64,abc123",
      secret: "SECRET123",
    });

    assert.deepEqual(
      await apiClientModule.settingsApi.enable2FA({ totp_code: "123456" }),
      { backup_codes: ["11111111", "22222222"] },
    );

    assert.deepEqual(await apiClientModule.settingsApi.regenerateBackupCodes(), {
      backup_codes: ["33333333", "44444444"],
    });

    assert.equal("getBackupCodes" in apiClientModule.settingsApi, false);

    assert.deepEqual(postCalls, [
      { endpoint: "/customer/2fa/initiate", body: {} },
      { endpoint: "/customer/2fa/enable", body: { totp_code: "123456" } },
      { endpoint: "/customer/2fa/backup-codes/regenerate", body: {} },
    ]);
    assert.deepEqual(getCalls, []);
  } finally {
    apiClientModule.apiClient.post = originalPost;
    apiClientModule.apiClient.get = originalGet;
  }
});
