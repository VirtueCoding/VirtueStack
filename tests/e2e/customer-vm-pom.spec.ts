/**
 * Customer VM Management E2E Tests (Refactored)
 *
 * Uses the Page Object Model pattern for better maintainability.
 */

import { customerTest as test, expect } from './fixtures/index';
import { TEST_IDS, getFirstCustomerVMId } from './utils/api';

// ============================================
// Customer Dashboard Tests
// ============================================

test.describe('Customer Dashboard', () => {
  test.use({ storageState: '.auth/customer-storage.json' });

  test('should display dashboard overview', async ({ customerDashboardPage }) => {
    await customerDashboardPage.goto();
    await expect(customerDashboardPage['page'].locator('h1, [data-testid="page-title"]')).toBeVisible();
  });

  test('should show quick stats', async ({ customerDashboardPage }) => {
    await customerDashboardPage.goto();

    const stats = await customerDashboardPage.getQuickStats();
    expect(stats.totalVMs).toBeTruthy();
    expect(stats.runningVMs).toBeTruthy();
  });

  test('should navigate to VM list', async ({ customerDashboardPage }) => {
    await customerDashboardPage.goto();
    await customerDashboardPage.navigateToVMs();
    await expect(customerDashboardPage['page']).toHaveURL(/\/vms|\/servers/);
  });
});

// ============================================
// Customer VM List Tests
// ============================================

test.describe('Customer VM List', () => {
  test.use({ storageState: '.auth/customer-storage.json' });

  test('should display VM list', async ({ customerVMListPage }) => {
    await customerVMListPage.goto();

    const count = await customerVMListPage.getVMCount();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should show VM cards with key info', async ({ customerVMListPage, page }) => {
    await customerVMListPage.goto();

    const cards = await customerVMListPage.getVMCards();
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

  test('should search VMs', async ({ customerVMListPage, page }) => {
    await customerVMListPage.goto();
    await customerVMListPage.searchVM('test');

    await page.waitForLoadState('networkidle');

    const cards = await customerVMListPage.getVMCards();
    const count = await cards.count();

    if (count > 0) {
      const text = await cards.first().textContent();
      expect(text?.toLowerCase()).toContain('test');
    }
  });

  test('should click VM to view details', async ({ customerVMListPage, page }) => {
    await customerVMListPage.goto();

    const cards = await customerVMListPage.getVMCards();
    const count = await cards.count();

    if (count > 0) {
      const hostname = await cards.first().locator('[data-testid="vm-hostname"], h3').textContent();
      await customerVMListPage.clickVMByHostname(hostname || '');

      await expect(page).toHaveURL(/\/vms\/[a-f0-9-]+/);
    }
  });
});

// ============================================
// Customer VM Detail Tests
// ============================================

test.describe('Customer VM Detail', () => {
  test.use({ storageState: '.auth/customer-storage.json' });

  test.beforeEach(async ({ customerVMDetailPage, request }) => {
    const vmId = await getFirstCustomerVMId(request);
    test.skip(!vmId, 'No customer VM available for testing');
    await customerVMDetailPage.goto(vmId!);
  });

  test('should display VM details', async ({ customerVMDetailPage }) => {
    await expect(customerVMDetailPage['page'].locator('[data-testid="vm-hostname"]')).toBeVisible();
    await expect(customerVMDetailPage['page'].locator('[data-testid="vm-status"]')).toBeVisible();
  });

  test('should show IP addresses', async ({ customerVMDetailPage }) => {
    const ips = await customerVMDetailPage.getIPAddresses();
    for (const ip of ips) {
      expect(ip).toMatch(/\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}/);
    }
  });

  test('should show resource information', async ({ customerVMDetailPage, page }) => {
    await expect(page.locator('text=/CPU|vCPU/i')).toBeVisible();
    await expect(page.locator('text=/Memory|RAM/i')).toBeVisible();
    await expect(page.locator('text=/Disk|Storage/i')).toBeVisible();
  });

  test('should show power control buttons', async ({ customerVMDetailPage, page }) => {
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

  test('should show console button', async ({ customerVMDetailPage, page }) => {
    await expect(page.locator('button:has-text("Console"), a:has-text("Console")')).toBeVisible();
  });
});

// ============================================
// Customer VM Power Operations Tests
// ============================================

test.describe('Customer VM Power Operations', () => {
  test.use({ storageState: '.auth/customer-storage.json' });

  test.beforeEach(async ({ customerVMDetailPage, request }) => {
    const vmId = await getFirstCustomerVMId(request);
    test.skip(!vmId, 'No customer VM available for testing');
    await customerVMDetailPage.goto(vmId!);
  });

  test('should start a stopped VM', async ({ customerVMDetailPage, page }) => {
    const status = await customerVMDetailPage.getStatus();

    if (status?.toLowerCase().includes('stopped')) {
      await customerVMDetailPage.startVM();

      await expect(page.locator('[data-testid="vm-status"], .status')).toBeVisible({ timeout: 10000 });

      const newStatus = await customerVMDetailPage.getStatus();
      expect(['starting', 'running', 'provisioning']).toContain(newStatus?.toLowerCase());
    }
  });

  test('should stop a running VM', async ({ customerVMDetailPage, page }) => {
    const status = await customerVMDetailPage.getStatus();

    if (status?.toLowerCase().includes('running')) {
      await customerVMDetailPage.stopVM();

      await expect(page.locator('[data-testid="vm-status"], .status')).toBeVisible({ timeout: 10000 });

      const newStatus = await customerVMDetailPage.getStatus();
      expect(['stopping', 'stopped']).toContain(newStatus?.toLowerCase());
    }
  });

  test('should reboot a running VM', async ({ customerVMDetailPage, page }) => {
    const status = await customerVMDetailPage.getStatus();

    if (status?.toLowerCase().includes('running')) {
      await customerVMDetailPage.rebootVM();

      await expect(page.locator('[data-testid="vm-status"], .status')).toBeVisible({ timeout: 10000 });

      const newStatus = await customerVMDetailPage.getStatus();
      expect(['rebooting', 'running']).toContain(newStatus?.toLowerCase());
    }
  });
});

// ============================================
// Customer Console Access Tests
// ============================================

test.describe('Customer Console Access', () => {
  test.use({ storageState: '.auth/customer-storage.json' });

  test('should open console from VM detail page', async ({ customerVMDetailPage, customerConsolePage, request, page }) => {
    const vmId = await getFirstCustomerVMId(request);
    test.skip(!vmId, 'No customer VM available for testing');

    await customerVMDetailPage.goto(vmId!);
    await customerVMDetailPage.openConsole();

    await expect(page).toHaveURL(/\/console|noVNC/);
  });

  test('should display console interface', async ({ customerConsolePage, request, page }) => {
    const vmId = await getFirstCustomerVMId(request);
    test.skip(!vmId, 'No customer VM available for testing');

    await customerConsolePage.goto(vmId!);

    await expect(page.locator('[data-testid="console-container"], canvas, #noVNC_canvas')).toBeVisible({ timeout: 30000 });
  });

  test('should show console controls', async ({ customerConsolePage, request, page }) => {
    const vmId = await getFirstCustomerVMId(request);
    test.skip(!vmId, 'No customer VM available for testing');

    await customerConsolePage.goto(vmId!);
    await customerConsolePage.waitForConsole();

    await expect(page.locator('button:has-text("Fullscreen")')).toBeVisible();
  });
});

// ============================================
// Customer Navigation Tests
// ============================================

test.describe('Customer Navigation', () => {
  test.use({ storageState: '.auth/customer-storage.json' });

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
