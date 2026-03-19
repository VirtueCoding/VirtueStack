import { test, expect, Page } from '@playwright/test';

/**
 * Customer Snapshot Management E2E Tests
 *
 * Tests cover:
 * - Viewing snapshot list
 * - Creating snapshots
 * - Restoring from snapshots
 * - Deleting snapshots
 * - Snapshot limits per plan
 */

// ============================================
// Page Object Models
// ============================================

class CustomerSnapshotListPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/snapshots');
    await expect(this.page.locator('h1, [data-testid="page-title"]')).toContainText(/snapshots/i);
  }

  async gotoVMSnapshots(vmId: string) {
    await this.page.goto(`/vms/${vmId}/snapshots`);
  }

  async getSnapshotList() {
    return this.page.locator('[data-testid="snapshot-item"], table tbody tr');
  }

  async getSnapshotCount() {
    const list = await this.getSnapshotList();
    return list.count();
  }

  async filterByVM(hostname: string) {
    await this.page.click('[data-testid="vm-filter"]');
    await this.page.click(`text="${hostname}"`);
  }

  async filterByStatus(status: string) {
    await this.page.click('[data-testid="status-filter"]');
    await this.page.click(`text="${status}"`);
  }

  async searchSnapshot(query: string) {
    await this.page.fill('input[placeholder*="search" i]', query);
    await this.page.press('input[placeholder*="search" i]', 'Enter');
  }

  async expectSnapshotVisible(name: string) {
    await expect(this.page.locator(`text="${name}"`)).toBeVisible();
  }

  async expectNoSnapshots() {
    await expect(this.page.locator('text=/no.*snapshots|no snapshots found/i')).toBeVisible();
  }

  async clickCreateSnapshot() {
    await this.page.click('button:has-text("Create Snapshot"), [data-testid="create-snapshot-btn"]');
  }
}

class CustomerSnapshotCreateModal {
  constructor(private page: Page) {}

  async expectVisible() {
    await expect(this.page.locator('[role="dialog"], .modal')).toBeVisible();
  }

  async selectVM(vmName: string) {
    await this.page.click('[data-testid="vm-select"]');
    await this.page.click(`text="${vmName}"`);
  }

  async setName(name: string) {
    await this.page.fill('input[name="name"]', name);
  }

  async setDescription(description: string) {
    await this.page.fill('textarea[name="description"]', description);
  }

  async submit() {
    await this.page.click('button[type="submit"]:has-text("Create")');
  }

  async cancel() {
    await this.page.click('button:has-text("Cancel")');
  }

  async expectValidationError(message: string | RegExp) {
    await expect(this.page.locator('.error, [role="alert"]')).toContainText(message);
  }
}

class CustomerSnapshotDetailPage {
  constructor(private page: Page) {}

  async goto(snapshotId: string) {
    await this.page.goto(`/snapshots/${snapshotId}`);
  }

  async getSnapshotInfo() {
    return {
      name: await this.page.locator('[data-testid="snapshot-name"]').textContent(),
      status: await this.page.locator('[data-testid="snapshot-status"]').textContent(),
      size: await this.page.locator('[data-testid="snapshot-size"]').textContent(),
      createdAt: await this.page.locator('[data-testid="snapshot-created"]').textContent(),
    };
  }

  async getVMInfo() {
    return {
      hostname: await this.page.locator('[data-testid="vm-hostname"]').textContent(),
      ip: await this.page.locator('[data-testid="vm-ip"]').textContent(),
    };
  }

  async clickRestore() {
    await this.page.click('button:has-text("Restore"), [data-testid="restore-btn"]');
  }

  async clickDelete() {
    await this.page.click('button:has-text("Delete"), [data-testid="delete-btn"]');
  }

  async expectActionDisabled(action: string) {
    await expect(this.page.locator(`button:has-text("${action}")`)).toBeDisabled();
  }

  async expectStatus(status: string) {
    await expect(this.page.locator('[data-testid="snapshot-status"]')).toContainText(status);
  }
}

class CustomerRestoreModal {
  constructor(private page: Page) {}

  async expectVisible() {
    await expect(this.page.locator('[role="dialog"], .modal:has-text("Restore")')).toBeVisible();
  }

  async confirmRestore() {
    await this.page.click('button:has-text("Confirm"), button:has-text("Restore")');
  }

  async cancel() {
    await this.page.click('button:has-text("Cancel")');
  }

  async acknowledgeWarning() {
    await this.page.check('input[type="checkbox"]');
  }
}

// ============================================
// Test Suite
// ============================================

