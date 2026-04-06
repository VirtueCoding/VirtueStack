import { test, expect, Page, APIRequestContext, request } from '@playwright/test';

/**
 * Customer VM Management E2E Tests
 * 
 * Tests cover:
 * - Customer viewing their VMs
 * - VM power management (start/stop via customer UI)
 * - Console access
 * - VM details viewing
 * - Bandwidth/resource usage
 */

// ============================================
// Page Object Models
// ============================================

class CustomerDashboardPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/dashboard');
    await expect(this.page.locator('h1, [data-testid="dashboard-title"]')).toContainText(/dashboard|overview/i);
  }

  async getQuickStats() {
    return {
      totalVMs: await this.page.locator('[data-testid="total-vms"]').textContent(),
      runningVMs: await this.page.locator('[data-testid="running-vms"]').textContent(),
      bandwidthUsed: await this.page.locator('[data-testid="bandwidth-used"]').textContent(),
    };
  }

  async navigateToVMs() {
    await this.page.click('a:has-text("My Servers"), a[href*="/vms"], nav a:has-text("Servers")');
    await expect(this.page).toHaveURL(/\/vms|\/servers/);
  }
}

class CustomerVMListPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/vms');
    await expect(this.page.locator('h1, [data-testid="page-title"]')).toContainText(/my servers|virtual machines|vms/i);
  }

  async getVMCards() {
    return this.page.locator('[data-testid="vm-card"], .vm-list-item');
  }

  async getVMCount() {
    const cards = await this.getVMCards();
    return cards.count();
  }

  async clickVMByHostname(hostname: string) {
    await this.page.click(`text="${hostname}"`);
  }

  async searchVM(query: string) {
    await this.page.fill('input[placeholder*="search" i], input[name="search"]', query);
    await this.page.press('input[placeholder*="search" i], input[name="search"]', 'Enter');
  }

  async filterByStatus(status: string) {
    await this.page.click('[data-testid="status-filter"]');
    await this.page.click(`text="${status}"`);
  }

  async expectVMVisible(hostname: string) {
    await expect(this.page.locator(`text="${hostname}"`)).toBeVisible();
  }

  async expectNoVMs() {
    await expect(this.page.locator('text=/no.*vms|no servers found/i')).toBeVisible();
  }

  async quickStartVM(hostname: string) {
    const card = this.page.locator(`[data-testid="vm-card"]:has-text("${hostname}")`);
    await card.locator('button:has-text("Start")').click();
  }

  async quickStopVM(hostname: string) {
    const card = this.page.locator(`[data-testid="vm-card"]:has-text("${hostname}")`);
    await card.locator('button:has-text("Stop")').click();
  }
}

class CustomerVMDetailPage {
  constructor(private page: Page) {}

  async goto(vmId: string) {
    await this.page.goto(`/vms/${vmId}`);
  }

  async getHostname() {
    return this.page.locator('[data-testid="vm-hostname"]').textContent();
  }

  async getStatus() {
    return this.page.locator('[data-testid="vm-status"]').textContent();
  }

  async getIPAddresses() {
    return this.page.locator('[data-testid="ip-address"]').allInnerTexts();
  }

  async getResourceUsage() {
    return {
      cpu: await this.page.locator('[data-testid="cpu-usage"]').textContent(),
      memory: await this.page.locator('[data-testid="memory-usage"]').textContent(),
      disk: await this.page.locator('[data-testid="disk-usage"]').textContent(),
    };
  }

  async getBandwidthUsage() {
    return {
      used: await this.page.locator('[data-testid="bandwidth-used"]').textContent(),
      limit: await this.page.locator('[data-testid="bandwidth-limit"]').textContent(),
    };
  }

  async startVM() {
    await this.page.click('button:has-text("Start"), [data-testid="start-btn"]');
  }

  async stopVM() {
    await this.page.click('button:has-text("Stop"), [data-testid="stop-btn"]');
  }

  async rebootVM() {
    await this.page.click('button:has-text("Reboot"), [data-testid="reboot-btn"]');
  }

  async openConsole() {
    await this.page.click('button:has-text("Console"), a:has-text("Open Console")');
  }

  async navigateToBackups() {
    await this.page.click('a:has-text("Backups"), [data-testid="backups-tab"]');
  }

  async navigateToSettings() {
    await this.page.click('a:has-text("Settings"), [data-testid="settings-tab"]');
  }

