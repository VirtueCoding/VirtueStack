/**
 * Centralized API Client for VirtueStack Admin Portal
 *
 * Provides a fetch wrapper with:
 * - Automatic JWT token attachment
 * - Token refresh mechanism
 * - Standardized error handling
 * - Request/response interceptors
 */

// API Configuration
const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || "/api/v1";

// Token storage keys
const ACCESS_TOKEN_KEY = "admin_access_token";
const REFRESH_TOKEN_KEY = "admin_refresh_token";
const TOKEN_EXPIRES_KEY = "admin_token_expires";

// Types
export interface ApiError {
  code: string;
  message: string;
  correlation_id?: string;
}

export interface ApiResponse<T> {
  data: T;
}

export interface AuthTokens {
  access_token: string;
  refresh_token: string;
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

// Custom error class for API errors
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

// Token management
export const tokenStorage = {
  getAccessToken(): string | null {
    if (typeof window === "undefined") return null;
    return localStorage.getItem(ACCESS_TOKEN_KEY);
  },

  getRefreshToken(): string | null {
    if (typeof window === "undefined") return null;
    return localStorage.getItem(REFRESH_TOKEN_KEY);
  },

  getTokenExpiry(): number | null {
    if (typeof window === "undefined") return null;
    const expiry = localStorage.getItem(TOKEN_EXPIRES_KEY);
    return expiry ? parseInt(expiry, 10) : null;
  },

  setTokens(tokens: AuthTokens): void {
    if (typeof window === "undefined") return;
    localStorage.setItem(ACCESS_TOKEN_KEY, tokens.access_token);
    localStorage.setItem(REFRESH_TOKEN_KEY, tokens.refresh_token);
    const expiresAt = Date.now() + tokens.expires_in * 1000;
    localStorage.setItem(TOKEN_EXPIRES_KEY, expiresAt.toString());
  },

  setAccessToken(token: string, expiresIn: number): void {
    if (typeof window === "undefined") return;
    localStorage.setItem(ACCESS_TOKEN_KEY, token);
    const expiresAt = Date.now() + expiresIn * 1000;
    localStorage.setItem(TOKEN_EXPIRES_KEY, expiresAt.toString());
  },

  clearTokens(): void {
    if (typeof window === "undefined") return;
    localStorage.removeItem(ACCESS_TOKEN_KEY);
    localStorage.removeItem(REFRESH_TOKEN_KEY);
    localStorage.removeItem(TOKEN_EXPIRES_KEY);
  },

  isTokenExpired(): boolean {
    const expiry = this.getTokenExpiry();
    if (!expiry) return true;
    // Consider token expired 60 seconds before actual expiry
    return Date.now() >= expiry - 60000;
  },
};

// Build request headers
function buildHeaders(includeAuth = true): HeadersInit {
  const headers: HeadersInit = {
    "Content-Type": "application/json",
    "Accept": "application/json",
  };

  if (includeAuth) {
    const token = tokenStorage.getAccessToken();
    if (token) {
      headers["Authorization"] = `Bearer ${token}`;
    }
  }

  return headers;
}

// Parse API error response
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
    // If we can't parse JSON, use status-based message
    message = response.statusText || message;
  }

  return new ApiClientError(message, code, response.status, correlationId);
}

// Main API request function
export async function apiRequest<T>(
  endpoint: string,
  options: RequestInit = {},
  includeAuth = true
): Promise<T> {
  const url = `${API_BASE_URL}${endpoint}`;
  
  const config: RequestInit = {
    ...options,
    headers: {
      ...buildHeaders(includeAuth),
      ...options.headers,
    },
  };

  const response = await fetch(url, config);

  if (!response.ok) {
    const error = await parseError(response);
    throw error;
  }

  // Handle 204 No Content
  if (response.status === 204) {
    return undefined as T;
  }

  const data = await response.json();
  return data.data as T;
}

// HTTP method wrappers
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

