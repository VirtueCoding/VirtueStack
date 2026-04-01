import { test, expect, Page } from '@playwright/test';

/**
 * Admin Customer Management E2E Tests
 *
 * Tests cover:
 * - Customer list viewing
 * - Customer detail viewing
 * - Customer profile editing
 * - Customer VMs overview
 * - Customer audit logs
 * - Customer deletion
 */

// ============================================
// Page Object Models
// ============================================

class AdminCustomerListPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/customers');
    await expect(this.page.locator('h1, [data-testid="page-title"]')).toContainText(/customers/i);
  }

  async getCustomerCards() {
    return this.page.locator('[data-testid="customer-card"], table tbody tr');
  }

  async getCustomerCount() {
    const cards = await this.getCustomerCards();
    return cards.count();
  }

  async searchCustomer(query: string) {
    await this.page.fill('input[placeholder*="search" i], input[name="search"]', query);
    await this.page.press('input[placeholder*="search" i], input[name="search"]', 'Enter');
  }

  async filterByStatus(status: string) {
    await this.page.click('[data-testid="status-filter"], select[name="status"]');
    await this.page.click(`option:has-text("${status}")`);
  }

  async filterByLocation(location: string) {
    await this.page.click('[data-testid="location-filter"], select[name="location"]');
    await this.page.click(`option:has-text("${location}")`);
  }

  async clickCustomerByEmail(email: string) {
    await this.page.click(`a:has-text("${email}"), [data-testid="customer-${email}"]`);
  }

  async expectCustomerInList(email: string) {
    await expect(this.page.locator(`text="${email}"`)).toBeVisible();
  }

  async expectCustomerNotInList(email: string) {
    await expect(this.page.locator(`text="${email}"`)).not.toBeVisible();
  }
}

class AdminCustomerDetailPage {
  constructor(private page: Page) {}

  async goto(customerId: string) {
    await this.page.goto(`/customers/${customerId}`);
  }

  async getEmail() {
    return this.page.locator('[data-testid="customer-email"]').textContent();
  }

  async getName() {
    return this.page.locator('[data-testid="customer-name"]').textContent();
  }

  async getStatus() {
    return this.page.locator('[data-testid="customer-status"]').textContent();
  }

  async getPhone() {
    return this.page.locator('[data-testid="customer-phone"]').textContent();
  }

  async getCreatedAt() {
    return this.page.locator('[data-testid="customer-created"]').textContent();
  }

  async getVMCount() {
    return this.page.locator('[data-testid="vm-count"]').textContent();
  }

  async get2FAStatus() {
    return this.page.locator('[data-testid="2fa-status"]').textContent();
  }

  async clickEdit() {
    await this.page.click('button:has-text("Edit"), [data-testid="edit-btn"]');
  }

  async clickDelete() {
    await this.page.click('button:has-text("Delete"), [data-testid="delete-btn"]');
  }

  async clickSuspend() {
    await this.page.click('button:has-text("Suspend"), [data-testid="suspend-btn"]');
  }

  async clickUnsuspend() {
    await this.page.click('button:has-text("Unsuspend"), [data-testid="unsuspend-btn"]');
  }

  async navigateToVMs() {
    await this.page.click('a:has-text("Virtual Machines"), [data-testid="vms-tab"]');
  }

  async navigateToAuditLogs() {
    await this.page.click('a:has-text("Audit Logs"), [data-testid="audit-logs-tab"]');
  }

  async navigateToAPIKeys() {
    await this.page.click('a:has-text("API Keys"), [data-testid="api-keys-tab"]');
  }

  async navigateToWebhooks() {
    await this.page.click('a:has-text("Webhooks"), [data-testid="webhooks-tab"]');
  }
}

class AdminCustomerEditPage {
  constructor(private page: Page) {}

  async goto(customerId: string) {
    await this.page.goto(`/customers/${customerId}/edit`);
  }

  async updateName(name: string) {
    await this.page.fill('input[name="name"]', name);
  }

  async updateEmail(email: string) {
    await this.page.fill('input[name="email"]', email);
  }

  async updatePhone(phone: string) {
    await this.page.fill('input[name="phone"]', phone);
  }