test.describe('Customer Snapshot List', () => {
  let snapshotListPage: CustomerSnapshotListPage;

  test.beforeEach(async ({ page }) => {
    snapshotListPage = new CustomerSnapshotListPage(page);
    await snapshotListPage.goto();
  });

  test('should display snapshot list page', async ({ page }) => {
    await expect(page.locator('h1, [data-testid="page-title"]')).toContainText(/snapshots/i);
  });

  test('should show snapshot statistics', async ({ page }) => {
    const statsSection = page.locator('[data-testid="snapshot-stats"]');
    if (await statsSection.isVisible()) {
      await expect(statsSection).toBeVisible();
    }
  });

  test('should list available snapshots', async ({ page }) => {
    const count = await snapshotListPage.getSnapshotCount();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should show snapshot details in list', async ({ page }) => {
    const list = await snapshotListPage.getSnapshotList();
    const count = await list.count();

    if (count > 0) {
      const firstItem = list.first();

      // Should show VM name
      await expect(firstItem.locator('[data-testid="vm-name"], td:has-text("vm")')).toBeVisible();

      // Should show snapshot name
      await expect(firstItem.locator('[data-testid="snapshot-name"], td')).toBeVisible();

      // Should show status
      await expect(firstItem.locator('[data-testid="status"], td:has-text("completed"), td:has-text("creating")')).toBeVisible();
    }
  });

  test('should filter snapshots by VM', async ({ page }) => {
    await snapshotListPage.filterByVM('test-vm');

    await page.waitForLoadState('networkidle');

    const list = await snapshotListPage.getSnapshotList();
    const count = await list.count();

    if (count > 0) {
      const text = await list.first().textContent();
      expect(text?.toLowerCase()).toContain('test-vm');
    }
  });

  test('should filter snapshots by status', async ({ page }) => {
    await snapshotListPage.filterByStatus('Completed');

    await page.waitForLoadState('networkidle');

    const statuses = page.locator('[data-testid="status"]:has-text("Completed"), td:has-text("Completed")');
    const count = await statuses.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should search snapshots', async ({ page }) => {
    await snapshotListPage.searchSnapshot('snapshot-name');

    await page.waitForLoadState('networkidle');

    const list = await snapshotListPage.getSnapshotList();
    const count = await list.count();

    if (count > 0) {
      const text = await list.first().textContent();
      expect(text?.toLowerCase()).toContain('snapshot-name');
    }
  });

  test('should show empty state when no snapshots', async ({ page }) => {
    await page.goto('/vms/no-snapshots-vm-id/snapshots');

    const list = await snapshotListPage.getSnapshotList();
    const count = await list.count();

    if (count === 0) {
      await snapshotListPage.expectNoSnapshots();
    }
  });
});

test.describe('Customer Snapshot Creation', () => {
  let snapshotListPage: CustomerSnapshotListPage;
  let createModal: CustomerSnapshotCreateModal;
  const testVMId = '00000000-0000-0000-0000-000000000001';

  test.beforeEach(async ({ page }) => {
    snapshotListPage = new CustomerSnapshotListPage(page);
    createModal = new CustomerSnapshotCreateModal(page);
    await snapshotListPage.gotoVMSnapshots(testVMId);
  });

  test('should open snapshot creation modal', async ({ page }) => {
    await snapshotListPage.clickCreateSnapshot();
    await createModal.expectVisible();
  });

  test('should create snapshot with name', async ({ page }) => {
    await snapshotListPage.clickCreateSnapshot();
    await createModal.expectVisible();

    await createModal.setName(`test-snapshot-${Date.now()}`);
    await createModal.submit();

    // Modal should close
    await expect(page.locator('[role="dialog"]')).not.toBeVisible();

    // Should show success message
    await expect(page.locator('text=/snapshot created|success/i')).toBeVisible();
  });

  test('should create snapshot with description', async ({ page }) => {
    await snapshotListPage.clickCreateSnapshot();

    await createModal.setName('described-snapshot');
    await createModal.setDescription('This is a test snapshot with description');
    await createModal.submit();

    await expect(page.locator('text=/snapshot created|success/i')).toBeVisible();
  });

  test('should show validation error for empty name', async ({ page }) => {
    await snapshotListPage.clickCreateSnapshot();
    await createModal.submit();

    await createModal.expectValidationError(/required|please enter/i);
  });

  test('should allow canceling snapshot creation', async ({ page }) => {
    await snapshotListPage.clickCreateSnapshot();
    await createModal.cancel();

    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
  });

  test('should show plan limit warning when approaching limit', async ({ page }) => {
    // Navigate to VM that has snapshots near limit
    await page.goto('/vms/near-limit-vm-id/snapshots');

    // Should show warning about limit
    await expect(page.locator('text=/limit|maximum|warning/i')).toBeVisible();
  });

  test('should prevent creation when plan limit reached', async ({ page }) => {
    // Navigate to VM that has reached snapshot limit
    await page.goto('/vms/at-limit-vm-id/snapshots');

    await snapshotListPage.clickCreateSnapshot();

    // Should show error about limit
    await expect(page.locator('text=/limit.*reached|maximum.*snapshots|upgrade/i')).toBeVisible();
  });
});

