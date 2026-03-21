const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || "/api/v1";

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
    await fetch(`${API_BASE_URL}/customer/profile`, { method: "GET", credentials: "include" });
  } catch (err) {
    // Log for debugging - non-fatal, the request may fail if server is unreachable
    console.warn('fetchCsrfToken: Failed (non-fatal):', err);
  }
}

function buildHeaders(includeCsrf = false): HeadersInit {
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
): Promise<T> {
  const url = `${API_BASE_URL}${endpoint}`;
  const isStateChanging = ["POST", "PUT", "PATCH", "DELETE"].includes(
    (options.method || "GET").toUpperCase()
  );

  const config: RequestInit = {
    ...options,
    credentials: "include",
    headers: {
      ...buildHeaders(isStateChanging),
      ...options.headers,
    },
  };

  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), 10_000);
  let response: Response;
  try {
    try {
      response = await fetch(url, {
        ...config,
        signal: controller.signal,
      });
    } catch (networkErr) {
      const isAbort = networkErr instanceof DOMException && networkErr.name === "AbortError";
      throw new ApiClientError(
        isAbort ? "Request timed out" : "Network error: unable to reach the server",
        isAbort ? "REQUEST_TIMEOUT" : "NETWORK_ERROR",
        0,
      );
    }
  } finally {
    clearTimeout(timeoutId);
  }

  if (!response.ok) {
    const error = await parseError(response);
    throw error;
  }

  if (response.status === 204) {
    return undefined as unknown as T;
  }

  const data = await response.json();
  return data.data as T;
}

