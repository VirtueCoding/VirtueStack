const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || "/api/v1";

export interface ApiError {
  code: string;
  message: string;
  correlation_id?: string;
}

export interface ApiResponse<T> {
  data: T;
}

export interface AuthTokens {
  token_type: string;
  expires_in: number;
  requires_2fa?: boolean;
  temp_token?: string;
}

export interface LoginRequest {
  email: string;
  password: string;
}

export interface Verify2FARequest {
  temp_token: string;
  totp_code: string;
}

export class ApiClientError extends Error {
  public readonly code: string;
  public readonly status: number;
  public readonly correlationId?: string;

  constructor(message: string, code: string, status: number, correlationId?: string) {
    super(message);
    this.name = "ApiClientError";
    this.code = code;
    this.status = status;
    this.correlationId = correlationId;
  }
}

function getCsrfToken(): string | null {
  if (typeof document === "undefined") return null;
  const match = document.cookie.match(/csrf_token=([^;]+)/);
  return match ? decodeURIComponent(match[1]) : null;
}

async function fetchCsrfToken(): Promise<void> {
  try {
    await fetch(`${API_BASE_URL}/admin/nodes`, { method: "GET", credentials: "include" });
  } catch {
    // CSRF token will be set in cookie
  }
}

function buildHeaders(includeAuth = true, includeCsrf = false): HeadersInit {
  const headers: HeadersInit = {
    "Content-Type": "application/json",
    "Accept": "application/json",
  };

  if (includeCsrf) {
    const csrfToken = getCsrfToken();
    if (csrfToken) {
      headers["X-CSRF-Token"] = csrfToken;
    }
  }

  return headers;
}

async function parseError(response: Response): Promise<ApiClientError> {
  let code = "UNKNOWN_ERROR";
  let message = "An unexpected error occurred";
  let correlationId: string | undefined;

  try {
    const data = await response.json();
    if (data.error) {
      code = data.error.code || code;
      message = data.error.message || message;
      correlationId = data.error.correlation_id;
    }
  } catch {
    message = response.statusText || message;
  }

  return new ApiClientError(message, code, response.status, correlationId);
}

export async function apiRequest<T>(
  endpoint: string,
  options: RequestInit = {},
  includeAuth = true
): Promise<T> {
  const url = `${API_BASE_URL}${endpoint}`;
  const isStateChanging = ["POST", "PUT", "PATCH", "DELETE"].includes(
    (options.method || "GET").toUpperCase()
  );

  const config: RequestInit = {
    ...options,
    credentials: "include",
    headers: {
      ...buildHeaders(includeAuth, isStateChanging),
      ...options.headers,
    },
  };

  const response = await fetch(url, config);

  if (!response.ok) {
    const error = await parseError(response);
    throw error;
  }

  if (response.status === 204) {
    return undefined as T;
  }

  const data = await response.json();
  return data.data as T;
}

export const apiClient = {
  get<T>(endpoint: string, includeAuth = true): Promise<T> {
    return apiRequest<T>(endpoint, { method: "GET" }, includeAuth);
  },

  post<T>(endpoint: string, body: unknown, includeAuth = true): Promise<T> {
    return apiRequest<T>(
      endpoint,
      { method: "POST", body: JSON.stringify(body) },
      includeAuth
    );
  },

  put<T>(endpoint: string, body: unknown, includeAuth = true): Promise<T> {
    return apiRequest<T>(
      endpoint,
      { method: "PUT", body: JSON.stringify(body) },
      includeAuth
    );
  },

  patch<T>(endpoint: string, body: unknown, includeAuth = true): Promise<T> {
    return apiRequest<T>(
      endpoint,
      { method: "PATCH", body: JSON.stringify(body) },
      includeAuth
    );
  },

  delete<T>(endpoint: string, includeAuth = true): Promise<T> {
    return apiRequest<T>(endpoint, { method: "DELETE" }, includeAuth);
  },
};

function getAccessTokenFromCookie(): string | null {
  if (typeof document === "undefined") return null;
  const match = document.cookie.match(/(?:^|;\s*)vs_access_token=([^;]*)/);
  return match ? decodeURIComponent(match[1]) : null;
}

function decodeJWTPayload(token: string): { exp?: number } | null {
  try {
    const base64Url = token.split(".")[1];
    if (!base64Url) return null;
    const base64 = base64Url.replace(/-/g, "+").replace(/_/g, "/");
    const padded = base64.padEnd(
      base64.length + ((4 - (base64.length % 4)) % 4),
      "="
    );
    const json = atob(padded);
    return JSON.parse(json);
  } catch {
    return null;
  }
}

let tokenValidUntil = 0;