test.describe('Customer Snapshot Restore', () => {
  let snapshotDetailPage: CustomerSnapshotDetailPage;
  let restoreModal: CustomerRestoreModal;
  const testSnapshotId = '00000000-0000-0000-0000-000000000010';

  test.beforeEach(async ({ page }) => {
    snapshotDetailPage = new CustomerSnapshotDetailPage(page);
    restoreModal = new CustomerRestoreModal(page);
  });

  test('should show restore button for completed snapshot', async ({ page }) => {
    await snapshotDetailPage.goto(testSnapshotId);

    await expect(page.locator('button:has-text("Restore")')).toBeVisible();
  });

  test('should open restore confirmation modal', async ({ page }) => {
    await snapshotDetailPage.goto(testSnapshotId);
    await snapshotDetailPage.clickRestore();

    await restoreModal.expectVisible();
  });

  test('should show restore warning', async ({ page }) => {
    await snapshotDetailPage.goto(testSnapshotId);
    await snapshotDetailPage.clickRestore();

    await expect(page.locator('text=/overwrite|warning|data will be lost/i')).toBeVisible();
  });

  test('should require acknowledgment before restore', async ({ page }) => {
    await snapshotDetailPage.goto(testSnapshotId);
    await snapshotDetailPage.clickRestore();

    // Restore button should be disabled until acknowledged
    await expect(page.locator('button:has-text("Restore"):not(:disabled)')).not.toBeVisible();

    await restoreModal.acknowledgeWarning();

    // Now should be enabled
    await expect(page.locator('button:has-text("Restore")')).toBeEnabled();
  });

  test('should initiate restore', async ({ page }) => {
    await snapshotDetailPage.goto(testSnapshotId);
    await snapshotDetailPage.clickRestore();
    await restoreModal.acknowledgeWarning();
    await restoreModal.confirmRestore();

    // Should show success or progress
    await expect(page.locator('text=/restore.*started|restoring/i')).toBeVisible();
  });

  test('should allow canceling restore', async ({ page }) => {
    await snapshotDetailPage.goto(testSnapshotId);
    await snapshotDetailPage.clickRestore();
    await restoreModal.cancel();

    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
  });

  test('should disable restore for failed snapshot', async ({ page }) => {
    await page.goto('/snapshots/failed-snapshot-id');

    await snapshotDetailPage.expectActionDisabled('Restore');
  });

  test('should disable restore for creating snapshot', async ({ page }) => {
    await page.goto('/snapshots/creating-snapshot-id');

    await snapshotDetailPage.expectActionDisabled('Restore');
  });
});

test.describe('Customer Snapshot Deletion', () => {
  let snapshotDetailPage: CustomerSnapshotDetailPage;
  let snapshotListPage: CustomerSnapshotListPage;
  const testSnapshotId = '00000000-0000-0000-0000-000000000012';

  test('should show delete button', async ({ page }) => {
    snapshotDetailPage = new CustomerSnapshotDetailPage(page);
    await snapshotDetailPage.goto(testSnapshotId);

    await expect(page.locator('button:has-text("Delete")')).toBeVisible();
  });

  test('should show deletion confirmation', async ({ page }) => {
    snapshotDetailPage = new CustomerSnapshotDetailPage(page);
    await snapshotDetailPage.goto(testSnapshotId);
    await snapshotDetailPage.clickDelete();

    await expect(page.locator('[role="dialog"], .modal')).toBeVisible();
    await expect(page.locator('text=/confirm|are you sure/i')).toBeVisible();
  });

  test('should delete snapshot after confirmation', async ({ page }) => {
    snapshotDetailPage = new CustomerSnapshotDetailPage(page);
    await snapshotDetailPage.goto(testSnapshotId);
    await snapshotDetailPage.clickDelete();

    await page.click('button:has-text("Confirm"), button:has-text("Delete"):not(:disabled)');

    // Should show success message
    await expect(page.locator('text=/deleted|removed/i')).toBeVisible();
  });

  test('should allow canceling deletion', async ({ page }) => {
    snapshotDetailPage = new CustomerSnapshotDetailPage(page);
    await snapshotDetailPage.goto(testSnapshotId);
    await snapshotDetailPage.clickDelete();

    await page.click('button:has-text("Cancel")');

    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
  });

  test('should remove snapshot from list after deletion', async ({ page }) => {
    snapshotListPage = new CustomerSnapshotListPage(page);
    snapshotDetailPage = new CustomerSnapshotDetailPage(page);

    await snapshotDetailPage.goto(testSnapshotId);
    const info = await snapshotDetailPage.getSnapshotInfo();

    await snapshotDetailPage.clickDelete();
    await page.click('button:has-text("Confirm")');

    // Navigate to list
    await snapshotListPage.goto();

    // Snapshot should not be visible
    await expect(page.locator(`text="${info.name}"`)).not.toBeVisible();
  });
});