// Admin Auth API
export const adminAuthApi = {
  /**
   * Login with email and password
   * Returns tokens or 2FA challenge (requires_2fa: true with temp_token)
   */
  async login(credentials: LoginRequest): Promise<AuthTokens> {
    return apiClient.post<AuthTokens>("/admin/auth/login", credentials, false);
  },

  /**
   * Verify 2FA code to complete login
   * Exchanges temp token for access and refresh tokens
   */
  async verify2FA(request: Verify2FARequest): Promise<AuthTokens> {
    return apiClient.post<AuthTokens>("/admin/auth/verify-2fa", request, false);
  },

  /**
   * Refresh access token using refresh token
   * Implements token rotation - new refresh token is issued
   */
  async refreshToken(refreshToken: string): Promise<AuthTokens> {
    const response = await apiClient.post<AuthTokens>(
      "/admin/auth/refresh",
      { refresh_token: refreshToken },
      false
    );
    // Update stored tokens
    tokenStorage.setTokens(response);
    return response;
  },

  /**
   * Logout - invalidates the current session
   */
  async logout(): Promise<void> {
    const refreshToken = tokenStorage.getRefreshToken();
    if (refreshToken) {
      try {
        await apiClient.post("/admin/auth/logout", { refresh_token: refreshToken }, true);
      } catch (error) {
        // Even if logout fails server-side, clear local tokens
        console.error("Logout error:", error);
      }
    }
    tokenStorage.clearTokens();
  },

  /**
   * Check if token needs refresh and refresh if necessary
   * Call this before making authenticated requests
   */
  async ensureValidToken(): Promise<string | null> {
    const token = tokenStorage.getAccessToken();
    const refreshToken = tokenStorage.getRefreshToken();

    if (!token || !refreshToken) {
      return null;
    }

    // If token is expired or about to expire, refresh it
    if (tokenStorage.isTokenExpired()) {
      try {
        const tokens = await this.refreshToken(refreshToken);
        return tokens.access_token;
      } catch (error) {
        // Refresh failed - clear tokens and return null
        tokenStorage.clearTokens();
        return null;
      }
    }

    return token;
  },
};

// Export types for use in other modules
export type { AuthTokens as AuthTokensType };

// ============================================================================
// Admin Nodes API
// ============================================================================

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
  /**
   * Get a single node by ID
   */
  async getNode(id: string): Promise<Node> {
    return apiClient.get<Node>(`/admin/nodes/${id}`);
  },

  /**
   * Drain a node - migrates all VMs to other nodes
   */
  async drainNode(id: string): Promise<void> {
    return apiClient.post<void>(`/admin/nodes/${id}/drain`, {});
  },

  /**
   * Initiate failover for a failed node
   */
  async failoverNode(id: string): Promise<void> {
    return apiClient.post<void>(`/admin/nodes/${id}/failover`, {});
  },
};

// ============================================================================
// Admin Customers API
// ============================================================================

export interface Customer {
  id: string;
  name: string;
  email: string;
  vm_count: number;
  status: "active" | "suspended";
  created_at: string;
}

export const adminCustomersApi = {
  /**
   * Suspend a customer account
   */
  async suspendCustomer(id: string): Promise<void> {
    return apiClient.post<void>(`/admin/customers/${id}/suspend`, {});
  },

  /**
   * Unsuspend a customer account
   */
  async unsuspendCustomer(id: string): Promise<void> {
    return apiClient.post<void>(`/admin/customers/${id}/unsuspend`, {});
  },

  /**
   * Delete a customer account
   */
  async deleteCustomer(id: string): Promise<void> {
    return apiClient.delete<void>(`/admin/customers/${id}`);
  },
};

// ============================================================================
// Admin Plans API
// ============================================================================

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
  /**
   * Get a single plan by ID
   */
  async getPlan(id: string): Promise<Plan> {
    return apiClient.get<Plan>(`/admin/plans/${id}`);
  },

  /**
   * Delete a plan
   */
  async deletePlan(id: string): Promise<void> {
    return apiClient.delete<void>(`/admin/plans/${id}`);
  },

  /**
   * Create a new plan
   */
  async createPlan(plan: CreatePlanRequest): Promise<Plan> {
    return apiClient.post<Plan>("/admin/plans", plan);
  },

  /**
   * Update an existing plan
   */
  async updatePlan(id: string, plan: UpdatePlanRequest): Promise<Plan> {
    return apiClient.patch<Plan>(`/admin/plans/${id}`, plan);
  },
};
