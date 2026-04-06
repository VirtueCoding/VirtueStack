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
      session_id: "admin-session-123",
    }),
  });
}

void assertLogoutRecoveryAcceptsAuthSessionPayload;

interface AdminAPIClientModule {
  ApiClientError: new (
    message: string,
    code: string,
    status: number,
    correlationId?: string,
  ) => Error;
  adminAuthApi: {
    login: (credentials: { email: string; password: string }) => Promise<unknown>;
    logout: () => Promise<void>;
    invalidateSession: (sessionCleanupToken?: string) => Promise<void>;
    refresh: () => Promise<unknown>;
  };
  inAppNotificationApi: {
    getUnreadCount: () => Promise<unknown>;
  };
  adminBillingApi: {
    getConfig: () => Promise<unknown>;
    creditAdjustment: (
      customerID: string,
      amount: number,
      description: string,
    ) => Promise<unknown>;
    refundPayment: (
      paymentID: string,
      req: { amount: number; reason: string },
    ) => Promise<unknown>;
  };
  adminInvoicesApi: {
    get: (id: string) => Promise<unknown>;
    voidInvoice: (id: string) => Promise<unknown>;
  };
  adminStorageBackendsApi: {
    assignStorageBackendNodes: (id: string, nodeIDs: string[]) => Promise<unknown>;
  };
  apiClient: {
    get: (endpoint: string) => Promise<unknown>;
    post: (endpoint: string, body: unknown) => Promise<unknown>;
    postVoid: (endpoint: string, body: unknown) => Promise<unknown>;
  };
}

async function loadAdminAPIClientModule(): Promise<AdminAPIClientModule | null> {
  try {
    return (await import(
      new URL("./api-client.ts", import.meta.url).href
    )) as AdminAPIClientModule;
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

test("adminAuthApi exposes refresh for logout recovery", async () => {
  const apiClientModule = await loadAdminAPIClientModule();
  assert.ok(apiClientModule?.adminAuthApi, "adminAuthApi should exist");

  assert.equal(typeof apiClientModule.adminAuthApi.refresh, "function");
});

test("adminAuthApi.logout refreshes then retries logout after 401", async () => {
  const apiClientModule = await loadAdminAPIClientModule();
  assert.ok(apiClientModule?.adminAuthApi, "adminAuthApi should exist");

  const originalPost = apiClientModule.apiClient.post;
  const requests: string[] = [];
  const unauthorizedError = new apiClientModule.ApiClientError(
    "logout unauthorized",
    "UNAUTHORIZED",
    401,
  );

  apiClientModule.apiClient.post = async (endpoint) => {
    requests.push(endpoint);
    if (endpoint === "/admin/auth/logout" && requests.length === 1) {
      throw unauthorizedError;
    }
    return {};
  };

  try {
    await assert.doesNotReject(apiClientModule.adminAuthApi.logout());
    assert.deepEqual(requests, [
      "/admin/auth/logout",
      "/admin/auth/refresh",
      "/admin/auth/logout",
    ]);
  } finally {
    apiClientModule.apiClient.post = originalPost;
  }
});

test("adminAuthApi.logout resolves when refresh confirms there is no session left", async () => {
  const apiClientModule = await loadAdminAPIClientModule();
  assert.ok(apiClientModule?.adminAuthApi, "adminAuthApi should exist");

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
    if (endpoint === "/admin/auth/logout") {
      throw unauthorizedError;
    }
    throw invalidRefreshError;
  };

  try {
    await assert.doesNotReject(apiClientModule.adminAuthApi.logout());
    assert.deepEqual(requests, [
      "/admin/auth/logout",
      "/admin/auth/refresh",
    ]);
  } finally {
    apiClientModule.apiClient.post = originalPost;
  }
});

test("adminAuthApi.logout propagates unexpected refresh failures", async () => {
  const apiClientModule = await loadAdminAPIClientModule();
  assert.ok(apiClientModule?.adminAuthApi, "adminAuthApi should exist");

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
    if (endpoint === "/admin/auth/logout") {
      throw unauthorizedError;
    }
    throw refreshError;
  };

  try {
    await assert.rejects(
      apiClientModule.adminAuthApi.logout(),
      refreshError,
    );
    assert.deepEqual(requests, [
      "/admin/auth/logout",
      "/admin/auth/refresh",
    ]);
  } finally {
    apiClientModule.apiClient.post = originalPost;
  }
});

test("adminAuthApi.invalidateSession propagates logout request failures", async () => {
  const apiClientModule = await loadAdminAPIClientModule();
  assert.ok(apiClientModule?.adminAuthApi, "adminAuthApi should exist");

  const originalPost = apiClientModule.apiClient.post;
  const logoutError = new Error("logout request failed");

  apiClientModule.apiClient.post = async () => {
    throw logoutError;
  };

  try {
    await assert.rejects(
      apiClientModule.adminAuthApi.invalidateSession(),
      logoutError,
    );
  } finally {
    apiClientModule.apiClient.post = originalPost;
  }
});

test("adminAuthApi.invalidateSession forwards the cleanup token body", async () => {
  const apiClientModule = await loadAdminAPIClientModule();
  assert.ok(apiClientModule?.adminAuthApi, "adminAuthApi should exist");

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
      apiClientModule.adminAuthApi.invalidateSession("cleanup-token-123"),
    );
    assert.equal(capturedEndpoint, "/admin/auth/logout");
    assert.deepEqual(capturedBody, {
      session_cleanup_token: "cleanup-token-123",
    });
  } finally {
    apiClientModule.apiClient.post = originalPost;
  }
});