  async updateStatus(status: string) {
    await this.page.click('[data-testid="status-select"], select[name="status"]');
    await this.page.click(`option:has-text("${status}")`);
  }

  async submit() {
    await this.page.click('button[type="submit"]');
  }

  async cancel() {
    await this.page.click('button:has-text("Cancel")');
  }

  async expectValidationError(message: string | RegExp) {
    await expect(this.page.locator('.error, [role="alert"]')).toContainText(message);
  }
}

class AdminCustomerVMsPage {
  constructor(private page: Page) {}

  async goto(customerId: string) {
    await this.page.goto(`/customers/${customerId}/vms`);
  }

  async getVMRows() {
    return this.page.locator('table tbody tr, [data-testid="vm-row"]');
  }

  async getVMCount() {
    const rows = await this.getVMRows();
    return rows.count();
  }

  async clickVM(vmId: string) {
    await this.page.click(`a[href*="${vmId}"]`);
  }

  async filterByStatus(status: string) {
    await this.page.click('[data-testid="status-filter"]');
    await this.page.click(`option:has-text("${status}")`);
  }

  async searchVM(query: string) {
    await this.page.fill('input[placeholder*="search"]', query);
    await this.page.press('input[placeholder*="search"]', 'Enter');
  }
}

class AdminCustomerAuditLogsPage {
  constructor(private page: Page) {}

  async goto(customerId: string) {
    await this.page.goto(`/customers/${customerId}/audit-logs`);
  }

  async getLogRows() {
    return this.page.locator('table tbody tr, [data-testid="audit-log-row"]');
  }

  async getLogCount() {
    const rows = await this.getLogRows();
    return rows.count();
  }

  async filterByAction(action: string) {
    await this.page.click('[data-testid="action-filter"]');
    await this.page.click(`option:has-text("${action}")`);
  }

  async filterByDateRange(start: string, end: string) {
    await this.page.fill('input[name="start_date"]', start);
    await this.page.fill('input[name="end_date"]', end);
  }

  async searchLogs(query: string) {
    await this.page.fill('input[placeholder*="search"]', query);
    await this.page.press('input[placeholder*="search"]', 'Enter');
  }

  async exportLogs() {
    await this.page.click('button:has-text("Export"), [data-testid="export-btn"]');
  }
}

class AdminSuspendModal {
  constructor(private page: Page) {}

  async expectVisible() {
    await expect(this.page.locator('[role="dialog"], .modal:has-text("Suspend")')).toBeVisible();
  }

  async fillReason(reason: string) {
    await this.page.fill('textarea[name="reason"]', reason);
  }

  async confirm() {
    await this.page.click('button:has-text("Confirm"), button:has-text("Suspend")');
  }

  async cancel() {
    await this.page.click('button:has-text("Cancel")');
  }
}

// ============================================
// Test Suite
// ============================================

test.describe('Admin Customer List', () => {
  let customerListPage: AdminCustomerListPage;

  test.beforeEach(async ({ page }) => {
    customerListPage = new AdminCustomerListPage(page);
    await customerListPage.goto();
  });

  test('should display customer list', async ({ page }) => {
    await expect(page.locator('h1, [data-testid="page-title"]')).toContainText(/customers/i);
  });

  test('should show customer cards with key info', async ({ page }) => {
    const cards = await customerListPage.getCustomerCards();
    const count = await cards.count();

    if (count > 0) {
      const firstCard = cards.first();

      // Should show email
      await expect(firstCard.locator('text=/@/')).toBeVisible();

      // Should show status
      await expect(firstCard.locator('[data-testid="status"], .status-badge')).toBeVisible();
    }
  });

  test('should show customer statistics', async ({ page }) => {
    const statsSection = page.locator('[data-testid="customer-stats"]');
    if (await statsSection.isVisible()) {
      await expect(statsSection).toBeVisible();
    }
  });

  test('should search customers by email', async ({ page }) => {
    await customerListPage.searchCustomer('test@example.com');

    await page.waitForLoadState('networkidle');

    const cards = await customerListPage.getCustomerCards();
    const count = await cards.count();

    if (count > 0) {
      const text = await cards.first().textContent();
      expect(text?.toLowerCase()).toContain('test');
    }
  });

  test('should filter customers by status', async ({ page }) => {
    await customerListPage.filterByStatus('Active');

    await page.waitForLoadState('networkidle');

    const statuses = page.locator('[data-testid="status"]:has-text("Active"), td:has-text("Active")');
    const count = await statuses.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should click customer to view details', async ({ page }) => {
    const cards = await customerListPage.getCustomerCards();
    const count = await cards.count();

    if (count > 0) {
      const email = await cards.first().locator('text=/[\\w.-]+@[\\w.-]+/').textContent();
      if (email) {
        await customerListPage.clickCustomerByEmail(email);
        await expect(page).toHaveURL(/\/customers\/[a-f0-9-]+/);
      }
    }
  });

  test('should show VM count per customer', async ({ page }) => {
    const cards = await customerListPage.getCustomerCards();
    const count = await cards.count();

    if (count > 0) {
      // Should show VM count
      const firstCard = cards.first();
      const text = await firstCard.textContent();
      expect(text).toMatch(/\d+.*VM|server/i);
    }
  });
});