export const apiClient = {
  get<T>(endpoint: string): Promise<T> {
    return apiRequest<T>(endpoint, { method: "GET" });
  },

  post<T>(endpoint: string, body: unknown): Promise<T> {
    return apiRequest<T>(
      endpoint,
      { method: "POST", body: JSON.stringify(body) },
    );
  },

  put<T>(endpoint: string, body: unknown): Promise<T> {
    return apiRequest<T>(
      endpoint,
      { method: "PUT", body: JSON.stringify(body) },
    );
  },

  patch<T>(endpoint: string, body: unknown): Promise<T> {
    return apiRequest<T>(
      endpoint,
      { method: "PATCH", body: JSON.stringify(body) },
    );
  },

  delete<T>(endpoint: string): Promise<T> {
    return apiRequest<T>(endpoint, { method: "DELETE" });
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

export const customerAuthApi = {
  async login(credentials: LoginRequest): Promise<AuthTokens> {
    await fetchCsrfToken();
    return apiClient.post<AuthTokens>("/customer/auth/login", credentials);
  },

  async verify2FA(request: Verify2FARequest): Promise<AuthTokens> {
    return apiClient.post<AuthTokens>("/customer/auth/verify-2fa", request);
  },

  async refreshToken(): Promise<AuthTokens> {
    return apiClient.post<AuthTokens>("/customer/auth/refresh", {});
  },

  async logout(): Promise<void> {
    try {
      await apiClient.post("/customer/auth/logout", {});
    } catch (err) {
      // Logout errors are non-fatal — session may already be invalid.
      // Log for debugging but always clear local state regardless.
      console.warn('Logout request failed (session may already be invalid):', err);
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
      const tokens = await customerAuthApi.refreshToken();
      tokenValidUntil =
        Date.now() + Math.max((tokens.expires_in || 900) - 60, 60) * 1000;
      return true;
    } catch {
      tokenValidUntil = 0;
      return false;
    }
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
  status: "running" | "stopped" | "error" | "provisioning" | "suspended" | "migrating" | "reinstalling" | "deleted";
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

export interface VMOperationResponse {
  message: string;
}

export const vmApi = {
  async getConsoleToken(vmId: string): Promise<ConsoleTokenResponse> {
    return apiClient.post<ConsoleTokenResponse>(`/customer/vms/${vmId}/console-token`, {});
  },

  async getSerialToken(vmId: string): Promise<ConsoleTokenResponse> {
    return apiClient.post<ConsoleTokenResponse>(`/customer/vms/${vmId}/serial-token`, {});
  },

  async startVM(vmId: string): Promise<VMOperationResponse> {
    return apiClient.post<VMOperationResponse>(`/customer/vms/${vmId}/start`, {});
  },

  async stopVM(vmId: string): Promise<VMOperationResponse> {
    return apiClient.post<VMOperationResponse>(`/customer/vms/${vmId}/stop`, {});
  },

  async forceStopVM(vmId: string): Promise<VMOperationResponse> {
    return apiClient.post<VMOperationResponse>(`/customer/vms/${vmId}/force-stop`, {});
  },

  async restartVM(vmId: string): Promise<VMOperationResponse> {
    return apiClient.post<VMOperationResponse>(`/customer/vms/${vmId}/restart`, {});
  },

  async getVMs(): Promise<VM[]> {
    return apiClient.get<VM[]>("/customer/vms");
  },

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

export interface Backup {
  id: string;
  vm_id: string;
  name: string;
  type: "full";
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

export const backupApi = {
  async listBackups(vmId?: string): Promise<Backup[]> {
    const params = vmId ? `?vm_id=${vmId}` : "";
    return apiClient.get<Backup[]>(`/customer/backups${params}`);
  },

  async createBackup(request: CreateBackupRequest): Promise<CreateBackupResponse> {
    return apiClient.post<CreateBackupResponse>("/customer/backups", request);
  },

  async deleteBackup(backupId: string): Promise<void> {
    return apiClient.delete<void>(`/customer/backups/${backupId}`);
  },

  async restoreBackup(backupId: string): Promise<{ message: string }> {
    return apiClient.post<{ message: string }>(`/customer/backups/${backupId}/restore`, {});
  },
};

export const snapshotApi = {
  async listSnapshots(vmId?: string): Promise<Snapshot[]> {
    const params = vmId ? `?vm_id=${vmId}` : "";
    return apiClient.get<Snapshot[]>(`/customer/snapshots${params}`);
  },

  async createSnapshot(request: CreateSnapshotRequest): Promise<CreateSnapshotResponse> {
    return apiClient.post<CreateSnapshotResponse>("/customer/snapshots", request);
  },

  async deleteSnapshot(snapshotId: string): Promise<void> {
    return apiClient.delete<void>(`/customer/snapshots/${snapshotId}`);
  },

  async restoreSnapshot(snapshotId: string): Promise<{ message: string }> {
    return apiClient.post<{ message: string }>(`/customer/snapshots/${snapshotId}/restore`, {});
  },
};



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
  message?: string;
  status_code?: number;
  response_body?: string;
  error?: string;
}

export const settingsApi = {
  async getProfile(): Promise<CustomerProfile> {
    return apiClient.get<CustomerProfile>("/customer/profile");
  },

  async updateProfile(request: UpdateProfileRequest): Promise<CustomerProfile> {
    return apiClient.put<CustomerProfile>("/customer/profile", request);
  },

  async updatePassword(request: UpdatePasswordRequest): Promise<{ message: string }> {
    return apiClient.put<{ message: string }>("/customer/password", request);
  },

  async initiate2FA(): Promise<Initiate2FAResponse> {
    return apiClient.post<Initiate2FAResponse>("/customer/2fa/initiate", {});
  },

  async enable2FA(request: Enable2FARequest): Promise<Enable2FAResponse> {
    return apiClient.post<Enable2FAResponse>("/customer/2fa/enable", request);
  },

  async disable2FA(): Promise<{ message: string }> {
    return apiClient.post<{ message: string }>("/customer/2fa/disable", {});
  },

  async getBackupCodes(): Promise<BackupCodesResponse> {
    return apiClient.get<BackupCodesResponse>("/customer/2fa/backup-codes");
  },

  async get2FAStatus(): Promise<{ enabled: boolean }> {
    return apiClient.get<{ enabled: boolean }>("/customer/2fa/status");
  },

  async regenerateBackupCodes(): Promise<BackupCodesResponse> {
    return apiClient.post<BackupCodesResponse>("/customer/2fa/backup-codes/regenerate", {});
  },

  async getApiKeys(): Promise<ApiKey[]> {
    return apiClient.get<ApiKey[]>("/customer/api-keys");
  },

  async createApiKey(request: CreateApiKeyRequest): Promise<ApiKey> {
    return apiClient.post<ApiKey>("/customer/api-keys", request);
  },

  async rotateApiKey(keyId: string): Promise<ApiKey> {
    return apiClient.post<ApiKey>(`/customer/api-keys/${keyId}/rotate`, {});
  },

  async deleteApiKey(keyId: string): Promise<void> {
    return apiClient.delete<void>(`/customer/api-keys/${keyId}`);
  },

  async getWebhooks(): Promise<Webhook[]> {
    return apiClient.get<Webhook[]>("/customer/webhooks");
  },

  async createWebhook(request: CreateWebhookRequest): Promise<Webhook> {
    return apiClient.post<Webhook>("/customer/webhooks", request);
  },

  async updateWebhook(webhookId: string, request: UpdateWebhookRequest): Promise<Webhook> {
    return apiClient.put<Webhook>(`/customer/webhooks/${webhookId}`, request);
  },

  async deleteWebhook(webhookId: string): Promise<void> {
    return apiClient.delete<void>(`/customer/webhooks/${webhookId}`);
  },

  async testWebhook(webhookId: string): Promise<TestWebhookResponse> {
    return apiClient.post<TestWebhookResponse>(`/customer/webhooks/${webhookId}/test`, {});
  }
};

export interface ISORecord {
  id: string;
  vm_id: string;
  file_name: string;
  file_size: number;
  sha256: string;
  status: string;
  created_at: string;
}

export interface ISOUploadResponse {
  id: string;
  file_name: string;
  file_size: number;
  sha256: string;
}

export const isoApi = {
  async listISOs(vmId: string): Promise<ISORecord[]> {
    return apiClient.get<ISORecord[]>(`/customer/vms/${vmId}/iso`);
  },

  async uploadISO(
    vmId: string,
    file: File,
    onProgress?: (progress: number) => void,
    signal?: AbortSignal,
  ): Promise<ISOUploadResponse> {
    const url = `${API_BASE_URL}/customer/vms/${vmId}/iso/upload`;
    const csrfToken = getCsrfToken();

    return new Promise<ISOUploadResponse>((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      const formData = new FormData();
      formData.append("file", file);

      xhr.open("POST", url);
      xhr.withCredentials = true;
      xhr.timeout = 600000; // 10 minute timeout for large ISO uploads

      if (csrfToken) {
        xhr.setRequestHeader("X-CSRF-Token", csrfToken);
      }

      xhr.upload.onprogress = (event) => {
        if (event.lengthComputable && onProgress) {
          onProgress(Math.round((event.loaded / event.total) * 100));
        }
      };

      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          try {
            const data = JSON.parse(xhr.responseText);
            resolve(data.data as ISOUploadResponse);
          } catch {
            reject(new ApiClientError("Invalid response", "PARSE_ERROR", xhr.status));
          }
        } else {
          reject(new ApiClientError(
            xhr.statusText || "Upload failed",
            "UPLOAD_ERROR",
            xhr.status,
          ));
        }
      };

      xhr.onerror = () => {
        reject(new ApiClientError("Network error during upload", "NETWORK_ERROR", 0));
      };

      xhr.ontimeout = () => {
        reject(new ApiClientError("Upload timed out after 10 minutes", "TIMEOUT_ERROR", 0));
      };

      if (signal) {
        signal.addEventListener("abort", () => {
          xhr.abort();
          reject(new DOMException("Upload cancelled", "AbortError"));
        });
      }

      xhr.send(formData);
    });
  },

  async deleteISO(vmId: string, isoId: string): Promise<void> {
    return apiClient.delete<void>(`/customer/vms/${vmId}/iso/${isoId}`);
  },

  async attachISO(vmId: string, isoId: string): Promise<{ message: string }> {
    return apiClient.post<{ message: string }>(
      `/customer/vms/${vmId}/iso/${isoId}/attach`,
      {}
    );
  },

  async detachISO(vmId: string, isoId: string): Promise<{ message: string }> {
    return apiClient.post<{ message: string }>(
      `/customer/vms/${vmId}/iso/${isoId}/detach`,
      {}
    );
  },
};