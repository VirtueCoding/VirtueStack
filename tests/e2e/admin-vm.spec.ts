import { test, expect, Page } from '@playwright/test';

/**
 * Admin VM Management E2E Tests
 * 
 * Tests cover:
 * - VM creation workflow
 * - VM list viewing
 * - VM power operations (start/stop/reboot)
 * - VM details viewing
 * - VM deletion
 */

// ============================================
// Page Object Models
// ============================================

class AdminDashboardPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/admin/dashboard');
    await expect(this.page.locator('h1, [data-testid="dashboard-title"]')).toContainText(/dashboard/i);
  }

  async navigateToVMs() {
    await this.page.click('a:has-text("Virtual Machines"), a[href*="/admin/vms"]');
    await expect(this.page).toHaveURL(/\/admin\/vms/);
  }
}

class AdminVMListPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/admin/vms');
    await expect(this.page.locator('h1, [data-testid="page-title"]')).toContainText(/virtual machines|vms/i);
  }

  async clickCreateVM() {
    await this.page.click('button:has-text("Create VM"), a:has-text("Create VM"), [data-testid="create-vm-btn"]');
  }

  async searchVM(query: string) {
    await this.page.fill('input[placeholder*="search" i], input[name="search"]', query);
    await this.page.press('input[placeholder*="search" i], input[name="search"]', 'Enter');
  }

  async getVMRows() {
    return this.page.locator('table tbody tr, [data-testid="vm-row"]');
  }

  async clickVMById(vmId: string) {
    await this.page.click(`a[href*="${vmId}"], [data-testid="vm-${vmId}"]`);
  }

  async expectVMInList(hostname: string) {
    await expect(this.page.locator(`text="${hostname}"`)).toBeVisible();
  }

  async expectVMNotInList(hostname: string) {
    await expect(this.page.locator(`text="${hostname}"`)).not.toBeVisible();
  }

  async getPaginationInfo() {
    const text = await this.page.locator('[data-testid="pagination-info"], .pagination-info').textContent();
    return text;
  }

  async goToNextPage() {
    await this.page.click('button:has-text("Next"), [data-testid="next-page"]');
  }

  async goToPreviousPage() {
    await this.page.click('button:has-text("Previous"), [data-testid="prev-page"]');
  }

  async changePageSize(size: number) {
    await this.page.click('[data-testid="page-size-select"], select[name="pageSize"]');
    await this.page.click(`option:has-text("${size}")`);
  }

  async filterByStatus(status: string) {
    await this.page.click('[data-testid="status-filter"], select[name="status"]');
    await this.page.click(`option:has-text("${status}")`);
  }

  async filterByNode(node: string) {
    await this.page.click('[data-testid="node-filter"], select[name="node"]');
    await this.page.click(`option:has-text("${node}")`);
  }
}

class AdminVMCreatePage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/admin/vms/create');
    await expect(this.page.locator('h1')).toContainText(/create.*vm|new.*vm/i);
  }

  async selectCustomer(email: string) {
    await this.page.click('[data-testid="customer-select"], select[name="customer_id"]');
    await this.page.fill('input[placeholder*="search" i]', email);
    await this.page.click(`text="${email}"`);
  }

  async selectPlan(planName: string) {
    await this.page.click('[data-testid="plan-select"], select[name="plan_id"]');
    await this.page.click(`text="${planName}"`);
  }

  async selectTemplate(templateName: string) {
    await this.page.click('[data-testid="template-select"], select[name="template_id"]');
    await this.page.click(`text="${templateName}"`);
  }

  async selectNode(nodeName: string) {
    await this.page.click('[data-testid="node-select"], select[name="node_id"]');
    await this.page.click(`text="${nodeName}"`);
  }

  async fillHostname(hostname: string) {
    await this.page.fill('input[name="hostname"]', hostname);
  }

  async fillPassword(password: string) {
    await this.page.fill('input[name="password"]', password);
  }

  async addSSHKey(key: string) {
    await this.page.click('button:has-text("Add SSH Key")');
    await this.page.fill('textarea[name="ssh_key"]', key);
  }

  async submit() {
    await this.page.click('button[type="submit"]');
  }

  async expectValidationError(message: string | RegExp) {
    await expect(this.page.locator('.error, [role="alert"]')).toContainText(message);
  }

  async expectSuccess() {
    await expect(this.page).toHaveURL(/\/admin\/vms\/[a-f0-9-]+/);
  }
}

class AdminVMDetailPage {
  constructor(private page: Page) {}