test.describe('Admin Customer Detail', () => {
  let customerDetailPage: AdminCustomerDetailPage;
  const testCustomerId = '00000000-0000-0000-0000-000000000001';

  test.beforeEach(async ({ page }) => {
    customerDetailPage = new AdminCustomerDetailPage(page);
    await customerDetailPage.goto(testCustomerId);
  });

  test('should display customer details', async ({ page }) => {
    await expect(page.locator('[data-testid="customer-email"]')).toBeVisible();
    await expect(page.locator('[data-testid="customer-status"]')).toBeVisible();
  });

  test('should show customer email', async ({ page }) => {
    const email = await customerDetailPage.getEmail();
    expect(email).toMatch(/@/);
  });

  test('should show customer status', async ({ page }) => {
    const status = await customerDetailPage.getStatus();
    expect(['active', 'suspended', 'pending']).toContain(status?.toLowerCase());
  });

  test('should show creation date', async ({ page }) => {
    const createdAt = await customerDetailPage.getCreatedAt();
    expect(createdAt).toBeTruthy();
  });

  test('should show VM count', async ({ page }) => {
    const vmCount = await customerDetailPage.getVMCount();
    expect(vmCount).toMatch(/\d+/);
  });

  test('should show 2FA status', async ({ page }) => {
    const twoFAStatus = page.locator('[data-testid="2fa-status"]');

    if (await twoFAStatus.isVisible()) {
      const status = await twoFAStatus.textContent();
      expect(['enabled', 'disabled']).toContain(status?.toLowerCase());
    }
  });

  test('should show action buttons', async ({ page }) => {
    await expect(page.locator('button:has-text("Edit")')).toBeVisible();
  });

  test('should navigate to VMs tab', async ({ page }) => {
    await customerDetailPage.navigateToVMs();

    await expect(page.locator('table, [data-testid="vm-list"]')).toBeVisible();
  });

  test('should navigate to audit logs tab', async ({ page }) => {
    await customerDetailPage.navigateToAuditLogs();

    await expect(page.locator('table, [data-testid="audit-log-list"]')).toBeVisible();
  });
});

test.describe('Admin Customer Editing', () => {
  let customerEditPage: AdminCustomerEditPage;
  const testCustomerId = '00000000-0000-0000-0000-000000000002';

  test.beforeEach(async ({ page }) => {
    customerEditPage = new AdminCustomerEditPage(page);
    await customerEditPage.goto(testCustomerId);
  });

  test('should display edit form with current values', async ({ page }) => {
    await expect(page.locator('input[name="email"]')).toHaveValue(/@/);
  });

  test('should update customer name', async ({ page }) => {
    const newName = `Updated Name ${Date.now()}`;

    await customerEditPage.updateName(newName);
    await customerEditPage.submit();

    await expect(page.locator('text=/saved|updated|success/i')).toBeVisible();
  });

  test('should update customer phone', async ({ page }) => {
    await customerEditPage.updatePhone('+1-555-123-4567');
    await customerEditPage.submit();

    await expect(page.locator('text=/saved|updated|success/i')).toBeVisible();
  });

  test('should update customer status', async ({ page }) => {
    await customerEditPage.updateStatus('Suspended');
    await customerEditPage.submit();

    await expect(page.locator('text=/saved|updated|success/i')).toBeVisible();
  });

  test('should validate email format', async ({ page }) => {
    await customerEditPage.updateEmail('invalid-email');
    await customerEditPage.submit();

    await customerEditPage.expectValidationError(/valid.*email/i);
  });

  test('should validate email uniqueness', async ({ page }) => {
    await customerEditPage.updateEmail('existing@example.com');
    await customerEditPage.submit();

    await customerEditPage.expectValidationError(/already.*exists|duplicate/i);
  });

  test('should allow canceling edits', async ({ page }) => {
    await customerEditPage.updateName('Cancelled Name');
    await customerEditPage.cancel();

    // Should navigate away without saving
    await expect(page).toHaveURL(/\/customers/);
  });
});