test("adminAuthApi.login bootstraps CSRF before posting credentials", async () => {
  const apiClientModule = await loadAdminAPIClientModule();
  assert.ok(apiClientModule?.adminAuthApi, "adminAuthApi should exist");

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
    return { token_type: "Bearer", expires_in: 900 };
  };

  try {
    const credentials = {
      email: "admin@example.com",
      password: "correct horse battery staple",
    };

    await assert.doesNotReject(apiClientModule.adminAuthApi.login(credentials));
    assertCsrfBootstrapCall(fetchCalls, "/api/v1/admin/auth/csrf");
    assert.equal(capturedEndpoint, "/admin/auth/login");
    assert.deepEqual(capturedBody, credentials);
  } finally {
    globalThis.fetch = originalFetch;
    apiClientModule.apiClient.post = originalPost;
  }
});

test("admin billing and notification helpers accept already-unwrapped API payloads", async () => {
  const apiClientModule = await loadAdminAPIClientModule();
  assert.ok(apiClientModule, "admin API client module should load");

  const originalFetch = globalThis.fetch;
  const originalDocument = globalThis.document;
  const originalGet = apiClientModule.apiClient.get;
  const originalPost = apiClientModule.apiClient.post;

  const unreadCount = { count: 7 };
  const billingConfig = {
    top_up: {
      min_amount_cents: 500,
      max_amount_cents: 50000,
      presets: [1000, 2500, 5000],
      gateways: ["stripe", "paypal"],
      currency: "USD",
    },
    gateways: ["stripe", "paypal"],
  };
  const invoice = {
    id: "inv-123",
    customer_id: "cust-123",
    invoice_number: "INV-0001",
    period_start: "2026-01-01T00:00:00Z",
    period_end: "2026-01-31T23:59:59Z",
    subtotal: 1000,
    tax_amount: 0,
    total: 1000,
    currency: "USD",
    status: "issued",
    line_items: [],
    has_pdf: true,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
  const creditAdjustment = {
    id: "txn-123",
    customer_id: "cust-123",
    type: "adjustment",
    amount: 1500,
    balance_after: 5500,
    description: "Manual credit",
    created_at: "2026-01-01T00:00:00Z",
  };
  const refundResult = {
    gateway_refund_id: "re_123",
    gateway_payment_id: "pay_123",
    amount_cents: 500,
    currency: "USD",
    status: "succeeded",
  };
  const voidResult = { status: "void" };

  Object.defineProperty(globalThis, "document", {
    configurable: true,
    value: { cookie: "" } as Document,
  });
  globalThis.fetch = async (input, init) => {
    const url = typeof input === "string" ? input : String(input);
    if (url === "/api/v1/admin/auth/csrf") {
      globalThis.document.cookie = "csrf_token=admin-test-token";
      return new Response(null, { status: 204 });
    }
    throw new Error(`unexpected fetch ${url} ${String(init?.method ?? "GET")}`);
  };

  apiClientModule.apiClient.get = async (endpoint) => {
    switch (endpoint) {
      case "/admin/notifications/unread-count":
        return unreadCount;
      case "/admin/billing/config":
        return billingConfig;
      case "/admin/invoices/inv-123":
        return invoice;
      default:
        throw new Error(`unexpected GET ${endpoint}`);
    }
  };

  apiClientModule.apiClient.post = async (endpoint) => {
    switch (endpoint) {
      case "/admin/billing/credit?customer_id=cust-123":
        return creditAdjustment;
      case "/admin/billing/refund/pay_123":
        return refundResult;
      case "/admin/invoices/inv-123/void":
        return voidResult;
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
      assert.deepEqual(
        await apiClientModule.adminBillingApi.getConfig(),
        billingConfig,
      );
      assert.deepEqual(
        await apiClientModule.adminInvoicesApi.get("inv-123"),
        invoice,
      );
      assert.deepEqual(
        await apiClientModule.adminBillingApi.creditAdjustment(
          "cust-123",
          1500,
          "Manual credit",
        ),
        creditAdjustment,
      );
      assert.deepEqual(
        await apiClientModule.adminBillingApi.refundPayment("pay_123", {
          amount: 500,
          reason: "requested",
        }),
        refundResult,
      );
      assert.deepEqual(
        await apiClientModule.adminInvoicesApi.voidInvoice("inv-123"),
        voidResult,
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

test("adminStorageBackendsApi.assignStorageBackendNodes returns updated node assignments", async () => {
  const apiClientModule = await loadAdminAPIClientModule();
  assert.ok(apiClientModule?.adminStorageBackendsApi, "adminStorageBackendsApi should exist");

  const originalPost = apiClientModule.apiClient.post;
  const originalPostVoid = apiClientModule.apiClient.postVoid;
  let capturedEndpoint: string | undefined;
  let capturedBody: unknown;
  const updatedNodes = [
    {
      node_id: "node-1",
      backend_id: "backend-1",
      hostname: "node-1.example.com",
    },
  ];

  apiClientModule.apiClient.post = async (endpoint, body) => {
    capturedEndpoint = endpoint;
    capturedBody = body;
    return updatedNodes;
  };
  apiClientModule.apiClient.postVoid = async () => {
    throw new Error("assignStorageBackendNodes should use JSON post response");
  };

  try {
    const result = await apiClientModule.adminStorageBackendsApi.assignStorageBackendNodes(
      "backend-1",
      ["node-1"],
    );

    assert.equal(capturedEndpoint, "/admin/storage-backends/backend-1/nodes");
    assert.deepEqual(capturedBody, { node_ids: ["node-1"] });
    assert.deepEqual(result, updatedNodes);
  } finally {
    apiClientModule.apiClient.post = originalPost;
    apiClientModule.apiClient.postVoid = originalPostVoid;
  }
});