export const adminAuthApi = {
  async login(credentials: LoginRequest): Promise<AuthTokens> {
    await fetchCsrfToken();
    return apiClient.post<AuthTokens>("/admin/auth/login", credentials, false);
  },

  async verify2FA(request: Verify2FARequest): Promise<AuthTokens> {
    return apiClient.post<AuthTokens>("/admin/auth/verify-2fa", request, false);
  },

  async refreshToken(): Promise<AuthTokens> {
    return apiClient.post<AuthTokens>("/admin/auth/refresh", {}, false);
  },

  async logout(): Promise<void> {
    try {
      await apiClient.post("/admin/auth/logout", {}, true);
    } catch {
    }
    tokenValidUntil = 0;
  },

  async ensureValidToken(): Promise<boolean> {
    const token = getAccessTokenFromCookie();

    if (token) {
      const payload = decodeJWTPayload(token);
      if (payload && typeof payload.exp === "number") {
        const now = Math.floor(Date.now() / 1000);
        if (payload.exp - now >= 60) {
          return true;
        }
      }
    }

    if (Date.now() < tokenValidUntil) {
      return true;
    }

    try {
      const tokens = await adminAuthApi.refreshToken();
      tokenValidUntil =
        Date.now() + Math.max((tokens.expires_in || 900) - 60, 60) * 1000;
      return true;
    } catch {
      tokenValidUntil = 0;
      return false;
    }
  },
};

export type { AuthTokens as AuthTokensType };

export interface Node {
  id: string;
  name: string;
  hostname: string;
  status: "online" | "offline" | "draining" | "failed";
  location: string;
  vm_count: number;
  cpu_total: number;
  cpu_allocated: number;
  memory_total_gb: number;
  memory_allocated_gb: number;
}

export const adminNodesApi = {
  async getNodes(): Promise<Node[]> {
    return apiClient.get<Node[]>("/admin/nodes");
  },

  async getNode(id: string): Promise<Node> {
    return apiClient.get<Node>(`/admin/nodes/${id}`);
  },

  async drainNode(id: string): Promise<void> {
    return apiClient.post<void>(`/admin/nodes/${id}/drain`, {});
  },

  async failoverNode(id: string): Promise<void> {
    return apiClient.post<void>(`/admin/nodes/${id}/failover`, {});
  },
};

export interface Customer {
  id: string;
  name: string;
  email: string;
  vm_count: number;
  status: "active" | "suspended";
  created_at: string;
}

export const adminCustomersApi = {
  async getCustomers(): Promise<Customer[]> {
    return apiClient.get<Customer[]>("/admin/customers");
  },

  async suspendCustomer(id: string): Promise<void> {
    return apiClient.post<void>(`/admin/customers/${id}/suspend`, {});
  },

  async unsuspendCustomer(id: string): Promise<void> {
    return apiClient.post<void>(`/admin/customers/${id}/unsuspend`, {});
  },

  async deleteCustomer(id: string): Promise<void> {
    return apiClient.delete<void>(`/admin/customers/${id}`);
  },
};

export interface Plan {
  id: string;
  name: string;
  vcpu: number;
  memory_mb: number;
  disk_gb: number;
  bandwidth_mbps: number;
  price_monthly: number;
  status: "active" | "inactive";
}

export interface CreatePlanRequest {
  name: string;
  vcpu: number;
  memory_mb: number;
  disk_gb: number;
  bandwidth_mbps: number;
  price_monthly: number;
}

export interface UpdatePlanRequest {
  name?: string;
  vcpu?: number;
  memory_mb?: number;
  disk_gb?: number;
  bandwidth_mbps?: number;
  price_monthly?: number;
  status?: "active" | "inactive";
}

export const adminPlansApi = {
  async getPlans(): Promise<Plan[]> {
    return apiClient.get<Plan[]>("/admin/plans");
  },

  async getPlan(id: string): Promise<Plan> {
    return apiClient.get<Plan>(`/admin/plans/${id}`);
  },

  async deletePlan(id: string): Promise<void> {
    return apiClient.delete<void>(`/admin/plans/${id}`);
  },

  async createPlan(plan: CreatePlanRequest): Promise<Plan> {
    return apiClient.post<Plan>("/admin/plans", plan);
  },

  async updatePlan(id: string, plan: UpdatePlanRequest): Promise<Plan> {
    return apiClient.patch<Plan>(`/admin/plans/${id}`, plan);
  },
};

export interface AuditLog {
  id: string;
  timestamp: string;
  actor_id?: string;
  actor_type: string;
  actor_ip?: string;
  action: string;
  resource_type: string;
  resource_id?: string;
  success: boolean;
  error_message?: string;
}

export const adminAuditLogsApi = {
  async getAuditLogs(): Promise<AuditLog[]> {
    return apiClient.get<AuditLog[]>('/admin/audit-logs');
  },
};

export interface VM {
  id: string;
  name: string;
  customer_id: string;
  node_id: string;
  status: string;
  created_at: string;
}

export const adminVMsApi = {
  async getVMs(): Promise<VM[]> {
    return apiClient.get<VM[]>('/admin/vms');
  }
};