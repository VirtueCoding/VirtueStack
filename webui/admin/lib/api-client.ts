const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || "/api/v1";
import {
  ApiClientError,
  apiRequest as sharedAPIRequest,
  fetchCsrfToken as sharedFetchCsrfToken,
  logoutWithRefreshRecovery,
} from "@virtuestack/api-client";

import type {
  AuthTokens,
  LoginRequest,
  Verify2FARequest,
} from "@virtuestack/api-client";

export { ApiClientError };
export type { AuthTokens, LoginRequest, Verify2FARequest };

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

async function fetchCsrfToken(): Promise<void> {
  await sharedFetchCsrfToken(API_BASE_URL, "/admin/auth/csrf");
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
  return sharedAPIRequest<T | void>(API_BASE_URL, endpoint, options);
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

  deleteVoidWithHeaders(endpoint: string, headers: HeadersInit): Promise<void> {
    return apiRequest(endpoint, { method: "DELETE", headers, expectNoContent: true });
  },
};

export interface ReauthResponse {
  reauth_token: string;
  expires_in: number;
}

export interface AuthSessionResponse extends AuthTokens {
  user?: AdminUser;
}

export const adminAuthApi = {
  async login(credentials: LoginRequest): Promise<AuthSessionResponse> {
    await fetchCsrfToken();
    return apiClient.post<AuthSessionResponse>("/admin/auth/login", credentials);
  },

  async verify2FA(request: Verify2FARequest): Promise<AuthSessionResponse> {
    return apiClient.post<AuthSessionResponse>("/admin/auth/verify-2fa", request);
  },

  async logout(): Promise<void> {
    await logoutWithRefreshRecovery({
      invalidateSession: () => adminAuthApi.invalidateSession(),
      refreshSession: () => adminAuthApi.refresh(),
    });
  },

  async invalidateSession(sessionCleanupToken?: string): Promise<void> {
    await apiClient.post("/admin/auth/logout", sessionCleanupToken
      ? { session_cleanup_token: sessionCleanupToken }
      : {});
  },

  async refresh(): Promise<AuthSessionResponse> {
    return apiClient.post<AuthSessionResponse>("/admin/auth/refresh", {});
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
  async getNodes(params: { per_page?: number; cursor?: string } = {}): Promise<PaginatedResponse<Node>> {
    const searchParams = new URLSearchParams();
    if (params.per_page !== undefined) searchParams.set("per_page", String(params.per_page));
    if (params.cursor) searchParams.set("cursor", params.cursor);
    const query = searchParams.toString();
    return apiClient.get<PaginatedResponse<Node>>(`/admin/nodes${query ? `?${query}` : ""}`);
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
  status: "active" | "pending_verification" | "suspended" | "deleted";
  created_at: string;
  updated_at?: string;
  phone?: string;
  auth_provider?: "local" | "oauth";
}

export interface CustomerDetail extends Customer {
  active_vms: number;
  backup_count: number;
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
  async getCustomers(params: { per_page?: number; cursor?: string; status?: string; search?: string } = {}): Promise<PaginatedResponse<Customer>> {
    const searchParams = new URLSearchParams();
    if (params.per_page !== undefined) searchParams.set("per_page", String(params.per_page));
    if (params.cursor) searchParams.set("cursor", params.cursor);
    if (params.status) searchParams.set("status", params.status);
    if (params.search) searchParams.set("search", params.search);
    const query = searchParams.toString();
    return apiClient.get<PaginatedResponse<Customer>>(`/admin/customers${query ? `?${query}` : ""}`);
  },

  async getCustomer(id: string): Promise<CustomerDetail> {
    return apiClient.get<CustomerDetail>(`/admin/customers/${id}`);
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

  async getCustomerAuditLogs(id: string, perPage = 20, cursor?: string): Promise<PaginatedResponse<AuditLog>> {
    const searchParams = new URLSearchParams();
    searchParams.set("per_page", String(perPage));
    if (cursor) searchParams.set("cursor", cursor);
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
  async getAuditLogs(perPage = 20, search?: string, cursor?: string): Promise<PaginatedResponse<AuditLog>> {
    const searchParams = new URLSearchParams();
    if (cursor) searchParams.set("cursor", cursor);
    searchParams.set("per_page", String(perPage));
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

export interface AdminVMListParams {
  per_page?: number;
  cursor?: string;
  customer_id?: string;
  node_id?: string;
  status?: string;
  search?: string;
}

export const adminVMsApi = {
  async getVMs(params: AdminVMListParams = {}): Promise<PaginatedResponse<VM>> {
    const searchParams = new URLSearchParams();
    if (params.per_page !== undefined) searchParams.set("per_page", String(params.per_page));
    if (params.cursor) searchParams.set("cursor", params.cursor);
    if (params.customer_id) searchParams.set("customer_id", params.customer_id);
    if (params.node_id) searchParams.set("node_id", params.node_id);
    if (params.status) searchParams.set("status", params.status);
    if (params.search) searchParams.set("search", params.search);
    const query = searchParams.toString();
    return apiClient.get<PaginatedResponse<VM>>(`/admin/vms${query ? `?${query}` : ""}`);
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
  storage_backend?: "ceph" | "qcow" | "lvm";
  file_path?: string;
}

export interface TemplateCacheEntry {
  template_id: string;
  node_id: string;
  status: "pending" | "downloading" | "ready" | "failed";
  local_path?: string;
  size_bytes?: number;
  synced_at?: string;
  error_msg?: string;
  created_at: string;
  updated_at: string;
}

export interface TemplateCacheStatusResponse {
  template_id: string;
  entries: TemplateCacheEntry[];
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
    storage_backend?: "ceph" | "qcow" | "lvm";
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

  async buildTemplateFromISO(data: {
    name: string;
    os_family: string;
    os_version: string;
    iso_path?: string;
    iso_url?: string;
    node_id: string;
    storage_backend: string;
    disk_size_gb?: number;
    memory_mb?: number;
    vcpus?: number;
    root_password?: string;
    custom_install_config?: string;
  }): Promise<{ task_id: string }> {
    return apiClient.post<{ task_id: string }>("/admin/templates/build-from-iso", data);
  },

  async distributeTemplate(id: string, nodeIds: string[]): Promise<{ task_id: string }> {
    return apiClient.post<{ task_id: string }>(`/admin/templates/${id}/distribute`, {
      node_ids: nodeIds,
    });
  },

  async getTemplateCacheStatus(id: string): Promise<TemplateCacheStatusResponse> {
    return apiClient.get<TemplateCacheStatusResponse>(`/admin/templates/${id}/cache-status`);
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
  per_page?: number;
  cursor?: string;
  customer_id?: string;
  vm_id?: string;
  status?: string;
  source?: string;
  admin_schedule_id?: string;
}

export interface PaginatedResponse<T> {
  data: T[];
  meta: {
    per_page: number;
    has_more?: boolean;
    next_cursor?: string;
    prev_cursor?: string;
  };
}

export const adminBackupsApi = {
  async getBackups(params: AdminBackupListParams = {}): Promise<PaginatedResponse<AdminBackup>> {
    const searchParams = new URLSearchParams();
    if (params.cursor) searchParams.set("cursor", params.cursor);
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
  async getSchedules(params: { per_page?: number; cursor?: string; active?: boolean } = {}): Promise<PaginatedResponse<AdminBackupSchedule>> {
    const searchParams = new URLSearchParams();
    if (params.cursor) searchParams.set("cursor", params.cursor);
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

  async getAvailableIPs(id: string, perPage = 50, cursor?: string): Promise<PaginatedResponse<{ id: string; address: string }>> {
    const searchParams = new URLSearchParams();
    searchParams.set("per_page", String(perPage));
    if (cursor) searchParams.set("cursor", cursor);
    return apiClient.get<PaginatedResponse<{ id: string; address: string }>>(
      `/admin/ip-sets/${id}/available?${searchParams.toString()}`
    );
  },
};

// ============================================================================
// Admin Failover Requests API
// ============================================================================

export interface FailoverRequest {
  id: string;
  node_id: string;
  requested_by: string;
  status: string;
  reason?: string;
  result?: Record<string, unknown> | string | null;
  approved_at?: string;
  completed_at?: string;
  created_at: string;
  updated_at: string;
}

export const adminFailoverRequestsApi = {
  async getFailoverRequests(params: {
    per_page?: number;
    cursor?: string;
    node_id?: string;
    status?: string;
  } = {}): Promise<PaginatedResponse<FailoverRequest>> {
    const searchParams = new URLSearchParams();
    if (params.per_page !== undefined) searchParams.set("per_page", String(params.per_page));
    if (params.cursor) searchParams.set("cursor", params.cursor);
    if (params.node_id) searchParams.set("node_id", params.node_id);
    if (params.status) searchParams.set("status", params.status);

    const query = searchParams.toString();
    const endpoint = query ? `/admin/failover-requests?${query}` : "/admin/failover-requests";
    return apiClient.get<PaginatedResponse<FailoverRequest>>(endpoint);
  },

  async getFailoverRequest(id: string): Promise<FailoverRequest> {
    return apiClient.get<FailoverRequest>(`/admin/failover-requests/${id}`);
  },
};

// ============================================================================
// Admin Provisioning Keys API
// ============================================================================

export interface AdminProvisioningKey {
  id: string;
  name: string;
  allowed_ips?: string[];
  last_used_at?: string;
  revoked_at?: string;
  created_at: string;
  created_by: string;
  description?: string;
  expires_at?: string;
}

export interface CreateProvisioningKeyRequest {
  name: string;
  allowed_ips?: string[];
  description?: string;
  expires_at?: string;
}

export interface UpdateProvisioningKeyRequest {
  name?: string;
  allowed_ips?: string[];
  description?: string;
  expires_at?: string | null;
}

export interface ProvisioningKeySecretResponse {
  id: string;
  name: string;
  key: string;
  created_at: string;
}

export const adminProvisioningKeysApi = {
  async getProvisioningKeys(includeRevoked = false): Promise<AdminProvisioningKey[]> {
    const query = includeRevoked ? "?include_revoked=true" : "";
    return apiClient.get<AdminProvisioningKey[]>(`/admin/provisioning-keys${query}`);
  },

  async getProvisioningKey(id: string): Promise<AdminProvisioningKey> {
    return apiClient.get<AdminProvisioningKey>(`/admin/provisioning-keys/${id}`);
  },

  async createProvisioningKey(data: CreateProvisioningKeyRequest): Promise<ProvisioningKeySecretResponse> {
    return apiClient.post<ProvisioningKeySecretResponse>("/admin/provisioning-keys", data);
  },

  async updateProvisioningKey(id: string, data: UpdateProvisioningKeyRequest): Promise<AdminProvisioningKey> {
    return apiClient.put<AdminProvisioningKey>(`/admin/provisioning-keys/${id}`, data);
  },

  async revokeProvisioningKey(id: string, reauthToken: string): Promise<void> {
    return apiClient.deleteVoidWithHeaders(`/admin/provisioning-keys/${id}`, {
      "X-Reauth-Token": reauthToken,
    });
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

  /** vmId is retained for call-site compatibility; the backend identifies the schedule by scheduleId. */
  async updateVMBackupSchedule(_vmId: string, scheduleId: string, data: UpdateVMBackupScheduleRequest): Promise<VMBackupSchedule> {
    return apiClient.put<VMBackupSchedule>(`/admin/backup-schedules/${scheduleId}`, data);
  },

  /** vmId is retained for call-site compatibility; the backend identifies the schedule by scheduleId. */
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

  async assignStorageBackendNodes(id: string, nodeIds: string[]): Promise<StorageBackendNode[]> {
    return apiClient.post<StorageBackendNode[]>(`/admin/storage-backends/${id}/nodes`, { node_ids: nodeIds });
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
    return apiClient.get<InAppNotificationListResponse>(`/admin/notifications${qs ? `?${qs}` : ""}`);
  },

  async markAsRead(id: string): Promise<void> {
    return apiClient.postVoid(`/admin/notifications/${id}/read`, {});
  },

  async markAllAsRead(): Promise<void> {
    return apiClient.postVoid("/admin/notifications/read-all", {});
  },

  async getUnreadCount(): Promise<UnreadCountResponse> {
    return apiClient.get<UnreadCountResponse>("/admin/notifications/unread-count");
  },
};

// Billing types (admin)
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

export interface BillingConfig {
  top_up: {
    min_amount_cents: number;
    max_amount_cents: number;
    presets: number[];
    gateways: string[];
    currency: string;
  };
  gateways: string[];
}

export interface RefundRequest {
  amount: number;
  reason: string;
}

export interface RefundResult {
  gateway_refund_id: string;
  gateway_payment_id: string;
  amount_cents: number;
  currency: string;
  status: string;
}

export const adminBillingApi = {
  async getTransactions(
    params: { perPage?: number; cursor?: string; customerID?: string } = {}
  ): Promise<PaginatedResponse<BillingTransaction>> {
    const queryParams = new URLSearchParams();
    if (params.perPage) queryParams.set("per_page", String(params.perPage));
    if (params.cursor) queryParams.set("cursor", params.cursor);
    if (params.customerID) queryParams.set("customer_id", params.customerID);
    const query = queryParams.toString() ? `?${queryParams.toString()}` : "";
    return apiClient.get<PaginatedResponse<BillingTransaction>>(
      `/admin/billing/transactions${query}`
    );
  },

  async creditAdjustment(
    customerID: string,
    amount: number,
    description: string
  ): Promise<BillingTransaction> {
    await fetchCsrfToken();
    return apiClient.post<BillingTransaction>(
      `/admin/billing/credit?customer_id=${customerID}`,
      { amount, description }
    );
  },

  async getPayments(
    params: {
      perPage?: number;
      cursor?: string;
      customerID?: string;
      gateway?: string;
      status?: string;
    } = {}
  ): Promise<PaginatedResponse<BillingPayment>> {
    const queryParams = new URLSearchParams();
    if (params.perPage) queryParams.set("per_page", String(params.perPage));
    if (params.cursor) queryParams.set("cursor", params.cursor);
    if (params.customerID) queryParams.set("customer_id", params.customerID);
    if (params.gateway) queryParams.set("gateway", params.gateway);
    if (params.status) queryParams.set("status", params.status);
    const query = queryParams.toString() ? `?${queryParams.toString()}` : "";
    return apiClient.get<PaginatedResponse<BillingPayment>>(
      `/admin/billing/payments${query}`
    );
  },

  async refundPayment(
    paymentID: string,
    req: RefundRequest
  ): Promise<RefundResult> {
    await fetchCsrfToken();
    return apiClient.post<RefundResult>(
      `/admin/billing/refund/${paymentID}`,
      req
    );
  },

  async getConfig(): Promise<BillingConfig> {
    return apiClient.get<BillingConfig>(
      "/admin/billing/config"
    );
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
  customer_name?: string;
  customer_email?: string;
}

export interface InvoiceListParams {
  customer_id?: string;
  status?: string;
  start_date?: string;
  end_date?: string;
  cursor?: string;
  per_page?: number;
}

export const adminInvoicesApi = {
  async list(
    params: InvoiceListParams = {}
  ): Promise<PaginatedResponse<Invoice>> {
    const searchParams = new URLSearchParams();
    if (params.customer_id) searchParams.set("customer_id", params.customer_id);
    if (params.status) searchParams.set("status", params.status);
    if (params.start_date) searchParams.set("start_date", params.start_date);
    if (params.end_date) searchParams.set("end_date", params.end_date);
    if (params.cursor) searchParams.set("cursor", params.cursor);
    if (params.per_page) searchParams.set("per_page", String(params.per_page));
    const query = searchParams.toString() ? `?${searchParams.toString()}` : "";
    return apiClient.get<PaginatedResponse<Invoice>>(
      `/admin/invoices${query}`
    );
  },

  async get(id: string): Promise<Invoice> {
    return apiClient.get<Invoice>(
      `/admin/invoices/${id}`
    );
  },

  async voidInvoice(id: string): Promise<{ status: string }> {
    await fetchCsrfToken();
    return apiClient.post<{ status: string }>(
      `/admin/invoices/${id}/void`,
      {}
    );
  },

  getPDFUrl(id: string): string {
    return `${API_BASE_URL}/admin/invoices/${id}/pdf`;
  },
};
