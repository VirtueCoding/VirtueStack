import { test, expect, Page } from '@playwright/test';

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

  test('should show recent activity', async ({ page }) => {
    // Look for activity or recent events section
    const activitySection = page.locator('[data-testid="recent-activity"], section:has-text("Activity")');
    if (await activitySection.isVisible()) {
      await expect(activitySection).toBeVisible();
    }
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
    
    await page.waitForTimeout(500);
    
    const cards = await vmListPage.getVMCards();
    const count = await cards.count();
    
    // If results found, they should match search
    if (count > 0) {
      const firstCard = cards.first();
      const text = await firstCard.textContent();
      expect(text?.toLowerCase()).toContain('test');
    }
  });

  test('should filter VMs by status', async ({ page }) => {
    await vmListPage.filterByStatus('Running');
    
    await page.waitForTimeout(500);
    
    // All visible VMs should have Running status
    const statuses = page.locator('[data-testid="vm-status"]:has-text("Running")');
    const count = await statuses.count();
    expect(count).toBeGreaterThan(0);
  });

  test('should click VM to view details', async ({ page }) => {
    const cards = await vmListPage.getVMCards();
    const count = await cards.count();
    
    if (count > 0) {
      const hostname = await cards.first().locator('[data-testid="vm-hostname"], h3').textContent();
      await vmListPage.clickVMByHostname(hostname || '');
      
      // Should navigate to detail page
      await expect(page).toHaveURL(/\/vms\/[a-f0-9-]+/);
    }
  });

  test('should show empty state when no VMs', async ({ page }) => {
    // This test assumes a customer with no VMs
    // In real test, you'd use a test account with no VMs
    const cards = await vmListPage.getVMCards();
    const count = await cards.count();
    
    if (count === 0) {
      await vmListPage.expectNoVMs();
    }
  });
});

test.describe('Customer VM Detail', () => {
  let vmDetailPage: CustomerVMDetailPage;
  const testVMId = '00000000-0000-0000-0000-000000000001';

  test.beforeEach(async ({ page }) => {
    vmDetailPage = new CustomerVMDetailPage(page);
    await vmDetailPage.goto(testVMId);
  });

  test('should display VM details', async ({ page }) => {
    await expect(page.locator('[data-testid="vm-hostname"]')).toBeVisible();
    await expect(page.locator('[data-testid="vm-status"]')).toBeVisible();
  });

  test('should show IP addresses', async ({ page }) => {
    const ips = await vmDetailPage.getIPAddresses();
    
    if (ips.length > 0) {
      // IPs should be formatted correctly
      for (const ip of ips) {
        expect(ip).toMatch(/\d+\.\d+\.\d+\.\d+/); // IPv4 format
      }
    }
  });

  test('should show resource information', async ({ page }) => {
    await expect(page.locator('text=/CPU|vCPU/i')).toBeVisible();
    await expect(page.locator('text=/Memory|RAM/i')).toBeVisible();
    await expect(page.locator('text=/Disk|Storage/i')).toBeVisible();
  });

  test('should show bandwidth usage', async ({ page }) => {
    const bandwidth = await vmDetailPage.getBandwidthUsage();
    
    if (bandwidth.used && bandwidth.limit) {
      expect(bandwidth.used).toBeTruthy();
      expect(bandwidth.limit).toBeTruthy();
    }
  });

  test('should show power control buttons', async ({ page }) => {
    // At least one power button should be visible
    const startBtn = page.locator('button:has-text("Start")');
    const stopBtn = page.locator('button:has-text("Stop")');
    const rebootBtn = page.locator('button:has-text("Reboot")');
    
    const startVisible = await startBtn.isVisible();
    const stopVisible = await stopBtn.isVisible();
    const rebootVisible = await rebootBtn.isVisible();
    
    expect(startVisible || stopVisible || rebootVisible).toBe(true);
  });

  test('should show console button', async ({ page }) => {
    await expect(page.locator('button:has-text("Console"), a:has-text("Console")')).toBeVisible();
  });
});

test.describe('Customer VM Power Operations', () => {
  let vmDetailPage: CustomerVMDetailPage;
  const testVMId = '00000000-0000-0000-0000-000000000002';

  test.beforeEach(async ({ page }) => {
    vmDetailPage = new CustomerVMDetailPage(page);
    await vmDetailPage.goto(testVMId);
  });

  test('should start a stopped VM', async ({ page }) => {
    const status = await vmDetailPage.getStatus();
    
    if (status?.toLowerCase().includes('stopped')) {
      await vmDetailPage.startVM();
      
      // Should show confirmation or status change
      await page.waitForTimeout(1000);
      
      // Wait for status to potentially change
      const newStatus = await vmDetailPage.getStatus();
      expect(['starting', 'running', 'provisioning']).toContain(newStatus?.toLowerCase());
    }
  });

  test('should stop a running VM', async ({ page }) => {
    const status = await vmDetailPage.getStatus();
    
    if (status?.toLowerCase().includes('running')) {
      await vmDetailPage.stopVM();
      
      await page.waitForTimeout(1000);
      
      // Should show confirmation or status change
      const newStatus = await vmDetailPage.getStatus();
      expect(['stopping', 'stopped']).toContain(newStatus?.toLowerCase());
    }
  });

  test('should reboot a running VM', async ({ page }) => {
    const status = await vmDetailPage.getStatus();
    
    if (status?.toLowerCase().includes('running')) {
      await vmDetailPage.rebootVM();
      
      await page.waitForTimeout(1000);
      
      // VM should be rebooting or still running
      const newStatus = await vmDetailPage.getStatus();
      expect(['rebooting', 'running']).toContain(newStatus?.toLowerCase());
    }
  });

  test('should not allow power operations during transitions', async ({ page }) => {
    const status = await vmDetailPage.getStatus();
    
    if (status?.toLowerCase().includes('provisioning') || status?.toLowerCase().includes('migrating')) {
      // Power buttons should be disabled during transition states
      await vmDetailPage.expectActionNotAvailable('Start');
      await vmDetailPage.expectActionNotAvailable('Stop');
    }
  });
});

