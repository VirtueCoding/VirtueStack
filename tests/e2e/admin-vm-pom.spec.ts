/**
 * Admin VM Management E2E Tests (Refactored)
 *
 * Uses the Page Object Model pattern for better maintainability.
 */

import { adminTest as test, expect } from './fixtures';
import { TEST_IDS } from './utils/api';

// ============================================
// Admin VM List Tests
// ============================================

test.describe('Admin VM List', () => {
  test.use({ storageState: '.auth/admin-storage.json' });

  test('should display VM list page', async ({ adminVMListPage }) => {
    await adminVMListPage.goto();
    await adminVMListPage.expectTableVisible();
  });

  test('should show VM counts in pagination', async ({ adminVMListPage }) => {
    await adminVMListPage.goto();
    const paginationText = await adminVMListPage.getPaginationText();
    expect(paginationText).toMatch(/\d+.*VMs?/i);
  });

  test('should search VMs by hostname', async ({ adminVMListPage, page }) => {
    await adminVMListPage.goto();
    await adminVMListPage.searchVM('test-vm');

    const count = await adminVMListPage.getVMCount();
    if (count > 0) {
      await adminVMListPage.expectVMInList('test-vm');
    }
  });

  test('should filter VMs by status', async ({ adminVMListPage, page }) => {
    await adminVMListPage.goto();
    await adminVMListPage.filterByStatus('Running');

    // Verify filtered results
    const statusCells = page.locator('table tbody td:has-text("Running"), [data-testid="vm-status"]:has-text("Running")');
    const count = await statusCells.count();
    expect(count).toBeGreaterThan(0);
  });

  test('should paginate VM list', async ({ adminVMListPage, page }) => {
    await adminVMListPage.goto();

    const nextBtn = page.locator('button:has-text("Next"), [data-testid="next-page"]');
    if (await nextBtn.isEnabled()) {
      await adminVMListPage.goToNextPage();

      const count = await adminVMListPage.getVMCount();
      expect(count).toBeGreaterThan(0);

      await adminVMListPage.goToPreviousPage();
    }
  });

  test('should navigate to create VM page', async ({ adminVMListPage }) => {
    await adminVMListPage.goto();
    await adminVMListPage.clickCreateVM();
    await expect(adminVMListPage['page']).toHaveURL(/\/admin\/vms\/create/);
  });
});

// ============================================
// Admin VM Detail Tests
// ============================================

test.describe('Admin VM Detail', () => {
  test.use({ storageState: '.auth/admin-storage.json' });

  test('should display VM details', async ({ adminVMDetailPage }) => {
    await adminVMDetailPage.goto(TEST_IDS.vms.testVM1);

    const info = await adminVMDetailPage.getVMInfo();
    expect(info.hostname).toBeTruthy();
    expect(info.status).toBeTruthy();
  });

  test('should show IP addresses', async ({ adminVMDetailPage }) => {
    await adminVMDetailPage.goto(TEST_IDS.vms.testVM1);

    const ips = await adminVMDetailPage.getIPAddresses();
    expect(ips.length).toBeGreaterThan(0);

    for (const ip of ips) {
      expect(ip).toMatch(/\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}/);
    }
  });

  test('should show action buttons', async ({ adminVMDetailPage }) => {
    await adminVMDetailPage.goto(TEST_IDS.vms.testVM1);

    const status = await adminVMDetailPage.getStatus();

    if (status.toLowerCase().includes('running')) {
      await adminVMDetailPage.expectActionEnabled('Stop');
      await adminVMDetailPage.expectActionEnabled('Reboot');
      await adminVMDetailPage.expectActionDisabled('Start');
    } else if (status.toLowerCase().includes('stopped')) {
      await adminVMDetailPage.expectActionEnabled('Start');
      await adminVMDetailPage.expectActionDisabled('Stop');
    }
  });
});

// ============================================
// Admin VM Power Operations Tests
// ============================================

test.describe('Admin VM Power Operations', () => {
  test.use({ storageState: '.auth/admin-storage.json' });

  test('should start a stopped VM', async ({ adminVMDetailPage }) => {
    await adminVMDetailPage.goto(TEST_IDS.vms.testVM2);

    const status = await adminVMDetailPage.getStatus();

    if (status.toLowerCase().includes('stopped')) {
      await adminVMDetailPage.startVM();

      const newStatus = await adminVMDetailPage.getStatus();
      expect(newStatus.toLowerCase()).toContain('running');
    }
  });

  test('should stop a running VM', async ({ adminVMDetailPage }) => {
    await adminVMDetailPage.goto(TEST_IDS.vms.testVM2);

    const status = await adminVMDetailPage.getStatus();

    if (status.toLowerCase().includes('running')) {
      await adminVMDetailPage.stopVM();

      const newStatus = await adminVMDetailPage.getStatus();
      expect(newStatus.toLowerCase()).toContain('stopped');
    }
  });

  test('should reboot a running VM', async ({ adminVMDetailPage }) => {
    await adminVMDetailPage.goto(TEST_IDS.vms.testVM2);

    const status = await adminVMDetailPage.getStatus();

    if (status.toLowerCase().includes('running')) {
      await adminVMDetailPage.rebootVM();
      await adminVMDetailPage.waitForStatus('running', 60000);
    }
  });
});

// ============================================
// Admin VM Deletion Tests
// ============================================

test.describe('Admin VM Deletion', () => {
  test.use({ storageState: '.auth/admin-storage.json' });

  test('should show deletion confirmation modal', async ({ adminVMDetailPage, page }) => {
    await adminVMDetailPage.goto(TEST_IDS.vms.testVM3);

    await page.click('button:has-text("Delete")');

    // Should show confirmation modal
    await expect(page.locator('[role="dialog"], .modal')).toBeVisible();
    await expect(page.locator('text=/confirm|are you sure/i')).toBeVisible();
  });

  test('should cancel deletion', async ({ adminVMDetailPage, page }) => {
    await adminVMDetailPage.goto(TEST_IDS.vms.testVM3);

    await page.click('button:has-text("Delete")');
    await page.click('button:has-text("Cancel")');

    // Modal should close
    await expect(page.locator('[role="dialog"], .modal')).not.toBeVisible();

    // VM should still be visible
    const hostname = await adminVMDetailPage.getHostname();
    expect(hostname).toBeTruthy();
  });
});