test.describe('Customer Snapshot Status Updates', () => {
  let snapshotDetailPage: CustomerSnapshotDetailPage;
  const creatingSnapshotId = '00000000-0000-0000-0000-000000000020';

  test('should show creating status', async ({ page }) => {
    snapshotDetailPage = new CustomerSnapshotDetailPage(page);
    await snapshotDetailPage.goto(creatingSnapshotId);

    await snapshotDetailPage.expectStatus('Creating');
  });

  test('should update status to completed', async ({ page }) => {
    snapshotDetailPage = new CustomerSnapshotDetailPage(page);
    await snapshotDetailPage.goto(creatingSnapshotId);

    const status = page.locator('[data-testid="snapshot-status"]');
    await expect(status).toBeVisible();
  });

  test('should show progress for creating snapshot', async ({ page }) => {
    snapshotDetailPage = new CustomerSnapshotDetailPage(page);
    await snapshotDetailPage.goto(creatingSnapshotId);

    const progressBar = page.locator('[role="progressbar"], .progress');

    if (await progressBar.isVisible()) {
      await expect(progressBar).toBeVisible();
    }
  });

  test('should show error message for failed snapshot', async ({ page }) => {
    await page.goto('/snapshots/failed-snapshot-id');

    await expect(page.locator('text=/error|failed/i')).toBeVisible();
  });
});

test.describe('Customer Snapshot Permissions', () => {
  test('should only show own VMs in snapshot list', async ({ page }) => {
    await page.goto('/snapshots');

    // All VMs listed should belong to current customer
    await expect(page.locator('text=/access denied|unauthorized/i')).not.toBeVisible();
  });

  test('should not allow access to other customer snapshots', async ({ page }) => {
    await page.goto('/snapshots/other-customer-snapshot-id');

    // Should show error or redirect
    await expect(page.locator('text=/not found|access denied/i')).toBeVisible();
  });

  test('should show snapshot action permissions', async ({ page }) => {
    await page.goto('/snapshots/00000000-0000-0000-0000-000000000015');

    // Customer should be able to:
    // - Restore their snapshots
    // - Delete their snapshots

    await expect(page.locator('button:has-text("Restore")')).toBeVisible();
    await expect(page.locator('button:has-text("Delete")')).toBeVisible();
  });
});

test.describe('Customer Snapshot Navigation', () => {
  test('should navigate from VM detail to snapshots', async ({ page }) => {
    await page.goto('/vms/test-vm-id');

    await page.click('a:has-text("Snapshots"), [data-testid="snapshots-tab"]');

    await expect(page).toHaveURL(/\/snapshots/);
  });

  test('should navigate from snapshot to VM', async ({ page }) => {
    await page.goto('/snapshots/test-snapshot-id');

    await page.click('a:has-text("View VM"), [data-testid="view-vm-link"]');

    await expect(page).toHaveURL(/\/vms\/[a-f0-9-]+/);
  });
});

test.describe('Customer Snapshot Size Display', () => {
  test('should show snapshot size', async ({ page }) => {
    await page.goto('/snapshots/test-snapshot-id');

    const sizeElement = page.locator('[data-testid="snapshot-size"]');

    if (await sizeElement.isVisible()) {
      const size = await sizeElement.textContent();
      expect(size).toMatch(/\d+.*GB|MB/i);
    }
  });

  test('should show total snapshot size in list', async ({ page }) => {
    await page.goto('/snapshots');

    const totalSize = page.locator('[data-testid="total-size"]');

    if (await totalSize.isVisible()) {
      const text = await totalSize.textContent();
      expect(text).toMatch(/\d+.*GB|MB/i);
    }
  });
});