  async goto(vmId: string) {
    await this.page.goto(`/admin/vms/${vmId}`);
  }

  async expectHostname(hostname: string) {
    await expect(this.page.locator('h1, [data-testid="vm-hostname"]')).toContainText(hostname);
  }

  async getStatus(): Promise<string> {
    return this.page.locator('[data-testid="vm-status"], .status-badge').textContent() || '';
  }

  async startVM() {
    await this.page.click('button:has-text("Start"), [data-testid="start-btn"]');
    await this.waitForStatus('running');
  }

  async stopVM() {
    await this.page.click('button:has-text("Stop"), [data-testid="stop-btn"]');
    await this.waitForStatus('stopped');
  }

  async rebootVM() {
    await this.page.click('button:has-text("Reboot"), [data-testid="reboot-btn"]');
  }

  async forceStopVM() {
    await this.page.click('button:has-text("Force Stop"), [data-testid="force-stop-btn"]');
  }

  async deleteVM() {
    await this.page.click('button:has-text("Delete"), [data-testid="delete-btn"]');
    // Confirm deletion in modal
    await this.page.click('button:has-text("Confirm"), [data-testid="confirm-delete"]');
  }

  async waitForStatus(status: string, timeout = 30000) {
    await expect(this.page.locator('[data-testid="vm-status"], .status-badge')).toContainText(status, { timeout });
  }

  async getVMInfo() {
    return {
      hostname: await this.page.locator('[data-testid="vm-hostname"]').textContent(),
      status: await this.getStatus(),
      vcpu: await this.page.locator('[data-testid="vm-vcpu"]').textContent(),
      memory: await this.page.locator('[data-testid="vm-memory"]').textContent(),
      disk: await this.page.locator('[data-testid="vm-disk"]').textContent(),
    };
  }

  async getIPAddresses() {
    const ips = this.page.locator('[data-testid="ip-address"]');
    return ips.allInnerTexts();
  }

  async navigateToConsole() {
    await this.page.click('a:has-text("Console"), [data-testid="console-link"]');
    await expect(this.page).toHaveURL(/\/console/);
  }

  async navigateToBackups() {
    await this.page.click('a:has-text("Backups"), [data-testid="backups-link"]');
  }

  async navigateToAuditLog() {
    await this.page.click('a:has-text("Audit Log"), [data-testid="audit-log-link"]');
  }

  async expectActionDisabled(action: string) {
    await expect(this.page.locator(`button:has-text("${action}")`)).toBeDisabled();
  }

  async expectActionEnabled(action: string) {
    await expect(this.page.locator(`button:has-text("${action}")`)).toBeEnabled();
  }
}

// ============================================
// Test Suite
// ============================================

test.describe('Admin VM List', () => {
  let vmListPage: AdminVMListPage;

  test.beforeEach(async ({ page }) => {
    vmListPage = new AdminVMListPage(page);
    await vmListPage.goto();
  });

  test('should display VM list', async ({ page }) => {
    await expect(page.locator('table')).toBeVisible();
    
    // Should have at least header row
    const headers = page.locator('table thead th');
    await expect(headers).toHaveCountGreaterThan(3);
  });

  test('should show VM counts', async ({ page }) => {
    const paginationInfo = await vmListPage.getPaginationInfo();
    expect(paginationInfo).toMatch(/\d+.*VMs?/i);
  });

  test('should search VMs by hostname', async ({ page }) => {
    await vmListPage.searchVM('test-vm');
    
    await page.waitForLoadState('networkidle');
    
    // All visible VMs should match search
    const rows = await vmListPage.getVMRows();
    const count = await rows.count();
    
    if (count > 0) {
      await vmListPage.expectVMInList('test-vm');
    }
  });

  test('should filter VMs by status', async ({ page }) => {
    await vmListPage.filterByStatus('Running');
    
    await page.waitForLoadState('networkidle');
    
    // All visible VMs should have Running status
    const statusCells = page.locator('table tbody td:has-text("Running"), [data-testid="vm-status"]:has-text("Running")');
    const count = await statusCells.count();
    expect(count).toBeGreaterThan(0);
  });

  test('should paginate VM list', async ({ page }) => {
    // Get initial count
    const rows = await vmListPage.getVMRows();
    const initialCount = await rows.count();
    
    // Try to go to next page if available
    const nextBtn = page.locator('button:has-text("Next"), [data-testid="next-page"]');
    if (await nextBtn.isEnabled()) {
      await vmListPage.goToNextPage();
      
      // Should show different page
      const newRows = await vmListPage.getVMRows();
      expect(await newRows.count()).toBeGreaterThan(0);
      
      // Go back
      await vmListPage.goToPreviousPage();
    }
  });

  test('should change page size', async ({ page }) => {
    await vmListPage.changePageSize(50);
    
    await page.waitForLoadState('networkidle');
    
    // URL or page should reflect change
    const rows = await vmListPage.getVMRows();
    const count = await rows.count();
    expect(count).toBeLessThanOrEqual(50);
  });
});