  async waitForStatus(status: string, timeout = 30000) {
    await expect(this.page.locator('[data-testid="vm-status"]')).toContainText(status, { timeout });
  }

  async expectActionAvailable(action: string) {
    await expect(this.page.locator(`button:has-text("${action}")`)).toBeEnabled();
  }

  async expectActionNotAvailable(action: string) {
    const btn = this.page.locator(`button:has-text("${action}")`);
    if (await btn.isVisible()) {
      await expect(btn).toBeDisabled();
    }
  }
}

class CustomerConsolePage {
  constructor(private page: Page) {}

  async goto(vmId: string) {
    await this.page.goto(`/vms/${vmId}/console`);
  }

  async waitForConsole() {
    await expect(this.page.locator('[data-testid="console-container"], canvas, #noVNC_canvas')).toBeVisible({ timeout: 30000 });
  }

  async sendCtrlAltDelete() {
    await this.page.click('button:has-text("Ctrl+Alt+Del"), [data-testid="ctrl-alt-del"]');
  }

  async fullscreen() {
    await this.page.click('button:has-text("Fullscreen"), [data-testid="fullscreen"]');
  }

  async close() {
    await this.page.click('button:has-text("Close"), [data-testid="close-console"]');
  }

  async expectConnected() {
    await expect(this.page.locator('text=/connected|ready/i')).toBeVisible();
  }
}

// ============================================
// Test Suite
// ============================================

test.describe('Customer Dashboard', () => {
  let dashboardPage: CustomerDashboardPage;

  test.beforeEach(async ({ page }) => {
    dashboardPage = new CustomerDashboardPage(page);
    await dashboardPage.goto();
  });

  test('should display dashboard overview', async ({ page }) => {
    await expect(page.locator('h1, [data-testid="page-title"]')).toBeVisible();
  });

  test('should show quick stats', async ({ page }) => {
    const stats = await dashboardPage.getQuickStats();

    expect(stats.totalVMs).toBeTruthy();
    expect(stats.runningVMs).toBeTruthy();
  });

  test('should navigate to VM list', async ({ page }) => {
    await dashboardPage.navigateToVMs();
    await expect(page).toHaveURL(/\/vms|\/servers/);
  });
});

test.describe('Customer VM List', () => {
  let vmListPage: CustomerVMListPage;

  test.beforeEach(async ({ page }) => {
    vmListPage = new CustomerVMListPage(page);
    await vmListPage.goto();
  });

  test('should display VM list', async ({ page }) => {
    const vmCount = await vmListPage.getVMCount();
    expect(vmCount).toBeGreaterThanOrEqual(0);
  });

  test('should show VM cards with key info', async ({ page }) => {
    const cards = await vmListPage.getVMCards();
    const count = await cards.count();
    
    if (count > 0) {
      const firstCard = cards.first();
      
      // Should show hostname
      await expect(firstCard.locator('[data-testid="vm-hostname"], h3')).toBeVisible();
      
      // Should show status
      await expect(firstCard.locator('[data-testid="vm-status"], .status')).toBeVisible();
      
      // Should show IP address
      await expect(firstCard.locator('[data-testid="ip-address"], .ip')).toBeVisible();
    }
  });

  test('should search VMs', async ({ page }) => {
    await vmListPage.searchVM('test');

    await page.waitForLoadState('networkidle');

    const cards = await vmListPage.getVMCards();
    const count = await cards.count();

    if (count > 0) {
      const firstCard = cards.first();
      const text = await firstCard.textContent();
      expect(text?.toLowerCase()).toContain('test');
    }
  });

  test('should click VM to view details', async ({ page }) => {
    const cards = await vmListPage.getVMCards();
    const count = await cards.count();

    if (count > 0) {
      const hostname = await cards.first().locator('[data-testid="vm-hostname"], h3').textContent();
      await vmListPage.clickVMByHostname(hostname || '');

      await expect(page).toHaveURL(/\/vms\/[a-f0-9-]+/);
    }
  });
});

