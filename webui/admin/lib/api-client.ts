const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || "/api/v1";

// AdminUser represents the identity of the currently authenticated admin.
// The fields are populated from the server via GET /admin/auth/me endpoint,
// which is used for lightweight session validation.
export interface AdminUser {
  id: string;
  email: string;
  role: string;
  permissions?: string[];
}

// Permission represents a single permission with its description.
export interface Permission {
  name: string;
  description: string;
}

// Admin represents an admin user with full details for permission management.
export interface Admin {
  id: string;
  email: string;
  name: string;
  role: string;
  permissions: string[];
  totp_enabled: boolean;
  created_at: string;
  updated_at: string;
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
  // Use the /admin/auth/me endpoint to bootstrap the CSRF cookie.
  // The server sets the CSRF cookie on any response; we use this lightweight
  // endpoint instead of the non-existent /admin/auth/csrf route.
  try {
    await fetch(`${API_BASE_URL}/admin/auth/me`, {
      method: 'GET',
      credentials: 'include',
    });
    if (!getCsrfToken()) {
      // Log for debugging CSRF issues - this indicates the server didn't set the expected cookie
      console.warn('fetchCsrfToken: CSRF cookie was not set after bootstrap request.');
    }
  } catch (err) {
    // Log for debugging - non-fatal, the request may fail if server is unreachable
    console.warn('fetchCsrfToken: Failed to bootstrap CSRF cookie (non-fatal):', err);
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

/**
 * Options for API requests that expect a response body (default behavior).
 */
interface JsonRequestOptions extends RequestInit {
  /** Set to true when expecting HTTP 204 No Content response */
  expectNoContent?: false;
}

/**
 * Options for API requests that expect no response body (HTTP 204 No Content).
 */
interface VoidRequestOptions extends RequestInit {
  /** Must be true - indicates the endpoint returns HTTP 204 No Content */
  expectNoContent: true;
}

/**
 * Fetches an API endpoint that returns HTTP 204 No Content.
 * Use this overload when the endpoint returns no response body.
 */
export async function apiRequest(
  endpoint: string,
  options: VoidRequestOptions,
): Promise<void>;

/**
 * Fetches an API endpoint and returns the response body as type T.
 * Use this overload for endpoints that return JSON data.
 */
export async function apiRequest<T>(
  endpoint: string,
  options?: JsonRequestOptions,
): Promise<T>;

/**
 * Internal implementation - do not call directly, use the typed overloads above.
 */
export async function apiRequest<T>(
  endpoint: string,
  options: (JsonRequestOptions | VoidRequestOptions) = {},
): Promise<T | void> {
  const url = `${API_BASE_URL}${endpoint}`;
  const isStateChanging = ["POST", "PUT", "PATCH", "DELETE"].includes(
    (options.method || "GET").toUpperCase()
  );

  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), 10_000);

  const config: RequestInit = {
    ...options,
    signal: controller.signal,
    credentials: "include",
    headers: {
      ...buildHeaders(isStateChanging),
      ...options.headers,
    },
  };

  try {
    let response: Response;
    try {
      response = await fetch(url, config);
    } catch (networkErr) {
      const isAbort = networkErr instanceof DOMException && networkErr.name === "AbortError";
      throw new ApiClientError(
        isAbort ? "Request timed out" : "Network error: unable to reach the server",
        isAbort ? "REQUEST_TIMEOUT" : "NETWORK_ERROR",
        0,
      );
    }

    if (!response.ok) {
      const error = await parseError(response);
      throw error;
    }

    if (response.status === 204) {
      // HTTP 204 No Content - return void. This is type-safe when called with
      // expectNoContent: true, as the overload resolves to Promise<void>.
      return;
    }

    const data = await response.json();
    return data.data as T;
  } finally {
    clearTimeout(timeoutId);
  }
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