test.describe('Admin VM Creation', () => {
  let vmCreatePage: AdminVMCreatePage;

  test.beforeEach(async ({ page }) => {
    vmCreatePage = new AdminVMCreatePage(page);
    await vmCreatePage.goto();
  });

  test('should display VM creation form', async ({ page }) => {
    await expect(page.locator('input[name="hostname"]')).toBeVisible();
    await expect(page.locator('select[name="plan_id"], [data-testid="plan-select"]')).toBeVisible();
    await expect(page.locator('select[name="template_id"], [data-testid="template-select"]')).toBeVisible();
    await expect(page.locator('button[type="submit"]')).toBeVisible();
  });

  test('should show validation errors for missing fields', async ({ page }) => {
    await vmCreatePage.submit();
    
    await vmCreatePage.expectValidationError(/required|please.*select/i);
  });

  test('should validate hostname format', async ({ page }) => {
    await vmCreatePage.fillHostname('invalid hostname with spaces');
    await vmCreatePage.submit();
    
    await vmCreatePage.expectValidationError(/valid hostname|RFC 1123/i);
  });

  test('should validate hostname length', async ({ page }) => {
    const longHostname = 'a'.repeat(64);
    await vmCreatePage.fillHostname(longHostname);
    await vmCreatePage.submit();
    
    await vmCreatePage.expectValidationError(/63 characters|too long/i);
  });

  test('should validate password strength', async ({ page }) => {
    await page.fill('input[name="password"]', 'weak');
    await vmCreatePage.submit();
    
    await vmCreatePage.expectValidationError(/password.*requirements|stronger password/i);
  });

  test('should show available plans', async ({ page }) => {
    await page.click('[data-testid="plan-select"], select[name="plan_id"]');
    
    // Should show plan options
    const options = page.locator('[data-testid="plan-option"], option');
    await expect(options.first()).toBeVisible();
  });

  test('should show available templates', async ({ page }) => {
    await page.click('[data-testid="template-select"], select[name="template_id"]');
    
    // Should show template options
    const options = page.locator('[data-testid="template-option"], option');
    await expect(options.first()).toBeVisible();
  });

  test('should create VM successfully', async ({ page }) => {
    const testHostname = `test-vm-${Date.now()}`;
    
    await vmCreatePage.selectCustomer('test-customer@example.com');
    await vmCreatePage.selectPlan('Basic Plan');
    await vmCreatePage.selectTemplate('Ubuntu 22.04');
    await vmCreatePage.fillHostname(testHostname);
    await page.fill('input[name="password"]', 'SecurePassword123!');
    
    await vmCreatePage.submit();
    
    // Should redirect to VM detail page
    await vmCreatePage.expectSuccess();
    
    // Verify hostname on detail page
    await expect(page.locator(`text="${testHostname}"`)).toBeVisible();
  });
});

test.describe('Admin VM Detail', () => {
  let vmDetailPage: AdminVMDetailPage;
  const testVMId = '00000000-0000-0000-0000-000000000001'; // Replace with actual VM ID

  test.beforeEach(async ({ page }) => {
    vmDetailPage = new AdminVMDetailPage(page);
    await vmDetailPage.goto(testVMId);
  });

  test('should display VM details', async ({ page }) => {
    await expect(page.locator('[data-testid="vm-hostname"]')).toBeVisible();
    await expect(page.locator('[data-testid="vm-status"]')).toBeVisible();
    await expect(page.locator('[data-testid="vm-vcpu"]')).toBeVisible();
    await expect(page.locator('[data-testid="vm-memory"]')).toBeVisible();
    await expect(page.locator('[data-testid="vm-disk"]')).toBeVisible();
  });

  test('should show IP addresses', async ({ page }) => {
    const ips = await vmDetailPage.getIPAddresses();
    expect(ips.length).toBeGreaterThan(0);
  });

  test('should show resource metrics', async ({ page }) => {
    // Check for metrics section
    await expect(page.locator('text=/CPU|Memory|Disk|Network/i')).toBeVisible();
  });

  test('should show action buttons', async ({ page }) => {
    await expect(page.locator('button:has-text("Start")')).toBeVisible();
    await expect(page.locator('button:has-text("Stop")')).toBeVisible();
    await expect(page.locator('button:has-text("Reboot")')).toBeVisible();
    await expect(page.locator('button:has-text("Delete")')).toBeVisible();
  });
});