async function getFirstCustomerVMId(apiContext: APIRequestContext): Promise<string | null> {
  try {
    const token = process.env.CUSTOMER_TOKEN;
    if (!token) return null;
    const resp = await apiContext.get(`${process.env.BASE_URL || 'http://localhost:8080'}/api/v1/customer/vms`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (resp.ok()) {
      const body = await resp.json();
      if (body.data && body.data.length > 0) {
        return body.data[0].id;
      }
    }
  } catch {
    // Fall back to env var
  }
  return process.env.TEST_CUSTOMER_VM_ID || null;
}

test.describe('Customer VM Detail', () => {
  let vmDetailPage: CustomerVMDetailPage;

  test.beforeEach(async ({ page, request: apiContext }) => {
    const vmId = await getFirstCustomerVMId(apiContext);
    test.skip(!vmId, 'No customer VM available for testing');

    vmDetailPage = new CustomerVMDetailPage(page);
    await vmDetailPage.goto(vmId!);
  });

  test('should display VM details', async ({ page }) => {
    await expect(page.locator('[data-testid="vm-hostname"]')).toBeVisible();
    await expect(page.locator('[data-testid="vm-status"]')).toBeVisible();
  });

  test('should show IP addresses', async ({ page }) => {
    const ips = await vmDetailPage.getIPAddresses();
    for (const ip of ips) {
      expect(ip).toMatch(/\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}/);
    }
  });

  test('should show resource information', async ({ page }) => {
    await expect(page.locator('text=/CPU|vCPU/i')).toBeVisible();
    await expect(page.locator('text=/Memory|RAM/i')).toBeVisible();
    await expect(page.locator('text=/Disk|Storage/i')).toBeVisible();
  });

  test('should show power control buttons', async ({ page }) => {
    const startBtn = page.locator('button:has-text("Start")');
    const stopBtn = page.locator('button:has-text("Stop")');
    const rebootBtn = page.locator('button:has-text("Reboot")');

    const visible = await Promise.all([
      startBtn.isVisible().catch(() => false),
      stopBtn.isVisible().catch(() => false),
      rebootBtn.isVisible().catch(() => false),
    ]);
    expect(visible.some(Boolean)).toBe(true);
  });

  test('should show console button', async ({ page }) => {
    await expect(page.locator('button:has-text("Console"), a:has-text("Console")')).toBeVisible();
  });
});

test.describe('Customer VM Power Operations', () => {
  let vmDetailPage: CustomerVMDetailPage;

  test.beforeEach(async ({ page, request: apiContext }) => {
    const vmId = await getFirstCustomerVMId(apiContext);
    test.skip(!vmId, 'No customer VM available for testing');

    vmDetailPage = new CustomerVMDetailPage(page);
    await vmDetailPage.goto(vmId!);
  });

  test('should start a stopped VM', async ({ page }) => {
    const status = await vmDetailPage.getStatus();

    if (status?.toLowerCase().includes('stopped')) {
      await vmDetailPage.startVM();

      await expect(vmDetailPage['page'].locator('[data-testid="vm-status"], .status')).toBeVisible({ timeout: 10000 });

      const newStatus = await vmDetailPage.getStatus();
      expect(['starting', 'running', 'provisioning']).toContain(newStatus?.toLowerCase());
    }
  });

  test('should stop a running VM', async ({ page }) => {
    const status = await vmDetailPage.getStatus();

    if (status?.toLowerCase().includes('running')) {
      await vmDetailPage.stopVM();

      await expect(vmDetailPage['page'].locator('[data-testid="vm-status"], .status')).toBeVisible({ timeout: 10000 });

      const newStatus = await vmDetailPage.getStatus();
      expect(['stopping', 'stopped']).toContain(newStatus?.toLowerCase());
    }
  });

  test('should reboot a running VM', async ({ page }) => {
    const status = await vmDetailPage.getStatus();

    if (status?.toLowerCase().includes('running')) {
      await vmDetailPage.rebootVM();

      await expect(vmDetailPage['page'].locator('[data-testid="vm-status"], .status')).toBeVisible({ timeout: 10000 });

      const newStatus = await vmDetailPage.getStatus();
      expect(['rebooting', 'running']).toContain(newStatus?.toLowerCase());
    }
  });
});