  /** POST request expecting no response body (HTTP 204 No Content) */
  postVoid(endpoint: string, body: unknown): Promise<void> {
    return apiRequest(endpoint, {
      method: "POST",
      body: JSON.stringify(body),
      expectNoContent: true,
    });
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

  /** DELETE request expecting no response body (HTTP 204 No Content) */
  deleteVoid(endpoint: string): Promise<void> {
    return apiRequest(endpoint, { method: "DELETE", expectNoContent: true });
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

export interface ReauthResponse {
  reauth_token: string;
  expires_in: number;
}

export const adminAuthApi = {
  async login(credentials: LoginRequest): Promise<AuthTokens> {
    await fetchCsrfToken();
    return apiClient.post<AuthTokens>("/admin/auth/login", credentials);
  },

  async verify2FA(request: Verify2FARequest): Promise<AuthTokens> {
    return apiClient.post<AuthTokens>("/admin/auth/verify-2fa", request);
  },

  async refreshToken(): Promise<AuthTokens> {
    return apiClient.post<AuthTokens>("/admin/auth/refresh", {});
  },

  async logout(): Promise<void> {
    try {
      await apiClient.postVoid("/admin/auth/logout", {});
    } catch (err) {
      // Logout errors are non-fatal — session may already be invalid.
      // Log for debugging but don't propagate to prevent UI from hanging.
      console.warn('Logout request failed (session may already be invalid):', err);
    }
    tokenValidUntil = 0;
  },

  // me() fetches the current authenticated admin user's identity from the server.
  // This is a lightweight endpoint (GET /admin/auth/me) that returns only the
  // essential user fields (id, email, role). Use this for session validation
  // instead of heavy endpoints like getNodes().
  //
  // Returns null when the session is invalid (401/403), so callers can clear
  // local state and redirect to login.
  async me(): Promise<AdminUser | null> {
    try {
      const user = await apiClient.get<AdminUser>("/admin/auth/me");
      return user;
    } catch (err) {
      if (err instanceof ApiClientError && (err.status === 401 || err.status === 403)) {
        return null;
      }
      // Re-throw unexpected errors so the caller can handle them.
      throw err;
    }
  },

  // reauth() re-authenticates the admin by verifying their password.
  // Returns a short-lived re-auth token to include in the X-Reauth-Token
  // header when performing destructive operations (DELETE nodes, VMs, etc.).
  async reauth(password: string): Promise<ReauthResponse> {
    return apiClient.post<ReauthResponse>("/admin/auth/reauth", { password });
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


export interface Node {
  id: string;
  name: string;
  hostname: string;
  status: "online" | "offline" | "draining" | "degraded" | "failed";
  location: string;
  vm_count: number;
  cpu_total: number;
  cpu_allocated: number;
  memory_total_gb: number;
  memory_allocated_gb: number;
}

export interface NodeDetail {
  id: string;
  hostname: string;
  grpc_address: string;
  management_ip: string;
  location_id?: string;
  status: string;
  total_vcpu: number;
  total_memory_mb: number;
  allocated_vcpu: number;
  allocated_memory_mb: number;
  storage_backend: string;
  storage_path?: string;
  ceph_pool: string;
  last_heartbeat_at?: string;
  created_at: string;
}

export interface CreateNodeRequest {
  hostname: string;
  grpc_address: string;
  management_ip: string;
  location_id?: string;
  total_vcpu: number;
  total_memory_mb: number;
  ipmi_address?: string;
  ipmi_username?: string;
  ipmi_password?: string;
}

export interface UpdateNodeRequest {
  grpc_address?: string;
  location_id?: string;
  total_vcpu?: number;
  total_memory_mb?: number;
  ipmi_address?: string;
}

export const adminNodesApi = {
  async getNodes(): Promise<PaginatedResponse<Node>> {
    return apiClient.get<PaginatedResponse<Node>>("/admin/nodes");
  },

  async getNode(id: string): Promise<NodeDetail> {
    return apiClient.get<NodeDetail>(`/admin/nodes/${id}`);
  },

  async createNode(data: CreateNodeRequest): Promise<NodeDetail> {
    return apiClient.post<NodeDetail>("/admin/nodes", data);
  },

  async updateNode(id: string, data: UpdateNodeRequest): Promise<NodeDetail> {
    return apiClient.put<NodeDetail>(`/admin/nodes/${id}`, data);
  },

  async deleteNode(id: string): Promise<void> {
    return apiClient.deleteVoid(`/admin/nodes/${id}`);
  },

  async drainNode(id: string): Promise<{ status: string }> {
    return apiClient.post<{ status: string }>(`/admin/nodes/${id}/drain`, {});
  },

  async failoverNode(id: string): Promise<{ status: string }> {
    return apiClient.post<{ status: string }>(`/admin/nodes/${id}/failover`, {});
  },

  async undrainNode(id: string): Promise<{ status: string }> {
    return apiClient.post<{ status: string }>(`/admin/nodes/${id}/undrain`, {});
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

export interface CreateCustomerRequest {
  name: string;
  email: string;
  password: string;
  phone?: string;
}

export interface UpdateCustomerRequest {
  name?: string;
  status?: "active" | "suspended";
}

export const adminCustomersApi = {
  async getCustomers(): Promise<PaginatedResponse<Customer>> {
    return apiClient.get<PaginatedResponse<Customer>>("/admin/customers");
  },

  async getCustomer(id: string): Promise<Customer> {
    return apiClient.get<Customer>(`/admin/customers/${id}`);
  },

  async createCustomer(data: CreateCustomerRequest): Promise<Customer> {
    return apiClient.post<Customer>("/admin/customers", data);
  },

  async updateCustomer(id: string, data: UpdateCustomerRequest): Promise<Customer> {
    return apiClient.put<Customer>(`/admin/customers/${id}`, data);
  },

  async suspendCustomer(id: string): Promise<Customer> {
    return apiClient.put<Customer>(`/admin/customers/${id}`, { status: 'suspended' });
  },

  async unsuspendCustomer(id: string): Promise<Customer> {
    return apiClient.put<Customer>(`/admin/customers/${id}`, { status: 'active' });
  },

  async deleteCustomer(id: string): Promise<void> {
    return apiClient.deleteVoid(`/admin/customers/${id}`);
  },

  async getCustomerAuditLogs(id: string, page = 1, perPage = 20): Promise<PaginatedResponse<AuditLog>> {
    const searchParams = new URLSearchParams({
      page: String(page),
      per_page: String(perPage),
    });
    return apiClient.get<PaginatedResponse<AuditLog>>(`/admin/customers/${id}/audit-logs?${searchParams.toString()}`);
  },
};

export interface Plan {
  id: string;
  name: string;
  slug: string;
  vcpu: number;
  memory_mb: number;
  disk_gb: number;
  bandwidth_limit_gb: number;
  port_speed_mbps: number;
  price_monthly: number;
  price_hourly: number;
  storage_backend: string;
  is_active: boolean;
  sort_order: number;
  status: "active" | "inactive";
  snapshot_limit: number;
  backup_limit: number;
  iso_upload_limit: number;
  created_at: string;
  updated_at: string;
}

export interface CreatePlanRequest {
  name: string;
  slug: string;
  vcpu: number;
  memory_mb: number;
  disk_gb: number;
  bandwidth_limit_gb?: number;
  port_speed_mbps: number;
  price_monthly?: number;
  price_hourly?: number;
  storage_backend?: "ceph" | "qcow" | "lvm";
  is_active?: boolean;
  sort_order?: number;
  snapshot_limit?: number;
  backup_limit?: number;
  iso_upload_limit?: number;
}

export interface UpdatePlanRequest {
  name?: string;
  slug?: string;
  vcpu?: number;
  memory_mb?: number;
  disk_gb?: number;
  bandwidth_limit_gb?: number;
  port_speed_mbps?: number;
  price_monthly?: number;
  price_hourly?: number;
  storage_backend?: "ceph" | "qcow" | "lvm";
  is_active?: boolean;
  sort_order?: number;
  snapshot_limit?: number;
  backup_limit?: number;
  iso_upload_limit?: number;
}

export const adminPlansApi = {
  async getPlans(): Promise<Plan[]> {
    return apiClient.get<Plan[]>("/admin/plans");
  },

  async getPlan(id: string): Promise<Plan | undefined> {
    const plans = await this.getPlans();
    return plans.find((p) => p.id === id);
  },

  async getPlanUsage(id: string): Promise<{ plan_id: string; vm_count: number }> {
    return apiClient.get<{ plan_id: string; vm_count: number }>(`/admin/plans/${id}/usage`);
  },

  async deletePlan(id: string): Promise<void> {
    return apiClient.deleteVoid(`/admin/plans/${id}`);
  },

  async createPlan(plan: CreatePlanRequest): Promise<Plan> {
    return apiClient.post<Plan>("/admin/plans", plan);
  },

  async updatePlan(id: string, plan: UpdatePlanRequest): Promise<Plan> {
    return apiClient.put<Plan>(`/admin/plans/${id}`, plan);
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
  async getAuditLogs(page = 1, perPage = 20, search?: string): Promise<PaginatedResponse<AuditLog>> {
    const searchParams = new URLSearchParams({
      page: String(page),
      per_page: String(perPage),
    });
    if (search) searchParams.set("search", search);
    return apiClient.get<PaginatedResponse<AuditLog>>(`/admin/audit-logs?${searchParams.toString()}`);
  },
};

export interface VM {
  id: string;
  name: string;
  customer_id: string;
  node_id: string;
  plan_id?: string;
  template_id?: string;
  ip_addresses?: string[];
  status: string;
  created_at: string;
  hostname?: string;
  vcpu?: number;
  memory_mb?: number;
  disk_gb?: number;
  port_speed_mbps?: number;
  bandwidth_limit_gb?: number;
}

export interface CreateVMRequest {
  customer_id: string;
  plan_id: string;
  template_id: string;
  hostname: string;
  password: string;
  ssh_keys?: string[];
  location_id?: string;
  node_id?: string;
}

export interface UpdateVMRequest {
  hostname?: string;
  vcpu?: number;
  memory_mb?: number;
  disk_gb?: number;
  port_speed_mbps?: number;
  bandwidth_limit_gb?: number;
}

export interface CreateVMResponse {
  vm_id: string;
  task_id: string;
}

export const adminVMsApi = {
  async getVMs(): Promise<PaginatedResponse<VM>> {
    return apiClient.get<PaginatedResponse<VM>>('/admin/vms');
  },

  async getVM(id: string): Promise<VM> {
    return apiClient.get<VM>(`/admin/vms/${id}`);
  },

  async createVM(data: CreateVMRequest): Promise<CreateVMResponse> {
    return apiClient.post<CreateVMResponse>("/admin/vms", data);
  },

  async updateVM(id: string, data: UpdateVMRequest): Promise<VM> {
    return apiClient.put<VM>(`/admin/vms/${id}`, data);
  },

  async deleteVM(id: string): Promise<void> {
    return apiClient.deleteVoid(`/admin/vms/${id}`);
  },

  async migrateVM(id: string, data: { target_node_id: string }): Promise<{ vm_id: string; target_node_id: string; task_id: string; status: string }> {
    return apiClient.post<{ vm_id: string; target_node_id: string; task_id: string; status: string }>(`/admin/vms/${id}/migrate`, data);
  },

  async getVMIPs(id: string): Promise<{ id: string; address: string; rdns?: string }[]> {
    return apiClient.get<{ id: string; address: string; rdns?: string }[]>(`/admin/vms/${id}/ips`);
  },

  async getIPRDNS(vmId: string, ipId: string): Promise<{ ip_id: string; rdns: string }> {
    return apiClient.get<{ ip_id: string; rdns: string }>(`/admin/vms/${vmId}/ips/${ipId}/rdns`);
  },

  async updateIPRDNS(vmId: string, ipId: string, rdns: string): Promise<{ ip_id: string; rdns: string }> {
    return apiClient.put<{ ip_id: string; rdns: string }>(`/admin/vms/${vmId}/ips/${ipId}/rdns`, { rdns });
  },

  async deleteIPRDNS(vmId: string, ipId: string): Promise<void> {
    return apiClient.deleteVoid(`/admin/vms/${vmId}/ips/${ipId}/rdns`);
  },
};

export interface SystemSetting {
  key: string;
  value: string;
  description?: string;
}

export const adminSettingsApi = {
  async getSettings(): Promise<SystemSetting[]> {
    return apiClient.get<SystemSetting[]>("/admin/settings");
  },

  async getSetting(key: string): Promise<string | undefined> {
    const settings = await this.getSettings();
    return settings.find((s) => s.key === key)?.value;
  },

  async putSetting(key: string, value: string): Promise<SystemSetting> {
    return apiClient.put<SystemSetting>(`/admin/settings/${key}`, { value });
  },
};

export interface Template {
  id: string;
  name: string;
  os_family: string;
  os_version: string;
  rbd_image: string;
  rbd_snapshot: string;
  min_disk_gb: number;
  supports_cloudinit: boolean;
  is_active: boolean;
  sort_order: number;
  description?: string;
  storage_backend: string;
  file_path?: string;
  status: string;
  created_at: string;
  updated_at: string;
}

export interface UpdateTemplateRequest {
  name?: string;
  os_family?: string;
  os_version?: string;
  rbd_image?: string;
  rbd_snapshot?: string;
  min_disk_gb?: number;
  supports_cloudinit?: boolean;
  is_active?: boolean;
  sort_order?: number;
  description?: string;
  storage_backend?: "ceph" | "qcow";
  file_path?: string;
}

export const adminTemplatesApi = {
  async getTemplates(): Promise<Template[]> {
    return apiClient.get<Template[]>("/admin/templates");
  },

  async getTemplate(id: string): Promise<Template> {
    return apiClient.get<Template>(`/admin/templates/${id}`);
  },

  async createTemplate(data: {
    name: string;
    os_family: string;
    os_version?: string;
    rbd_image?: string;
    rbd_snapshot?: string;
    min_disk_gb?: number;
    supports_cloudinit?: boolean;
    is_active?: boolean;
    sort_order?: number;
    description?: string;
    storage_backend?: "ceph" | "qcow";
    file_path?: string;
  }): Promise<Template> {
    return apiClient.post<Template>("/admin/templates", data);
  },

  async updateTemplate(id: string, data: UpdateTemplateRequest): Promise<Template> {
    return apiClient.put<Template>(`/admin/templates/${id}`, data);
  },

  async deleteTemplate(id: string): Promise<void> {
    return apiClient.deleteVoid(`/admin/templates/${id}`);
  },

  async importTemplate(id: string, data: {
    name?: string;
    os_family?: string;
    os_version?: string;
    source_path?: string;
  } = {}): Promise<{ message: string }> {
    return apiClient.post<{ message: string }>(`/admin/templates/${id}/import`, data);
  },
};

// ============================================================================
// Admin Backup API
// ============================================================================

export interface AdminBackup {
  id: string;
  vm_id: string;
  vm_hostname?: string;
  customer_id?: string;
  customer_email?: string;
  source: "manual" | "customer_schedule" | "admin_schedule";
  admin_schedule_id?: string;
  admin_schedule_name?: string;
  storage_backend: string;
  status: "creating" | "completed" | "failed" | "restoring";
  size_bytes?: number;
  created_at: string;
  expires_at?: string;
}

export interface AdminBackupListParams {
  page?: number;
  per_page?: number;
  customer_id?: string;
  vm_id?: string;
  status?: string;
  source?: string;
  admin_schedule_id?: string;
}

export interface PaginatedResponse<T> {
  data: T[];
  meta: {
    page: number;
    per_page: number;
    total: number;
    total_pages: number;
  };
}

export const adminBackupsApi = {
  async getBackups(params: AdminBackupListParams = {}): Promise<PaginatedResponse<AdminBackup>> {
    const searchParams = new URLSearchParams();
    if (params.page !== undefined) searchParams.set("page", String(params.page));
    if (params.per_page !== undefined) searchParams.set("per_page", String(params.per_page));
    if (params.customer_id) searchParams.set("customer_id", params.customer_id);
    if (params.vm_id) searchParams.set("vm_id", params.vm_id);
    if (params.status) searchParams.set("status", params.status);
    if (params.source) searchParams.set("source", params.source);
    if (params.admin_schedule_id) searchParams.set("admin_schedule_id", params.admin_schedule_id);

    return apiClient.get<PaginatedResponse<AdminBackup>>(`/admin/backups?${searchParams.toString()}`);
  },

  async restoreBackup(id: string): Promise<{ backup_id: string; status: string }> {
    return apiClient.post<{ backup_id: string; status: string }>(`/admin/backups/${id}/restore`, {});
  },
};

// ============================================================================
// Admin Backup Schedule API
// ============================================================================

export interface AdminBackupSchedule {
  id: string;
  name: string;
  description?: string;
  frequency: "daily" | "weekly" | "monthly";
  retention_count: number;
  target_all: boolean;
  target_plan_ids?: string[];
  target_node_ids?: string[];
  target_customer_ids?: string[];
  active: boolean;
  next_run_at: string;
  last_run_at?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateAdminBackupScheduleRequest {
  name: string;
  description?: string;
  frequency: "daily" | "weekly" | "monthly";
  retention_count: number;
  target_all: boolean;
  target_plan_ids?: string[];
  target_node_ids?: string[];
  target_customer_ids?: string[];
  active?: boolean;
}

export interface UpdateAdminBackupScheduleRequest {
  name?: string;
  description?: string;
  frequency?: "daily" | "weekly" | "monthly";
  retention_count?: number;
  target_all?: boolean;
  target_plan_ids?: string[];
  target_node_ids?: string[];
  target_customer_ids?: string[];
  active?: boolean;
}

export const adminBackupSchedulesApi = {
  async getSchedules(params: { page?: number; per_page?: number; active?: boolean } = {}): Promise<PaginatedResponse<AdminBackupSchedule>> {
    const searchParams = new URLSearchParams();
    if (params.page) searchParams.set("page", String(params.page));
    if (params.per_page) searchParams.set("per_page", String(params.per_page));
    if (params.active !== undefined) searchParams.set("active", String(params.active));

    return apiClient.get<PaginatedResponse<AdminBackupSchedule>>(`/admin/admin-backup-schedules?${searchParams.toString()}`);
  },

  async getSchedule(id: string): Promise<AdminBackupSchedule> {
    return apiClient.get<AdminBackupSchedule>(`/admin/admin-backup-schedules/${id}`);
  },

  async createSchedule(data: CreateAdminBackupScheduleRequest): Promise<AdminBackupSchedule> {
    return apiClient.post<AdminBackupSchedule>("/admin/admin-backup-schedules", data);
  },

  async updateSchedule(id: string, data: UpdateAdminBackupScheduleRequest): Promise<AdminBackupSchedule> {
    return apiClient.put<AdminBackupSchedule>(`/admin/admin-backup-schedules/${id}`, data);
  },

  async deleteSchedule(id: string): Promise<void> {
    return apiClient.deleteVoid(`/admin/admin-backup-schedules/${id}`);
  },

  async runSchedule(id: string): Promise<{ message: string; next_run_at: string }> {
    return apiClient.post<{ message: string; next_run_at: string }>(`/admin/admin-backup-schedules/${id}/run`, {});
  },
};

// ============================================================================
// Admin Permissions API
// ============================================================================

export const adminPermissionsApi = {
  async getPermissions(): Promise<Permission[]> {
    const response = await apiClient.get<{ permissions: Permission[] }>("/admin/auth/permissions");
    return response.permissions;
  },

  async updateAdminPermissions(adminId: string, permissions: string[]): Promise<Admin> {
    const response = await apiClient.put<{ admin: Admin }>(`/admin/auth/permissions/${adminId}`, { permissions });
    return response.admin;
  },

  async getAdmins(): Promise<Admin[]> {
    return apiClient.get<Admin[]>("/admin/admins");
  },
};

// ============================================================================
// Admin IP Sets API
// ============================================================================

export interface IPSet {
  id: string;
  name: string;
  location_id?: string;
  network: string;
  gateway: string;
  vlan_id?: number;
  ip_version: number;
  node_ids?: string[];
  created_at: string;
  location?: string;
  total_ips?: number;
  available_ips?: number;
  assigned_ips?: number;
  reserved_ips?: number;
  cooldown_ips?: number;
}

export interface IPSetDetail {
  id: string;
  name: string;
  location_id?: string;
  network: string;
  gateway: string;
  vlan_id?: number;
  ip_version: number;
  node_ids?: string[];
  created_at: string;
  total_ips: number;
  assigned_ips: number;
  available_ips: number;
  reserved_ips: number;
  cooldown_ips: number;
}

export interface UpdateIPSetRequest {
  name?: string;
  location_id?: string;
  gateway?: string;
  vlan_id?: number;
  node_ids?: string[];
}

export const adminIPSetsApi = {
  async getIPSets(): Promise<IPSet[]> {
    return apiClient.get<IPSet[]>("/admin/ip-sets");
  },

  async getIPSet(id: string): Promise<IPSetDetail> {
    return apiClient.get<IPSetDetail>(`/admin/ip-sets/${id}`);
  },

  async createIPSet(data: {
    name: string;
    network: string;
    gateway: string;
    ip_version: number;
    location_id?: string;
    vlan_id?: number;
    node_ids?: string[];
  }): Promise<IPSet> {
    return apiClient.post<IPSet>("/admin/ip-sets", data);
  },

  async updateIPSet(id: string, data: UpdateIPSetRequest): Promise<IPSet> {
    return apiClient.put<IPSet>(`/admin/ip-sets/${id}`, data);
  },

  async deleteIPSet(id: string): Promise<void> {
    return apiClient.deleteVoid(`/admin/ip-sets/${id}`);
  },

  async getAvailableIPs(id: string, page = 1, perPage = 50): Promise<PaginatedResponse<{ id: string; address: string }>> {
    return apiClient.get<PaginatedResponse<{ id: string; address: string }>>(
      `/admin/ip-sets/${id}/available?page=${page}&per_page=${perPage}`
    );
  },
};

// ============================================================================
// Admin Failover Requests API
// ============================================================================

export interface FailoverRequest {
  id: string;
  node_id: string;
  status: string;
  created_at: string;
  updated_at: string;
}

export const adminFailoverRequestsApi = {
  async getFailoverRequests(): Promise<FailoverRequest[]> {
    return apiClient.get<FailoverRequest[]>("/admin/failover-requests");
  },

  async getFailoverRequest(id: string): Promise<FailoverRequest> {
    return apiClient.get<FailoverRequest>(`/admin/failover-requests/${id}`);
  },
};

// ============================================================================
// Per-VM Backup Schedule API
// ============================================================================

export interface VMBackupSchedule {
  id: string;
  vm_id: string;
  frequency: "daily" | "weekly" | "monthly";
  retention_count: number;
  active: boolean;
  next_run_at?: string;
  last_run_at?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateVMBackupScheduleRequest {
  frequency: "daily" | "weekly" | "monthly";
  retention_count: number;
  active?: boolean;
}

export interface UpdateVMBackupScheduleRequest {
  frequency?: "daily" | "weekly" | "monthly";
  retention_count?: number;
  active?: boolean;
}

export const adminVMBackupSchedulesApi = {
  async getVMBackupSchedules(vmId: string): Promise<VMBackupSchedule[]> {
    return apiClient.get<VMBackupSchedule[]>(`/admin/backup-schedules?vm_id=${vmId}`);
  },

  async createVMBackupSchedule(vmId: string, data: CreateVMBackupScheduleRequest): Promise<VMBackupSchedule> {
    return apiClient.post<VMBackupSchedule>(`/admin/backup-schedules`, { ...data, vm_id: vmId });
  },

  async updateVMBackupSchedule(_vmId: string, scheduleId: string, data: UpdateVMBackupScheduleRequest): Promise<VMBackupSchedule> {
    return apiClient.put<VMBackupSchedule>(`/admin/backup-schedules/${scheduleId}`, data);
  },

  async deleteVMBackupSchedule(_vmId: string, scheduleId: string): Promise<void> {
    return apiClient.deleteVoid(`/admin/backup-schedules/${scheduleId}`);
  },
};

// ============================================================================
// Storage Backend API
// ============================================================================

export interface StorageBackendNode {
  node_id: string;
  hostname: string;
  enabled: boolean;
}

export interface StorageBackend {
  id: string;
  name: string;
  type: "ceph" | "qcow" | "lvm";
  // Ceph-specific fields
  ceph_pool?: string;
  ceph_user?: string;
  ceph_monitors?: string;
  ceph_keyring_path?: string;
  // QCOW-specific fields
  storage_path?: string;
  // LVM-specific fields
  lvm_volume_group?: string;
  lvm_thin_pool?: string;
  // LVM threshold configuration (alerts trigger when usage exceeds these)
  lvm_data_percent_threshold?: number;
  lvm_metadata_percent_threshold?: number;
  // Capacity metrics
  total_gb?: number;
  used_gb?: number;
  available_gb?: number;
  // Health status
  health_status: "healthy" | "warning" | "critical" | "unknown";
  health_message?: string;
  // LVM-specific metrics
  lvm_data_percent?: number;
  lvm_metadata_percent?: number;
  // Node assignments
  nodes?: StorageBackendNode[];
  created_at: string;
  updated_at: string;
}

export interface CreateStorageBackendRequest {
  name: string;
  type: "ceph" | "qcow" | "lvm";
  ceph_pool?: string;
  ceph_user?: string;
  ceph_monitors?: string;
  ceph_keyring_path?: string;
  storage_path?: string;
  lvm_volume_group?: string;
  lvm_thin_pool?: string;
  lvm_data_percent_threshold?: number;
  lvm_metadata_percent_threshold?: number;
  node_ids?: string[];
}

export interface UpdateStorageBackendRequest {
  name?: string;
  ceph_pool?: string;
  ceph_user?: string;
  ceph_monitors?: string;
  ceph_keyring_path?: string;
  storage_path?: string;
  lvm_volume_group?: string;
  lvm_thin_pool?: string;
  lvm_data_percent_threshold?: number;
  lvm_metadata_percent_threshold?: number;
}

export const adminStorageBackendsApi = {
  async getStorageBackends(): Promise<StorageBackend[]> {
    return apiClient.get<StorageBackend[]>("/admin/storage-backends");
  },

  async getStorageBackend(id: string): Promise<StorageBackend> {
    return apiClient.get<StorageBackend>(`/admin/storage-backends/${id}`);
  },

  async createStorageBackend(data: CreateStorageBackendRequest): Promise<StorageBackend> {
    return apiClient.post<StorageBackend>("/admin/storage-backends", data);
  },

  async updateStorageBackend(id: string, data: UpdateStorageBackendRequest): Promise<StorageBackend> {
    return apiClient.put<StorageBackend>(`/admin/storage-backends/${id}`, data);
  },

  async deleteStorageBackend(id: string): Promise<void> {
    return apiClient.deleteVoid(`/admin/storage-backends/${id}`);
  },

  async getStorageBackendNodes(id: string): Promise<StorageBackendNode[]> {
    return apiClient.get<StorageBackendNode[]>(`/admin/storage-backends/${id}/nodes`);
  },

  async assignStorageBackendNodes(id: string, nodeIds: string[]): Promise<void> {
    return apiClient.postVoid(`/admin/storage-backends/${id}/nodes`, { node_ids: nodeIds });
  },

  async removeStorageBackendNode(id: string, nodeId: string): Promise<void> {
    return apiClient.deleteVoid(`/admin/storage-backends/${id}/nodes/${nodeId}`);
  },

  async getStorageBackendHealth(id: string): Promise<{ health_status: string; health_message?: string }> {
    return apiClient.get<{ health_status: string; health_message?: string }>(`/admin/storage-backends/${id}/health`);
  },

  async refreshStorageBackendHealth(id: string): Promise<StorageBackend> {
    return apiClient.post<StorageBackend>(`/admin/storage-backends/${id}/refresh`, {});
  },
};