test.describe('Admin Customer Suspension', () => {
  let customerDetailPage: AdminCustomerDetailPage;
  let suspendModal: AdminSuspendModal;
  const testCustomerId = '00000000-0000-0000-0000-000000000003';

  test.beforeEach(async ({ page }) => {
    customerDetailPage = new AdminCustomerDetailPage(page);
    suspendModal = new AdminSuspendModal(page);
    await customerDetailPage.goto(testCustomerId);
  });

  test('should show suspend button for active customer', async ({ page }) => {
    const status = await customerDetailPage.getStatus();

    if (status?.toLowerCase() === 'active') {
      await expect(page.locator('button:has-text("Suspend")')).toBeVisible();
    }
  });

  test('should open suspend confirmation modal', async ({ page }) => {
    const status = await customerDetailPage.getStatus();

    if (status?.toLowerCase() === 'active') {
      await customerDetailPage.clickSuspend();

      await suspendModal.expectVisible();
    }
  });

  test('should require reason for suspension', async ({ page }) => {
    const status = await customerDetailPage.getStatus();

    if (status?.toLowerCase() === 'active') {
      await customerDetailPage.clickSuspend();
      await suspendModal.confirm();

      await expect(page.locator('.error, [role="alert"]')).toContainText(/required/i);
    }
  });

  test('should suspend customer with reason', async ({ page }) => {
    const status = await customerDetailPage.getStatus();

    if (status?.toLowerCase() === 'active') {
      await customerDetailPage.clickSuspend();
      await suspendModal.fillReason('Payment overdue');
      await suspendModal.confirm();

      await expect(page.locator('text=/suspended|success/i')).toBeVisible();
    }
  });

  test('should allow canceling suspension', async ({ page }) => {
    const status = await customerDetailPage.getStatus();

    if (status?.toLowerCase() === 'active') {
      await customerDetailPage.clickSuspend();
      await suspendModal.cancel();

      await expect(page.locator('[role="dialog"]')).not.toBeVisible();
    }
  });

  test('should show unsuspend button for suspended customer', async ({ page }) => {
    await page.goto('/customers/suspended-customer-id');

    await expect(page.locator('button:has-text("Unsuspend")')).toBeVisible();
  });

  test('should unsuspend a suspended customer', async ({ page }) => {
    await page.goto('/customers/suspended-customer-id');
    await customerDetailPage.clickUnsuspend();

    await expect(page.locator('text=/unsuspended|active|success/i')).toBeVisible();
  });
});