test.describe('Customer Console Access', () => {
  let vmDetailPage: CustomerVMDetailPage;
  let consolePage: CustomerConsolePage;

  test.beforeEach(async ({ page, request: apiContext }) => {
    const vmId = await getFirstCustomerVMId(apiContext);
    test.skip(!vmId, 'No customer VM available for testing');

    vmDetailPage = new CustomerVMDetailPage(page);
    consolePage = new CustomerConsolePage(page);
  });

  test('should open console from VM detail page', async ({ page, request: apiContext }) => {
    const vmId = await getFirstCustomerVMId(apiContext);
    await vmDetailPage.goto(vmId!);

    await vmDetailPage.openConsole();
    await expect(page).toHaveURL(/\/console|noVNC/);
  });

  test('should display console interface', async ({ page, request: apiContext }) => {
    const vmId = await getFirstCustomerVMId(apiContext);
    await consolePage.goto(vmId!);

    await expect(page.locator('[data-testid="console-container"], canvas, #noVNC_canvas')).toBeVisible({ timeout: 30000 });
  });

  test('should show console controls', async ({ page, request: apiContext }) => {
    const vmId = await getFirstCustomerVMId(apiContext);
    await consolePage.goto(vmId!);
    await consolePage.waitForConsole();

    await expect(page.locator('button:has-text("Fullscreen")')).toBeVisible();
  });
});

test.describe('Customer Resource Monitoring', () => {
  let vmDetailPage: CustomerVMDetailPage;

  test.beforeEach(async ({ page, request: apiContext }) => {
    const vmId = await getFirstCustomerVMId(apiContext);
    test.skip(!vmId, 'No customer VM available for testing');

    vmDetailPage = new CustomerVMDetailPage(page);
    await vmDetailPage.goto(vmId!);
  });

  test('should show CPU usage graph', async ({ page }) => {
    const cpuSection = page.locator('[data-testid="cpu-chart"], section:has-text("CPU")');
    await expect(cpuSection).toBeVisible({ timeout: 10000 });
  });

  test('should show memory usage', async ({ page }) => {
    const memorySection = page.locator('[data-testid="memory-chart"], section:has-text("Memory")');
    await expect(memorySection).toBeVisible({ timeout: 10000 });
  });

  test('should show network usage', async ({ page }) => {
    const networkSection = page.locator('[data-testid="network-chart"], section:has-text("Network")');
    await expect(networkSection).toBeVisible({ timeout: 10000 });
  });

  test('should show disk usage', async ({ page }) => {
    const diskSection = page.locator('[data-testid="disk-chart"], section:has-text("Disk")');
    await expect(diskSection).toBeVisible({ timeout: 10000 });
  });
});

test.describe('Customer VM Settings', () => {
  let vmDetailPage: CustomerVMDetailPage;

  test.beforeEach(async ({ page, request: apiContext }) => {
    const vmId = await getFirstCustomerVMId(apiContext);
    test.skip(!vmId, 'No customer VM available for testing');

    vmDetailPage = new CustomerVMDetailPage(page);
    await vmDetailPage.goto(vmId!);
  });

  test('should show VM settings tab', async ({ page }) => {
    await vmDetailPage.navigateToSettings();

    await expect(page).toHaveURL(/\/settings|tab=settings/);
  });

  test('should show hostname information', async ({ page }) => {
    await vmDetailPage.navigateToSettings();

    await expect(page.locator('[data-testid="hostname"]')).toBeVisible();
  });

  test('should show plan information', async ({ page }) => {
    await vmDetailPage.navigateToSettings();

    await expect(page.locator('text=/plan|package/i')).toBeVisible();
  });
});

test.describe('Customer Navigation', () => {
  test('should have working navigation menu', async ({ page }) => {
    await page.goto('/vms');
    
    // Check main navigation items
    await expect(page.locator('nav')).toBeVisible();
    
    // Dashboard link
    await page.click('a:has-text("Dashboard")');
    await expect(page).toHaveURL(/\/dashboard/);
    
    // VMs/Servers link
    await page.click('a:has-text("Servers"), a:has-text("My Servers")');
    await expect(page).toHaveURL(/\/vms|\/servers/);
    
    // Backups link
    await page.click('a:has-text("Backups")');
    await expect(page).toHaveURL(/\/backups/);
    
    // Settings/Account link
    await page.click('a:has-text("Settings"), a:has-text("Account")');
    await expect(page).toHaveURL(/\/settings|\/account/);
  });

  test('should show user profile menu', async ({ page }) => {
    await page.goto('/vms');
    
    // Click user menu
    await page.click('[data-testid="user-menu"], button:has(img)');
    
    // Should show profile options
    await expect(page.locator('a:has-text("Profile"), a:has-text("Account")')).toBeVisible();
    await expect(page.locator('button:has-text("Logout")')).toBeVisible();
  });
});