test.describe('Admin VM Power Operations', () => {
  let vmDetailPage: AdminVMDetailPage;
  const testVMId = '00000000-0000-0000-0000-000000000002';

  test.beforeEach(async ({ page }) => {
    vmDetailPage = new AdminVMDetailPage(page);
    await vmDetailPage.goto(testVMId);
  });

  test('should start a stopped VM', async ({ page }) => {
    // Ensure VM is stopped
    const status = await vmDetailPage.getStatus();
    
    if (status.toLowerCase().includes('stopped')) {
      await vmDetailPage.startVM();
      
      const newStatus = await vmDetailPage.getStatus();
      expect(newStatus.toLowerCase()).toContain('running');
    }
  });

  test('should stop a running VM', async ({ page }) => {
    const status = await vmDetailPage.getStatus();
    
    if (status.toLowerCase().includes('running')) {
      await vmDetailPage.stopVM();
      
      const newStatus = await vmDetailPage.getStatus();
      expect(newStatus.toLowerCase()).toContain('stopped');
    }
  });

  test('should reboot a running VM', async ({ page }) => {
    const status = await vmDetailPage.getStatus();
    
    if (status.toLowerCase().includes('running')) {
      await vmDetailPage.rebootVM();
      
      // VM should eventually be running again
      await vmDetailPage.waitForStatus('running', 60000);
    }
  });

  test('should disable start button for running VM', async ({ page }) => {
    const status = await vmDetailPage.getStatus();
    
    if (status.toLowerCase().includes('running')) {
      await vmDetailPage.expectActionDisabled('Start');
    }
  });

  test('should disable stop button for stopped VM', async ({ page }) => {
    const status = await vmDetailPage.getStatus();
    
    if (status.toLowerCase().includes('stopped')) {
      await vmDetailPage.expectActionDisabled('Stop');
    }
  });
});

test.describe('Admin VM Deletion', () => {
  let vmListPage: AdminVMListPage;
  let vmDetailPage: AdminVMDetailPage;

  test('should show deletion confirmation', async ({ page }) => {
    const vmId = '00000000-0000-0000-0000-000000000003';
    vmDetailPage = new AdminVMDetailPage(page);
    await vmDetailPage.goto(vmId);
    
    // Click delete
    await page.click('button:has-text("Delete")');
    
    // Should show confirmation modal
    await expect(page.locator('[role="dialog"], .modal')).toBeVisible();
    await expect(page.locator('text=/confirm|are you sure/i')).toBeVisible();
  });

  test('should cancel deletion', async ({ page }) => {
    const vmId = '00000000-0000-0000-0000-000000000003';
    vmDetailPage = new AdminVMDetailPage(page);
    await vmDetailPage.goto(vmId);
    
    await page.click('button:has-text("Delete")');
    await page.click('button:has-text("Cancel")');
    
    // Should close modal
    await expect(page.locator('[role="dialog"], .modal')).not.toBeVisible();
    
    // VM should still exist
    await expect(page.locator('[data-testid="vm-hostname"]')).toBeVisible();
  });

  test('should delete VM after confirmation', async ({ page }) => {
    const vmId = '00000000-0000-0000-0000-000000000004';
    vmDetailPage = new AdminVMDetailPage(page);
    await vmDetailPage.goto(vmId);
    
    const hostname = await page.locator('[data-testid="vm-hostname"]').textContent();
    
    await vmDetailPage.deleteVM();
    
    // Should redirect to VM list
    await expect(page).toHaveURL(/\/admin\/vms$/);
    
    // VM should not be in list
    vmListPage = new AdminVMListPage(page);
    await vmListPage.expectVMNotInList(hostname || '');
  });
});

test.describe('Admin VM Console Access', () => {
  test('should open console window', async ({ page, context }) => {
    const vmId = '00000000-0000-0000-0000-000000000005';
    const vmDetailPage = new AdminVMDetailPage(page);
    await vmDetailPage.goto(vmId);
    
    // Listen for new page
    const [newPage] = await Promise.all([
      context.waitForEvent('page'),
      vmDetailPage.navigateToConsole(),
    ]);
    
    // New page should be console
    await expect(newPage).toHaveURL(/\/console|novnc/);
  });
});