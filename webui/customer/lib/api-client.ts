/**
 * Centralized API Client for VirtueStack Customer Portal
 *
 * Provides a fetch wrapper with:
 * - Automatic JWT token attachment
 * - Token refresh mechanism
 * - Standardized error handling
 * - Request/response interceptors
 */

// API Configuration
const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || "/api/v1";

// Token storage keys - customer-specific
const ACCESS_TOKEN_KEY = "customer_access_token";
const REFRESH_TOKEN_KEY = "customer_refresh_token";
const TOKEN_EXPIRES_KEY = "customer_token_expires";

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

// Customer Auth API
export const customerAuthApi = {
  /**
   * Login with email and password
   * Returns tokens or 2FA challenge (requires_2fa: true with temp_token)
   */
  async login(credentials: LoginRequest): Promise<AuthTokens> {
    return apiClient.post<AuthTokens>("/customer/auth/login", credentials, false);
  },

  /**
   * Verify 2FA code to complete login
   * Exchanges temp token for access and refresh tokens
   */
  async verify2FA(request: Verify2FARequest): Promise<AuthTokens> {
    return apiClient.post<AuthTokens>("/customer/auth/verify-2fa", request, false);
  },

  /**
   * Refresh access token using refresh token
   * Implements token rotation - new refresh token is issued
   */
  async refreshToken(refreshToken: string): Promise<AuthTokens> {
    const response = await apiClient.post<AuthTokens>(
      "/customer/auth/refresh",
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
        await apiClient.post("/customer/auth/logout", { refresh_token: refreshToken }, true);
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

// VM Operation Types
export interface VMOperationResponse {
  message: string;
}

export interface VM {
  id: string;
  name: string;
  hostname: string;
  status: "running" | "stopped" | "error" | "provisioning";
  ipv4: string;
  vcpu: number;
  memory_mb: number;
  disk_gb: number;
}

// VM API
export const vmApi = {
  /**
   * Start a stopped VM
   * POST /customer/vms/:id/start
   */
  async startVM(vmId: string): Promise<VMOperationResponse> {
    return apiClient.post<VMOperationResponse>(`/customer/vms/${vmId}/start`, {});
  },

  /**
   * Stop a running VM (graceful ACPI shutdown)
   * POST /customer/vms/:id/stop
   */
  async stopVM(vmId: string): Promise<VMOperationResponse> {
    return apiClient.post<VMOperationResponse>(`/customer/vms/${vmId}/stop`, {});
  },

  /**
   * Force stop a VM (equivalent to pulling power plug)
   * POST /customer/vms/:id/force-stop
   */
  async forceStopVM(vmId: string): Promise<VMOperationResponse> {
    return apiClient.post<VMOperationResponse>(`/customer/vms/${vmId}/force-stop`, {});
  },

  /**
   * Restart a running VM (graceful shutdown then start)
   * POST /customer/vms/:id/restart
   */
  async restartVM(vmId: string): Promise<VMOperationResponse> {
    return apiClient.post<VMOperationResponse>(`/customer/vms/${vmId}/restart`, {});
  },

  /**
   * Get all VMs for the current customer
   * GET /customer/vms
   */
  async getVMs(): Promise<VM[]> {
    return apiClient.get<VM[]>("/customer/vms");
  },

  /**
   * Get a specific VM by ID
   * GET /customer/vms/:id
   */
  async getVM(vmId: string): Promise<VM> {
    return apiClient.get<VM>(`/customer/vms/${vmId}`);
  },
};

// Backup Types
export interface Backup {
  id: string;
  vm_id: string;
  name: string;
  type: "full" | "incremental";
  size_bytes: number;
  status: "pending" | "creating" | "completed" | "failed" | "restoring";
  created_at: string;
  expires_at?: string;
  completed_at?: string;
}

export interface CreateBackupRequest {
  vm_id: string;
  name: string;
}

export interface CreateBackupResponse {
  id: string;
  vm_id: string;
  status: "creating";
}

// Snapshot Types
export interface Snapshot {
  id: string;
  vm_id: string;
  name: string;
  size_bytes: number;
  status: "active" | "creating" | "deleting";
  created_at: string;
}

export interface CreateSnapshotRequest {
  vm_id: string;
  name: string;
}

export interface CreateSnapshotResponse {
  id: string;
  vm_id: string;
  status: "creating";
}

// Backup API
export const backupApi = {
  /**
   * List all backups for the current customer
   * GET /customer/backups
   */
  async listBackups(vmId?: string): Promise<Backup[]> {
    const params = vmId ? `?vm_id=${vmId}` : "";
    return apiClient.get<Backup[]>(`/customer/backups${params}`);
  },

  /**
   * Create a new backup
   * POST /customer/backups
   */
  async createBackup(request: CreateBackupRequest): Promise<CreateBackupResponse> {
    return apiClient.post<CreateBackupResponse>("/customer/backups", request);
  },

  /**
   * Delete a backup
   * DELETE /customer/backups/:id
   */
  async deleteBackup(backupId: string): Promise<void> {
    return apiClient.delete<void>(`/customer/backups/${backupId}`);
  },

  /**
   * Restore a backup
   * POST /customer/backups/:id/restore
   */
  async restoreBackup(backupId: string): Promise<{ message: string }> {
    return apiClient.post<{ message: string }>(`/customer/backups/${backupId}/restore`, {});
  },
};

// Snapshot API
export const snapshotApi = {
  /**
   * List all snapshots for the current customer
   * GET /customer/snapshots
   */
  async listSnapshots(vmId?: string): Promise<Snapshot[]> {
    const params = vmId ? `?vm_id=${vmId}` : "";
    return apiClient.get<Snapshot[]>(`/customer/snapshots${params}`);
  },

  /**
   * Create a new snapshot
   * POST /customer/snapshots
   */
  async createSnapshot(request: CreateSnapshotRequest): Promise<CreateSnapshotResponse> {
    return apiClient.post<CreateSnapshotResponse>("/customer/snapshots", request);
  },

  /**
   * Delete a snapshot
   * DELETE /customer/snapshots/:id
   */
  async deleteSnapshot(snapshotId: string): Promise<void> {
    return apiClient.delete<void>(`/customer/snapshots/${snapshotId}`);
  },

  /**
   * Restore a snapshot
   * POST /customer/snapshots/:id/restore
   */
  async restoreSnapshot(snapshotId: string): Promise<{ message: string }> {
    return apiClient.post<{ message: string }>(`/customer/snapshots/${snapshotId}/restore`, {});
  },
};

// Export types for use in other modules
export type { AuthTokens as AuthTokensType };
