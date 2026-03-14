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
  } catch (err) {
    console.error("Failed to parse error response JSON:", err);
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

export interface IPAddress {
  id: string;
  address: string;
  ip_version: 4 | 6;
  is_primary: boolean;
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
  template_name?: string;
  template_id?: string;
  plan_name?: string;
  ip_addresses?: IPAddress[];
}

export interface ConsoleTokenResponse {
  token: string;
  url: string;
  expires_at: string;
}

// VM API
export const vmApi = {
  /**
   * Get console token for a running VM
   * POST /customer/vms/:id/console-token
   */
  async getConsoleToken(vmId: string): Promise<ConsoleTokenResponse> {
    return apiClient.post<ConsoleTokenResponse>(`/customer/vms/${vmId}/console-token`, {});
  },

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

  async getMetrics(vmId: string): Promise<VMMetrics> {
    return apiClient.get<VMMetrics>(`/customer/vms/${vmId}/metrics`);
  },

  async getBandwidth(vmId: string): Promise<VMBandwidth> {
    return apiClient.get<VMBandwidth>(`/customer/vms/${vmId}/bandwidth`);
  },
};

export interface VMMetrics {
  vm_id: string;
  cpu_usage_percent: number;
  memory_usage_bytes: number;
  memory_total_bytes: number;
  disk_read_bytes: number;
  disk_write_bytes: number;
  network_rx_bytes: number;
  network_tx_bytes: number;
  uptime_seconds: number;
  timestamp: string;
}

export interface VMBandwidth {
  vm_id: string;
  inbound_bytes: number;
  outbound_bytes: number;
  limit_gb: number;
  period: string;
}

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

// Profile Types
export interface CustomerProfile {
  id: string;
  email: string;
  name: string;
  phone?: string;
  created_at: string;
  updated_at: string;
}

export interface UpdateProfileRequest {
  name?: string;
  email?: string;
  phone?: string;
}

export interface UpdatePasswordRequest {
  current_password: string;
  new_password: string;
}

// 2FA Types
export interface Initiate2FAResponse {
  qr_code_url: string;
  secret: string;
}

export interface Enable2FARequest {
  totp_code: string;
}

export interface Enable2FAResponse {
  backup_codes: string[];
}

export interface BackupCodesResponse {
  backup_codes: string[];
}

// Settings Types
export interface ApiKey {
  id: string;
  name: string;
  key?: string;
  permissions: string[];
  is_active: boolean;
  expires_at?: string;
  created_at: string;
  last_used_at?: string;
}

export interface Webhook {
  id: string;
  url: string;
  events: string[];
  is_active: boolean;
  fail_count: number;
  last_success_at?: string;
  last_failure_at?: string;
  created_at: string;
  updated_at: string;
}

// Request/Response types for Settings API
export interface CreateApiKeyRequest {
  name: string;
  permissions: string[];
  expires_at?: string;
}

export interface CreateWebhookRequest {
  url: string;
  events: string[];
  secret: string;
}

export interface UpdateWebhookRequest {
  url?: string;
  events?: string[];
  is_active?: boolean;
}

export interface TestWebhookResponse {
  success: boolean;
  status_code?: number;
  response_body?: string;
  error?: string;
}

// Settings API
export const settingsApi = {
  /**
   * Get customer profile
   * GET /customer/profile
   */
  async getProfile(): Promise<CustomerProfile> {
    return apiClient.get<CustomerProfile>("/customer/profile");
  },

  /**
   * Update customer profile
   * PUT /customer/profile
   */
  async updateProfile(request: UpdateProfileRequest): Promise<CustomerProfile> {
    return apiClient.put<CustomerProfile>("/customer/profile", request);
  },

  /**
   * Update customer password
   * PUT /customer/password
   */
  async updatePassword(request: UpdatePasswordRequest): Promise<{ message: string }> {
    return apiClient.put<{ message: string }>("/customer/password", request);
  },

  /**
   * Initiate 2FA setup
   * POST /2fa/initiate
   */
  async initiate2FA(): Promise<Initiate2FAResponse> {
    return apiClient.post<Initiate2FAResponse>("/2fa/initiate", {});
  },

  /**
   * Enable 2FA with TOTP code
   * POST /2fa/enable
   */
  async enable2FA(request: Enable2FARequest): Promise<Enable2FAResponse> {
    return apiClient.post<Enable2FAResponse>("/2fa/enable", request);
  },

  /**
   * Disable 2FA
   * POST /2fa/disable
   */
  async disable2FA(): Promise<{ message: string }> {
    return apiClient.post<{ message: string }>("/2fa/disable", {});
  },

  /**
   * Get backup codes
   * GET /2fa/backup-codes
   */
  async getBackupCodes(): Promise<BackupCodesResponse> {
    return apiClient.get<BackupCodesResponse>("/2fa/backup-codes");
  },

  /**
   * Regenerate backup codes
   * POST /2fa/backup-codes/regenerate
   */
  async regenerateBackupCodes(): Promise<BackupCodesResponse> {
    return apiClient.post<BackupCodesResponse>("/2fa/backup-codes/regenerate", {});
  },

  /**
   * Get customer API keys
   * GET /customer/api-keys
   */
  async getApiKeys(): Promise<ApiKey[]> {
    return apiClient.get<ApiKey[]>("/customer/api-keys");
  },

  /**
   * Create a new API key
   * POST /customer/api-keys
   */
  async createApiKey(request: CreateApiKeyRequest): Promise<ApiKey> {
    return apiClient.post<ApiKey>("/customer/api-keys", request);
  },

  /**
   * Rotate an API key (returns new key)
   * POST /customer/api-keys/:id/rotate
   */
  async rotateApiKey(keyId: string): Promise<ApiKey> {
    return apiClient.post<ApiKey>(`/customer/api-keys/${keyId}/rotate`, {});
  },

  /**
   * Delete an API key
   * DELETE /customer/api-keys/:id
   */
  async deleteApiKey(keyId: string): Promise<void> {
    return apiClient.delete<void>(`/customer/api-keys/${keyId}`);
  },

  /**
   * Get customer Webhooks
   * GET /customer/webhooks
   */
  async getWebhooks(): Promise<Webhook[]> {
    return apiClient.get<Webhook[]>("/customer/webhooks");
  },

  /**
   * Create a new webhook
   * POST /customer/webhooks
   */
  async createWebhook(request: CreateWebhookRequest): Promise<Webhook> {
    return apiClient.post<Webhook>("/customer/webhooks", request);
  },

  /**
   * Update a webhook
   * PUT /customer/webhooks/:id
   */
  async updateWebhook(webhookId: string, request: UpdateWebhookRequest): Promise<Webhook> {
    return apiClient.put<Webhook>(`/customer/webhooks/${webhookId}`, request);
  },

  /**
   * Delete a webhook
   * DELETE /customer/webhooks/:id
   */
  async deleteWebhook(webhookId: string): Promise<void> {
    return apiClient.delete<void>(`/customer/webhooks/${webhookId}`);
  },

  /**
   * Test a webhook
   * POST /customer/webhooks/:id/test
   */
  async testWebhook(webhookId: string): Promise<TestWebhookResponse> {
    return apiClient.post<TestWebhookResponse>(`/customer/webhooks/${webhookId}/test`, {});
  }
};