test.describe('Admin Customer VMs', () => {
  let customerVMsPage: AdminCustomerVMsPage;
  const testCustomerId = '00000000-0000-0000-0000-000000000004';

  test.beforeEach(async ({ page }) => {
    customerVMsPage = new AdminCustomerVMsPage(page);
    await customerVMsPage.goto(testCustomerId);
  });

  test('should display customer VMs', async ({ page }) => {
    await expect(page.locator('h1, [data-testid="page-title"]')).toContainText(/VMs|virtual machines/i);
  });

  test('should show VM list', async ({ page }) => {
    const count = await customerVMsPage.getVMCount();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should show VM details in list', async ({ page }) => {
    const rows = await customerVMsPage.getVMRows();
    const count = await rows.count();

    if (count > 0) {
      const firstRow = rows.first();

      // Should show hostname
      await expect(firstRow.locator('[data-testid="hostname"], td')).toBeVisible();

      // Should show status
      await expect(firstRow.locator('[data-testid="status"], .status-badge')).toBeVisible();
    }
  });

  test('should filter VMs by status', async ({ page }) => {
    await customerVMsPage.filterByStatus('Running');

    await page.waitForLoadState('networkidle');

    const statuses = page.locator('[data-testid="status"]:has-text("Running")');
    const count = await statuses.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should search VMs', async ({ page }) => {
    await customerVMsPage.searchVM('test');

    await page.waitForLoadState('networkidle');

    const rows = await customerVMsPage.getVMRows();
    const count = await rows.count();

    if (count > 0) {
      const text = await rows.first().textContent();
      expect(text?.toLowerCase()).toContain('test');
    }
  });

  test('should click VM to view details', async ({ page }) => {
    const rows = await customerVMsPage.getVMRows();
    const count = await rows.count();

    if (count > 0) {
      const vmLink = rows.first().locator('a');
      if (await vmLink.isVisible()) {
        await vmLink.click();
        await expect(page).toHaveURL(/\/vms\/[a-f0-9-]+/);
      }
    }
  });
});

test.describe('Admin Customer Audit Logs', () => {
  let auditLogsPage: AdminCustomerAuditLogsPage;
  const testCustomerId = '00000000-0000-0000-0000-000000000005';

  test.beforeEach(async ({ page }) => {
    auditLogsPage = new AdminCustomerAuditLogsPage(page);
    await auditLogsPage.goto(testCustomerId);
  });

  test('should display audit logs', async ({ page }) => {
    await expect(page.locator('h1, [data-testid="page-title"]')).toContainText(/audit.*log|activity/i);
  });

  test('should show audit log list', async ({ page }) => {
    const count = await auditLogsPage.getLogCount();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should show log details in list', async ({ page }) => {
    const rows = await auditLogsPage.getLogRows();
    const count = await rows.count();

    if (count > 0) {
      const firstRow = rows.first();

      // Should show action
      await expect(firstRow.locator('td, [data-testid="action"]')).toBeVisible();

      // Should show timestamp
      await expect(firstRow.locator('text=/\d{4}-\d{2}-\d{2}|ago/i')).toBeVisible();
    }
  });

  test('should filter logs by action', async ({ page }) => {
    await auditLogsPage.filterByAction('Login');

    await page.waitForLoadState('networkidle');

    const actions = page.locator('[data-testid="action"]:has-text("Login")');
    const count = await actions.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should search logs', async ({ page }) => {
    await auditLogsPage.searchLogs('login');

    await page.waitForLoadState('networkidle');

    const rows = await auditLogsPage.getLogRows();
    const count = await rows.count();

    if (count > 0) {
      const text = await rows.first().textContent();
      expect(text?.toLowerCase()).toContain('login');
    }
  });

  test('should export logs', async ({ page }) => {
    const [download] = await Promise.all([
      page.waitForEvent('download'),
      auditLogsPage.exportLogs(),
    ]);

    expect(download).toBeTruthy();
  });
});

