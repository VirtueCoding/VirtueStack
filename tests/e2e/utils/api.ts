/**
 * API Utilities
 *
 * Provides API request helpers for E2E tests.
 */

import { APIRequestContext, APIResponse } from '@playwright/test';

const BASE_URL = process.env.BASE_URL || 'http://localhost:8080';

export interface APIClientOptions {
  baseUrl?: string;
  token?: string;
}

/**
 * API Client for making authenticated requests
 */
export class APIClient {
  private baseUrl: string;
  private token?: string;

  constructor(private request: APIRequestContext, options: APIClientOptions = {}) {
    this.baseUrl = options.baseUrl || BASE_URL;
    this.token = options.token;
  }

  private getHeaders(): Record<string, string> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };
    if (this.token) {
      headers['Authorization'] = `Bearer ${this.token}`;
    }
    return headers;
  }

  async get<T = any>(path: string): Promise<{ data: T; status: number }> {
    const resp = await this.request.get(`${this.baseUrl}${path}`, {
      headers: this.getHeaders(),
    });
    const data = resp.ok() ? await resp.json() : null;
    return { data, status: resp.status() };
  }

  async post<T = any>(path: string, body: any): Promise<{ data: T; status: number }> {
    const resp = await this.request.post(`${this.baseUrl}${path}`, {
      headers: this.getHeaders(),
      data: JSON.stringify(body),
    });
    const data = resp.ok() ? await resp.json() : null;
    return { data, status: resp.status() };
  }

  async put<T = any>(path: string, body: any): Promise<{ data: T; status: number }> {
    const resp = await this.request.put(`${this.baseUrl}${path}`, {
      headers: this.getHeaders(),
      data: JSON.stringify(body),
    });
    const data = resp.ok() ? await resp.json() : null;
    return { data, status: resp.status() };
  }

  async delete(path: string): Promise<{ status: number }> {
    const resp = await this.request.delete(`${this.baseUrl}${path}`, {
      headers: this.getHeaders(),
    });
    return { status: resp.status() };
  }
}

/**
 * Admin API Client
 */
export class AdminAPIClient extends APIClient {
  constructor(request: APIRequestContext) {
    super(request, { token: process.env.ADMIN_TOKEN });
  }
}

/**
 * Customer API Client
 */
export class CustomerAPIClient extends APIClient {
  constructor(request: APIRequestContext) {
    super(request, { token: process.env.CUSTOMER_TOKEN });
  }
}

/**
 * Helper to get first VM ID for testing
 */
export async function getFirstCustomerVMId(request: APIRequestContext): Promise<string | null> {
  try {
    const client = new CustomerAPIClient(request);
    const { data, status } = await client.get('/api/v1/customer/vms');
    if (status === 200 && data?.data?.length > 0) {
      return data.data[0].id;
    }
  } catch {
    // Fall back to env var
  }
  return process.env.TEST_CUSTOMER_VM_ID || null;
}

/**
 * Helper to get first admin VM ID for testing
 */
export async function getFirstAdminVMId(request: APIRequestContext): Promise<string | null> {
  try {
    const client = new AdminAPIClient(request);
    const { data, status } = await client.get('/api/v1/admin/vms');
    if (status === 200 && data?.data?.length > 0) {
      return data.data[0].id;
    }
  } catch {
    // Fall back to env var
  }
  return process.env.TEST_ADMIN_VM_ID || null;
}

/**
 * Helper to create a test VM and return its ID
 */
export async function createTestVM(
  request: APIRequestContext,
  vmData: {
    customer_id: string;
    plan_id: string;
    template_id: string;
    node_id: string;
    hostname: string;
    password: string;
  }
): Promise<string | null> {
  const client = new AdminAPIClient(request);
  const { data, status } = await client.post('/api/v1/admin/vms', vmData);
  if (status === 201 && data?.id) {
    return data.id;
  }
  return null;
}

/**
 * Helper to delete a VM
 */
export async function deleteTestVM(request: APIRequestContext, vmId: string): Promise<boolean> {
  const client = new AdminAPIClient(request);
  const { status } = await client.delete(`/api/v1/admin/vms/${vmId}`);
  return status === 200 || status === 204;
}

/**
 * Test data IDs (from seed script)
 */
export const TEST_IDS = {
  plans: {
    basic: '11111111-1111-1111-1111-111111111001',
    standard: '11111111-1111-1111-1111-111111111002',
    premium: '11111111-1111-1111-1111-111111111003',
    enterprise: '11111111-1111-1111-1111-111111111004',
  },
  locations: {
    usEast: '22222222-2222-2222-2222-222222222001',
    usWest: '22222222-2222-2222-2222-222222222002',
  },
  nodes: {
    node1: '33333333-3333-3333-3333-333333333001',
    node2: '33333333-3333-3333-3333-333333333002',
    node3: '33333333-3333-3333-3333-333333333003',
    node4: '33333333-3333-3333-3333-333333333004',
    node5: '33333333-3333-3333-3333-333333333005',
  },
  ipSets: {
    public: '44444444-4444-4444-4444-444444444001',
    private: '44444444-4444-4444-4444-444444444002',
  },
  templates: {
    ubuntu2204: '66666666-6666-6666-6666-666666666001',
    ubuntu2404: '66666666-6666-6666-6666-666666666002',
    debian12: '66666666-6666-6666-6666-666666666003',
    rocky9: '66666666-6666-6666-6666-666666666004',
    almalinux9: '66666666-6666-6666-6666-666666666005',
  },
  admins: {
    primary: '77777777-7777-7777-7777-777777777001',
    secondary: '77777777-7777-7777-7777-777777777002',
    with2FA: '77777777-7777-7777-7777-777777777003',
  },
  customers: {
    primary: '88888888-8888-8888-8888-888888888001',
    secondary: '88888888-8888-8888-8888-888888888002',
    with2FA: '88888888-8888-8888-8888-888888888003',
  },
  vms: {
    testVM1: '99999999-9999-9999-9999-999999999001',
    testVM2: '99999999-9999-9999-9999-999999999002',
    testVM3: '99999999-9999-9999-9999-999999999003',
  },
  backups: {
    backup1: 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa01',
    backup2: 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa02',
    backup3: 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa03',
  },
  snapshots: {
    snapshot1: 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb01',
    snapshot2: 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb02',
    snapshot3: 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb03',
  },
  apiKeys: {
    key1: 'cccccccc-cccc-cccc-cccc-cccccccccc01',
    key2: 'cccccccc-cccc-cccc-cccc-cccccccccc02',
  },
  webhooks: {
    webhook1: 'dddddddd-dddd-dddd-dddd-dddddddddd01',
  },
} as const;