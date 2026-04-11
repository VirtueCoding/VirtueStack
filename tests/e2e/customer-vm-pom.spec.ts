/**
 * Customer VM Management E2E Tests (Refactored)
 *
 * Uses the Page Object Model pattern for better maintainability.
 */

import { customerTest as test, expect } from './fixtures';
import { TEST_IDS, getFirstCustomerVMId } from './utils/api';

// ============================================
// Customer Dashboard Tests
// ============================================

test.describe('Customer Dashboard', () => {
  test.use({ storageState: '.auth/customer-storage.json' });

  test('should display dashboard overview', async ({ customerDashboardPage }) => {
    await customerDashboardPage.goto();
    await expect(customerDashboardPage['page'].locator('body')).toContainText(/virtual machines|no virtual machines/i);
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
      await expect(firstCard).toContainText(/\S+/);
      await expect(firstCard).toContainText(/running|stopped|suspended|error|provisioning/i);
      await expect(firstCard).toContainText(/\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}/);
    }
  });

  test('should search VMs', async ({ customerVMListPage, page }) => {
    await customerVMListPage.goto();
    await customerVMListPage.searchVM('test');

    const cards = await customerVMListPage.getVMCards();
    const count = await cards.count();

    if (count > 0) {
      const text = await cards.first().textContent();
      expect(text?.toLowerCase()).toContain('test');
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
    await expect(customerVMDetailPage['page'].locator('h1')).toBeVisible();
    await expect(customerVMDetailPage['page'].locator('h1').locator('xpath=following-sibling::*[1]')).toBeVisible();
  });

  test('should show IP addresses', async ({ page }) => {
    await expect(page.locator('body')).toContainText(/IPv4 Address/i);
    await expect(page.locator('body')).toContainText(/IPv6 Address/i);
    await expect(page.locator('body')).toContainText(/\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}|Not assigned/i);
  });

  test('should show resource information', async ({ customerVMDetailPage, page }) => {
    await expect(page.locator('body')).toContainText(/CPU|vCPU/i);
    await expect(page.locator('body')).toContainText(/Memory|RAM/i);
    await expect(page.locator('body')).toContainText(/Disk|Storage/i);
  });

  test('should show power control buttons', async ({ customerVMDetailPage, page }) => {
    await expect(page.locator('body')).toContainText(/VM Controls/i);
    await expect(page.locator('body')).toContainText(/Start|Stop|Restart|Force Stop|Provisioning/i);
  });

  test('should show console button', async ({ customerVMDetailPage, page }) => {
    await expect(page.locator('[role="tab"]:has-text("VNC")')).toBeVisible();
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

      await expect(page.locator('h1').locator('xpath=following-sibling::*[1]')).toBeVisible({ timeout: 10000 });

      const newStatus = await customerVMDetailPage.getStatus();
      expect(['starting', 'running', 'provisioning']).toContain(newStatus?.toLowerCase());
    }
  });

  test('should stop a running VM', async ({ customerVMDetailPage, page }) => {
    const status = await customerVMDetailPage.getStatus();

    if (status?.toLowerCase().includes('running')) {
      await customerVMDetailPage.stopVM();

      await page.locator('button:has-text("Stop VM")').click();
      await expect(page.locator('h1').locator('xpath=following-sibling::*[1]')).toBeVisible({ timeout: 10000 });

      const newStatus = await customerVMDetailPage.getStatus();
      expect(['stopping', 'stopped']).toContain(newStatus?.toLowerCase());
    }
  });

  test('should reboot a running VM', async ({ customerVMDetailPage, page }) => {
    const status = await customerVMDetailPage.getStatus();

    if (status?.toLowerCase().includes('running')) {
      await customerVMDetailPage.rebootVM();

      await expect(page.locator('h1').locator('xpath=following-sibling::*[1]')).toBeVisible({ timeout: 10000 });

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

    await expect(page.locator('body')).toContainText(/VM Console|Console Access|Console Unavailable/i);
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

    await expect(page.locator('[role="tab"]:has-text("VNC")')).toBeVisible();
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

    await page.click('a:has-text("My VMs")');
    await expect(page).toHaveURL(/\/vms/);

    await page.click('a:has-text("Billing")');
    await expect(page).toHaveURL(/\/billing/);

    await page.click('a:has-text("Settings"), a:has-text("Account")');
    await expect(page).toHaveURL(/\/settings|\/account/);
  });

  test('should show user profile menu', async ({ page }) => {
    await page.goto('/vms');

    await page.locator('button').filter({ hasText: /@/ }).first().click();

    await expect(page.locator('a:has-text("Account Settings")')).toBeVisible();
    await expect(page.locator('text="Log out"')).toBeVisible();
  });
});