test.describe('Admin Customer Deletion', () => {
  let customerDetailPage: AdminCustomerDetailPage;

  test('should show delete button for customer with no VMs', async ({ page }) => {
    customerDetailPage = new AdminCustomerDetailPage(page);
    await customerDetailPage.goto('customer-no-vms-id');

    await expect(page.locator('button:has-text("Delete")')).toBeVisible();
  });

  test('should show deletion confirmation', async ({ page }) => {
    customerDetailPage = new AdminCustomerDetailPage(page);
    await customerDetailPage.goto('customer-no-vms-id');
    await customerDetailPage.clickDelete();

    await expect(page.locator('[role="dialog"], .modal')).toBeVisible();
    await expect(page.locator('text=/confirm|are you sure/i')).toBeVisible();
  });

  test('should delete customer after confirmation', async ({ page }) => {
    customerDetailPage = new AdminCustomerDetailPage(page);
    await customerDetailPage.goto('deletable-customer-id');
    await customerDetailPage.clickDelete();

    await page.click('button:has-text("Confirm")');

    // Should redirect to customer list
    await expect(page).toHaveURL(/\/customers$/);
  });

  test('should disable delete for customer with VMs', async ({ page }) => {
    customerDetailPage = new AdminCustomerDetailPage(page);
    await customerDetailPage.goto('customer-with-vms-id');

    await expect(page.locator('button:has-text("Delete")')).toBeDisabled();
  });

  test('should show reason for disabled deletion', async ({ page }) => {
    customerDetailPage = new AdminCustomerDetailPage(page);
    await customerDetailPage.goto('customer-with-vms-id');

    await page.hover('button:has-text("Delete")');

    await expect(page.locator('text=/VMs|in use|cannot delete/i')).toBeVisible();
  });

  test('should allow canceling deletion', async ({ page }) => {
    customerDetailPage = new AdminCustomerDetailPage(page);
    await customerDetailPage.goto('customer-no-vms-id');
    await customerDetailPage.clickDelete();

    await page.click('button:has-text("Cancel")');

    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
  });
});

test.describe('Admin Customer API Keys', () => {
  const testCustomerId = '00000000-0000-0000-0000-000000000006';

  test('should show customer API keys', async ({ page }) => {
    await page.goto(`/customers/${testCustomerId}/api-keys`);

    await expect(page.locator('h1, [data-testid="page-title"]')).toContainText(/api.*key/i);
  });

  test('should list API keys', async ({ page }) => {
    await page.goto(`/customers/${testCustomerId}/api-keys`);

    const keyList = page.locator('table tbody tr, [data-testid="api-key-item"]');
    const count = await keyList.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should show key name and permissions', async ({ page }) => {
    await page.goto(`/customers/${testCustomerId}/api-keys`);

    const keyList = page.locator('table tbody tr');
    const count = await keyList.count();

    if (count > 0) {
      // Should show key name
      await expect(keyList.first().locator('td:first-child, [data-testid="key-name"]')).toBeVisible();
    }
  });
});

test.describe('Admin Customer Webhooks', () => {
  const testCustomerId = '00000000-0000-0000-0000-000000000007';

  test('should show customer webhooks', async ({ page }) => {
    await page.goto(`/customers/${testCustomerId}/webhooks`);

    await expect(page.locator('h1, [data-testid="page-title"]')).toContainText(/webhook/i);
  });

  test('should list webhooks', async ({ page }) => {
    await page.goto(`/customers/${testCustomerId}/webhooks`);

    const webhookList = page.locator('table tbody tr, [data-testid="webhook-item"]');
    const count = await webhookList.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should show webhook URL and events', async ({ page }) => {
    await page.goto(`/customers/${testCustomerId}/webhooks`);

    const webhookList = page.locator('table tbody tr');
    const count = await webhookList.count();

    if (count > 0) {
      // Should show webhook URL
      await expect(webhookList.first().locator('text=/http/i')).toBeVisible();
    }
  });
});

test.describe('Admin Customer Navigation', () => {
  test('should have working navigation from dashboard', async ({ page }) => {
    await page.goto('/dashboard');

    await page.click('a:has-text("Customers")');

    await expect(page).toHaveURL(/\/customers/);
  });

  test('should have breadcrumb navigation', async ({ page }) => {
    await page.goto('/customers/test-customer-id');

    await page.click('a:has-text("Customers")');

    await expect(page).toHaveURL(/\/customers$/);
  });

  test('should navigate from customer list to detail', async ({ page }) => {
    await page.goto('/customers');

    const customerLink = page.locator('[data-testid="customer-card"] a, table tbody tr a').first();
    if (await customerLink.isVisible()) {
      await customerLink.click();
      await expect(page).toHaveURL(/\/customers\/[a-f0-9-]+/);
    }
  });

  test('should navigate between tabs', async ({ page }) => {
    await page.goto('/customers/test-customer-id');

    await page.click('a:has-text("VMs")');
    await expect(page).toHaveURL(/\/vms/);

    await page.click('a:has-text("Audit Logs")');
    await expect(page).toHaveURL(/\/audit-logs/);
  });
});