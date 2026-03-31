const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || "/api/v1";
import {
  ApiClientError,
  apiRequest as sharedAPIRequest,
  fetchCsrfToken as sharedFetchCsrfToken,
  getCsrfToken,
} from "@virtuestack/api-client";

import type {
  AuthTokens,
  LoginRequest,
  Verify2FARequest,
} from "@virtuestack/api-client";

export { ApiClientError };
export type { AuthTokens, LoginRequest, Verify2FARequest };

async function fetchCsrfToken(): Promise<void> {
  await sharedFetchCsrfToken(API_BASE_URL, "/customer/auth/csrf");
}

export async function apiRequest<T>(
  endpoint: string,
  options: RequestInit = {},
): Promise<T> {
  return sharedAPIRequest<T>(API_BASE_URL, endpoint, options);
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

  async forgotPassword(email: string): Promise<{ message: string }> {
    return apiClient.post<{ message: string }>("/customer/auth/forgot-password", { email });
  },

  async resetPassword(token: string, newPassword: string): Promise<{ message: string }> {
    return apiClient.post<{ message: string }>("/customer/auth/reset-password", {
      token,
      new_password: newPassword,
    });
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
  attached_iso?: string;
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

export interface CursorPaginatedResponse<T> {
  data: T[];
  meta: {
    per_page: number;
    has_more?: boolean;
    next_cursor?: string;
    prev_cursor?: string;
  };
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

  async getVMs(params: { perPage?: number; cursor?: string } = {}): Promise<CursorPaginatedResponse<VM>> {
    const queryParams = new URLSearchParams();
    if (params.perPage) queryParams.set("per_page", String(params.perPage));
    if (params.cursor) queryParams.set("cursor", params.cursor);
    const query = queryParams.toString() ? `?${queryParams.toString()}` : "";
    return apiClient.get<CursorPaginatedResponse<VM>>(`/customer/vms${query}`);
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

  async getNetworkHistory(vmId: string): Promise<VMBandwidth[]> {
    return apiClient.get<VMBandwidth[]>(`/customer/vms/${vmId}/network`);
  },
};

export interface Backup {
  id: string;
  vm_id: string;
  method: "full" | "snapshot";
  name: string;
  source: "manual" | "customer_schedule" | "admin_schedule";
  admin_schedule_id?: string;
  storage_backend?: "ceph" | "qcow";
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
  task_id?: string;
}

export interface TaskStatusResponse {
  id: string;
  type: string;
  status: "pending" | "running" | "completed" | "failed" | "cancelled";
  progress: number;
  progress_message?: string;
  error_message?: string;
  created_at: string;
  started_at?: string;
  completed_at?: string;
}

export const taskApi = {
  async getTaskStatus(taskId: string): Promise<TaskStatusResponse> {
    return apiClient.get<TaskStatusResponse>(`/customer/tasks/${taskId}`);
  },

  async pollTaskCompletion(
    taskId: string,
    onProgress?: (progress: number, message?: string) => void,
    intervalMs: number = 2000,
    timeoutMs: number = 600000,
  ): Promise<TaskStatusResponse> {
    const startTime = Date.now();

    while (Date.now() - startTime < timeoutMs) {
      try {
        const task = await this.getTaskStatus(taskId);

        if (onProgress) {
          onProgress(task.progress, task.progress_message || undefined);
        }

        if (task.status === "completed") {
          return task;
        }

        if (task.status === "failed") {
          throw new ApiClientError(
            task.error_message || "Task failed",
            "TASK_FAILED",
            500,
          );
        }

        if (task.status === "cancelled") {
          throw new ApiClientError(
            "Task was cancelled",
            "TASK_CANCELLED",
            500,
          );
        }

        // Wait before polling again
        await new Promise((resolve) => setTimeout(resolve, intervalMs));
      } catch (error) {
        // Re-throw ApiClientError instances
        if (error instanceof ApiClientError) {
          throw error;
        }
        // For network errors, wait and retry
        await new Promise((resolve) => setTimeout(resolve, intervalMs));
      }
    }

    throw new ApiClientError(
      "Task polling timed out",
      "TASK_TIMEOUT",
      0,
    );
  },
};

export const backupApi = {
  async listBackups(vmId?: string, method?: "full" | "snapshot", params?: { per_page?: number; cursor?: string }): Promise<Backup[]> {
    const searchParams = new URLSearchParams();
    if (vmId) searchParams.set("vm_id", vmId);
    if (method) searchParams.set("method", method);
    if (params?.cursor) searchParams.set("cursor", params.cursor);
    if (params?.per_page !== undefined) searchParams.set("per_page", String(params.per_page));
    const query = searchParams.toString();
    return apiClient.get<Backup[]>(`/customer/backups${query ? `?${query}` : ""}`);
  },

  async getBackup(backupId: string): Promise<Backup> {
    return apiClient.get<Backup>(`/customer/backups/${backupId}`);
  },

  async createBackup(request: CreateBackupRequest): Promise<CreateBackupResponse> {
    return apiClient.post<CreateBackupResponse>("/customer/backups", request);
  },

  async deleteBackup(backupId: string): Promise<void> {
    return apiClient.delete<void>(`/customer/backups/${backupId}`);
  },

  async restoreBackup(
    backupId: string,
    onProgress?: (progress: number, message?: string) => void,
  ): Promise<{ message: string; task_id?: string }> {
    const response = await apiClient.post<{ message: string; task_id?: string }>(
      `/customer/backups/${backupId}/restore`,
      {},
    );

    // If a task_id is returned, poll for completion
    if (response.task_id && onProgress) {
      try {
        await taskApi.pollTaskCompletion(response.task_id, onProgress);
      } catch (error) {
        // Log the polling error but don't fail the restore request
        // The restore has been initiated successfully
        console.warn("Failed to poll restore task:", error);
      }
    }

    return response;
  },
};

export const snapshotApi = {
  async listSnapshots(vmId?: string, params?: { per_page?: number; cursor?: string }): Promise<Snapshot[]> {
    const searchParams = new URLSearchParams();
    if (vmId) searchParams.set("vm_id", vmId);
    if (params?.cursor) searchParams.set("cursor", params.cursor);
    if (params?.per_page !== undefined) searchParams.set("per_page", String(params.per_page));
    const query = searchParams.toString();
    return apiClient.get<Snapshot[]>(`/customer/snapshots${query ? `?${query}` : ""}`);
  },

  async createSnapshot(request: CreateSnapshotRequest): Promise<CreateSnapshotResponse> {
    return apiClient.post<CreateSnapshotResponse>("/customer/snapshots", request);
  },

  async deleteSnapshot(snapshotId: string): Promise<{ task_id?: string }> {
    // Returns 202 Accepted with optional task_id for async deletion tracking.
    return apiClient.delete<{ task_id?: string }>(`/customer/snapshots/${snapshotId}`);
  },

  async restoreSnapshot(snapshotId: string): Promise<{ message: string; task_id?: string }> {
    // Returns 202 Accepted with optional task_id for async restore tracking.
    return apiClient.post<{ message: string; task_id?: string }>(`/customer/snapshots/${snapshotId}/restore`, {});
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

/**
 * The backend's UpdateProfile response does not include created_at.
 * Use this type for the return value of updateProfile() instead of CustomerProfile.
 */
export interface UpdateProfileResponse {
  id: string;
  email: string;
  name: string;
  phone?: string;
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

export interface Disable2FARequest {
  password: string;
}

export interface BackupCodesResponse {
  backup_codes: string[];
}

export interface ApiKey {
  id: string;
  name: string;
  key?: string;
  permissions: string[];
  allowed_ips?: string[];
  vm_ids?: string[];
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
  allowed_ips?: string[];
  vm_ids?: string[];
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

export interface WebhookDelivery {
  id: string;
  event: string;
  attempt_count: number;
  response_status?: number;
  success: boolean;
  next_retry_at?: string;
  delivered_at?: string;
  created_at: string;
}

export interface Template {
  id: string;
  name: string;
  os_family: string;
  os_version?: string;
  min_disk_gb: number;
  supports_cloudinit: boolean;
  description?: string;
  storage_backend: string;
}

export const settingsApi = {
  async getProfile(): Promise<CustomerProfile> {
    return apiClient.get<CustomerProfile>("/customer/profile");
  },

  async updateProfile(request: UpdateProfileRequest): Promise<UpdateProfileResponse> {
    return apiClient.put<UpdateProfileResponse>("/customer/profile", request);
  },

  async updatePassword(request: UpdatePasswordRequest): Promise<{ message: string }> {
    if (request.new_password.length < 12) {
      throw new ApiClientError(
        "New password must be at least 12 characters",
        "VALIDATION_ERROR",
        400
      );
    }
    return apiClient.put<{ message: string }>("/customer/password", request);
  },

  async initiate2FA(): Promise<Initiate2FAResponse> {
    return apiClient.post<Initiate2FAResponse>("/customer/2fa/initiate", {});
  },

  async enable2FA(request: Enable2FARequest): Promise<Enable2FAResponse> {
    return apiClient.post<Enable2FAResponse>("/customer/2fa/enable", request);
  },

  async disable2FA(request: Disable2FARequest): Promise<{ message: string }> {
    return apiClient.post<{ message: string }>("/customer/2fa/disable", request);
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

  async getWebhook(webhookId: string): Promise<Webhook> {
    return apiClient.get<Webhook>(`/customer/webhooks/${webhookId}`);
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
  },

  async listWebhookDeliveries(webhookId: string, perPage = 20, cursor?: string): Promise<WebhookDelivery[]> {
    const params = new URLSearchParams({ per_page: String(perPage) });
    if (cursor) params.set("cursor", cursor);
    return apiClient.get<WebhookDelivery[]>(`/customer/webhooks/${webhookId}/deliveries?${params.toString()}`);
  },
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

export interface IPAddressRecord {
  id: string;
  ip_set_id: string;
  address: string;
  ip_version: number;
  vm_id?: string;
  customer_id?: string;
  is_primary: boolean;
  rdns_hostname?: string;
  status: string;
  assigned_at?: string;
  released_at?: string;
  cooldown_until?: string;
  created_at: string;
}

export interface RDNSResponse {
  ip_address: string;
  rdns_hostname?: string;
}

export interface UpdateRDNSRequest {
  hostname: string;
}

export const rdnsApi = {
  async listIPs(vmId: string): Promise<IPAddressRecord[]> {
    return apiClient.get<IPAddressRecord[]>(`/customer/vms/${vmId}/ips`);
  },

  async getRDNS(vmId: string, ipId: string): Promise<RDNSResponse> {
    return apiClient.get<RDNSResponse>(`/customer/vms/${vmId}/ips/${ipId}/rdns`);
  },

  async updateRDNS(vmId: string, ipId: string, hostname: string): Promise<RDNSResponse> {
    return apiClient.put<RDNSResponse>(`/customer/vms/${vmId}/ips/${ipId}/rdns`, { hostname });
  },

  async deleteRDNS(vmId: string, ipId: string): Promise<void> {
    return apiClient.delete<void>(`/customer/vms/${vmId}/ips/${ipId}/rdns`);
  },
};

export interface NotificationPreferences {
  id: string;
  email_enabled: boolean;
  telegram_enabled: boolean;
  events: string[];
  created_at: string;
  updated_at: string;
}

export interface UpdateNotificationPreferencesRequest {
  email_enabled?: boolean;
  telegram_enabled?: boolean;
  events?: string[];
}

export interface NotificationEventType {
  events: string[];
}

export interface NotificationEvent {
  id: string;
  type: string;
  payload?: Record<string, unknown>;
  created_at: string;
}

export const notificationApi = {
  async getPreferences(): Promise<NotificationPreferences> {
    return apiClient.get<NotificationPreferences>("/customer/notifications/preferences");
  },

  async updatePreferences(prefs: UpdateNotificationPreferencesRequest): Promise<NotificationPreferences> {
    return apiClient.put<NotificationPreferences>("/customer/notifications/preferences", prefs);
  },

  async getEventTypes(): Promise<NotificationEventType> {
    return apiClient.get<NotificationEventType>("/customer/notifications/events/types");
  },

  async listEvents(): Promise<NotificationEvent[]> {
    return apiClient.get<NotificationEvent[]>("/customer/notifications/events");
  },
};

export const templateApi = {
  async listTemplates(osFamily?: string): Promise<Template[]> {
    const params = osFamily ? `?os_family=${encodeURIComponent(osFamily)}` : "";
    return apiClient.get<Template[]>(`/customer/templates${params}`);
  },
};

// --- In-App Notifications ---

export interface InAppNotification {
  id: string;
  customer_id?: string;
  admin_id?: string;
  type: string;
  title: string;
  message: string;
  data: Record<string, unknown>;
  read: boolean;
  created_at: string;
}

export interface InAppNotificationListResponse {
  data: InAppNotification[];
  meta: {
    per_page: number;
    has_more: boolean;
    cursor: string;
  };
}

export interface UnreadCountResponse {
  count: number;
}

export const inAppNotificationApi = {
  async list(params?: { unread?: boolean; cursor?: string; per_page?: number }): Promise<InAppNotificationListResponse> {
    const query = new URLSearchParams();
    if (params?.unread) query.set("unread", "true");
    if (params?.cursor) query.set("cursor", params.cursor);
    if (params?.per_page) query.set("per_page", String(params.per_page));
    const qs = query.toString();
    return apiClient.get<InAppNotificationListResponse>(`/customer/notifications${qs ? `?${qs}` : ""}`);
  },

  async markAsRead(id: string): Promise<void> {
    await apiRequest("/customer/notifications/" + id + "/read", { method: "POST", body: "{}" });
  },

  async markAllAsRead(): Promise<void> {
    await apiRequest("/customer/notifications/read-all", { method: "POST", body: "{}" });
  },

  async getUnreadCount(): Promise<UnreadCountResponse> {
    const resp = await apiClient.get<{ data: UnreadCountResponse }>("/customer/notifications/unread-count");
    return (resp as unknown as { data: UnreadCountResponse }).data;
  },
};

// Billing types
export interface BillingBalance {
  balance: number;
  currency: string;
}

export interface BillingTransaction {
  id: string;
  customer_id: string;
  type: "credit" | "debit" | "adjustment" | "refund";
  amount: number;
  balance_after: number;
  description: string;
  reference_type?: string;
  reference_id?: string;
  created_at: string;
}

export interface BillingPayment {
  id: string;
  customer_id: string;
  gateway: string;
  gateway_payment_id?: string;
  amount: number;
  currency: string;
  status: "pending" | "completed" | "failed" | "refunded";
  created_at: string;
  updated_at: string;
}

export interface TopUpConfig {
  min_amount_cents: number;
  max_amount_cents: number;
  presets: number[];
  gateways: string[];
  currency: string;
}

export interface TopUpRequest {
  gateway: string;
  amount: number;
  currency: string;
  return_url: string;
  cancel_url: string;
}

export interface TopUpResponse {
  payment_id: string;
  payment_url: string;
}

export const billingApi = {
  async getBalance(): Promise<BillingBalance> {
    const resp = await apiClient.get<{ data: BillingBalance }>(
      "/customer/billing/balance"
    );
    return (resp as unknown as { data: BillingBalance }).data;
  },

  async getTransactions(
    params: { perPage?: number; cursor?: string } = {}
  ): Promise<CursorPaginatedResponse<BillingTransaction>> {
    const queryParams = new URLSearchParams();
    if (params.perPage) queryParams.set("per_page", String(params.perPage));
    if (params.cursor) queryParams.set("cursor", params.cursor);
    const query = queryParams.toString() ? `?${queryParams.toString()}` : "";
    return apiClient.get<CursorPaginatedResponse<BillingTransaction>>(
      `/customer/billing/transactions${query}`
    );
  },

  async getPayments(
    params: { perPage?: number; cursor?: string } = {}
  ): Promise<CursorPaginatedResponse<BillingPayment>> {
    const queryParams = new URLSearchParams();
    if (params.perPage) queryParams.set("per_page", String(params.perPage));
    if (params.cursor) queryParams.set("cursor", params.cursor);
    const query = queryParams.toString() ? `?${queryParams.toString()}` : "";
    return apiClient.get<CursorPaginatedResponse<BillingPayment>>(
      `/customer/billing/payments${query}`
    );
  },

  async getTopUpConfig(): Promise<TopUpConfig> {
    const resp = await apiClient.get<{ data: TopUpConfig }>(
      "/customer/billing/top-up/config"
    );
    return (resp as unknown as { data: TopUpConfig }).data;
  },

  async initiateTopUp(req: TopUpRequest): Promise<TopUpResponse> {
    await fetchCsrfToken();
    const resp = await apiClient.post<{ data: TopUpResponse }>(
      "/customer/billing/top-up",
      req
    );
    return (resp as unknown as { data: TopUpResponse }).data;
  },
};

// Invoice types
export interface InvoiceLineItem {
  description: string;
  quantity: number;
  unit_price: number;
  amount: number;
  vm_name?: string;
  vm_id?: string;
  plan_name?: string;
  hours?: number;
}

export interface Invoice {
  id: string;
  customer_id: string;
  invoice_number: string;
  period_start: string;
  period_end: string;
  subtotal: number;
  tax_amount: number;
  total: number;
  currency: string;
  status: "draft" | "issued" | "paid" | "void";
  line_items: InvoiceLineItem[];
  issued_at?: string;
  paid_at?: string;
  has_pdf: boolean;
  created_at: string;
  updated_at: string;
}

export const invoiceApi = {
  async list(
    params: { cursor?: string; perPage?: number } = {}
  ): Promise<CursorPaginatedResponse<Invoice>> {
    const queryParams = new URLSearchParams();
    if (params.cursor) queryParams.set("cursor", params.cursor);
    if (params.perPage) queryParams.set("per_page", String(params.perPage));
    const query = queryParams.toString() ? `?${queryParams.toString()}` : "";
    return apiClient.get<CursorPaginatedResponse<Invoice>>(
      `/customer/invoices${query}`
    );
  },

  async get(id: string): Promise<Invoice> {
    const resp = await apiClient.get<{ data: Invoice }>(
      `/customer/invoices/${id}`
    );
    return (resp as unknown as { data: Invoice }).data;
  },

  getPDFUrl(id: string): string {
    return `${API_BASE_URL}/customer/invoices/${id}/pdf`;
  },
};