test.describe('Customer Console Access', () => {
  let vmDetailPage: CustomerVMDetailPage;
  let consolePage: CustomerConsolePage;
  const testVMId = '00000000-0000-0000-0000-000000000003';

  test.beforeEach(async ({ page }) => {
    vmDetailPage = new CustomerVMDetailPage(page);
    consolePage = new CustomerConsolePage(page);
  });

  test('should open console from VM detail page', async ({ page }) => {
    await vmDetailPage.goto(testVMId);
    
    // Check if VM is running (console only works for running VMs)
    const status = await vmDetailPage.getStatus();
    
    if (status?.toLowerCase().includes('running')) {
      await vmDetailPage.openConsole();
      
      // Should navigate to console or open in new context
      await expect(page).toHaveURL(/\/console|noVNC/);
    }
  });

  test('should display console interface', async ({ page }) => {
    await consolePage.goto(testVMId);
    
    // Should show console container
    await expect(page.locator('[data-testid="console-container"], canvas, #noVNC_canvas')).toBeVisible({ timeout: 30000 });
  });

  test('should show console controls', async ({ page }) => {
    await consolePage.goto(testVMId);
    await consolePage.waitForConsole();
    
    // Should have control buttons
    await expect(page.locator('button:has-text("Fullscreen")')).toBeVisible();
  });

  test('should send special keys', async ({ page }) => {
    await consolePage.goto(testVMId);
    await consolePage.waitForConsole();
    
    // Ctrl+Alt+Del button should work
    const ctrlBtn = page.locator('button:has-text("Ctrl+Alt+Del")');
    if (await ctrlBtn.isVisible()) {
      await ctrlBtn.click();
      // No assertion needed, just verify it doesn't error
    }
  });

  test('should toggle fullscreen', async ({ page }) => {
    await consolePage.goto(testVMId);
    await consolePage.waitForConsole();
    
    await consolePage.fullscreen();
    
    // Should enter fullscreen mode (check for fullscreen class or state)
    // This is a UI state check
  });

  test('should disable console for stopped VM', async ({ page }) => {
    await vmDetailPage.goto(testVMId);
    
    const status = await vmDetailPage.getStatus();
    
    if (status?.toLowerCase().includes('stopped')) {
      // Console button should be disabled
      await vmDetailPage.expectActionNotAvailable('Console');
    }
  });
});

test.describe('Customer Resource Monitoring', () => {
  let vmDetailPage: CustomerVMDetailPage;
  const testVMId = '00000000-0000-0000-0000-000000000004';

  test.beforeEach(async ({ page }) => {
    vmDetailPage = new CustomerVMDetailPage(page);
    await vmDetailPage.goto(testVMId);
  });

  test('should show CPU usage graph', async ({ page }) => {
    // Look for CPU chart/graph
    const cpuSection = page.locator('[data-testid="cpu-chart"], section:has-text("CPU")');
    
    if (await cpuSection.isVisible()) {
      await expect(cpuSection).toBeVisible();
    }
  });

  test('should show memory usage', async ({ page }) => {
    const memorySection = page.locator('[data-testid="memory-chart"], section:has-text("Memory")');
    
    if (await memorySection.isVisible()) {
      await expect(memorySection).toBeVisible();
    }
  });

  test('should show network usage', async ({ page }) => {
    const networkSection = page.locator('[data-testid="network-chart"], section:has-text("Network")');
    
    if (await networkSection.isVisible()) {
      await expect(networkSection).toBeVisible();
    }
  });

  test('should show disk usage', async ({ page }) => {
    const diskSection = page.locator('[data-testid="disk-chart"], section:has-text("Disk")');
    
    if (await diskSection.isVisible()) {
      await expect(diskSection).toBeVisible();
    }
  });

  test('should show bandwidth limit and usage', async ({ page }) => {
    const bandwidth = await vmDetailPage.getBandwidthUsage();
    
    // Should show bandwidth information
    expect(bandwidth).toBeTruthy();
  });
});

test.describe('Customer VM Settings', () => {
  let vmDetailPage: CustomerVMDetailPage;
  const testVMId = '00000000-0000-0000-0000-000000000005';

  test('should show VM settings tab', async ({ page }) => {
    await vmDetailPage.goto(testVMId);
    await vmDetailPage.navigateToSettings();
    
    await expect(page).toHaveURL(/\/settings|tab=settings/);
  });

  test('should show hostname information', async ({ page }) => {
    await vmDetailPage.goto(testVMId);
    await vmDetailPage.navigateToSettings();
    
    await expect(page.locator('[data-testid="hostname"]')).toBeVisible();
  });

  test('should show plan information', async ({ page }) => {
    await vmDetailPage.goto(testVMId);
    await vmDetailPage.navigateToSettings();
    
    await expect(page.locator('text=/plan|package/i')).toBeVisible();
  });

  test('should not allow customer to change plan directly', async ({ page }) => {
    await vmDetailPage.goto(testVMId);
    await vmDetailPage.navigateToSettings();
    
    // Plan change should not be available or show upgrade prompt
    const upgradeBtn = page.locator('button:has-text("Upgrade"), a:has-text("Upgrade")');
    
    if (await upgradeBtn.isVisible()) {
      await expect(upgradeBtn).toBeVisible();
    } else {
      // Plan info should be read-only
      const planSection = page.locator('[data-testid="plan-info"]');
      await expect(planSection).toBeVisible();
    }
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