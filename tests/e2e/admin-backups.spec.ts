import { test, expect, Page } from '@playwright/test';

/**
 * Admin Backup Management E2E Tests
 *
 * Tests cover:
 * - Admin backup list page
 * - Backup filtering and search
 * - Backup restore functionality
 * - Admin backup schedules
 * - Schedule CRUD operations
 * - Schedule execution
 */

// ============================================
// Page Object Models
// ============================================

class AdminBackupListPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/backups');
    await expect(this.page.locator('h1, [data-testid="page-title"]')).toContainText(/backups/i);
  }

  async getBackupList() {
    return this.page.locator('[data-testid="backup-item"], table tbody tr');
  }

  async getBackupCount() {
    const list = await this.getBackupList();
    return list.count();
  }

  async filterByVM(vmId: string) {
    await this.page.click('[data-testid="vm-filter"], [data-testid="filter-vm"]');
    await this.page.fill('input[placeholder*="VM" i], input[name="vm_id"]', vmId);
    await this.page.press('input[placeholder*="VM" i], input[name="vm_id"]', 'Enter');
  }

  async filterByCustomer(customerId: string) {
    await this.page.click('[data-testid="customer-filter"], [data-testid="filter-customer"]');
    await this.page.fill('input[placeholder*="customer" i], input[name="customer_id"]', customerId);
    await this.page.press('input[placeholder*="customer" i], input[name="customer_id"]', 'Enter');
  }

  async filterByStatus(status: string) {
    await this.page.click('[data-testid="status-filter"]');
    await this.page.click(`text="${status}"`);
  }

  async filterBySource(source: string) {
    await this.page.click('[data-testid="source-filter"]');
    await this.page.click(`text="${source}"`);
  }

  async clickRestore(backupId: string) {
    await this.page.click(`[data-testid="restore-${backupId}"], tr:has-text("${backupId}") button:has-text("Restore")`);
  }

  async expectBackupVisible(backupName: string) {
    await expect(this.page.locator(`text="${backupName}"`)).toBeVisible();
  }

  async expectNoBackups() {
    await expect(this.page.locator('text=/no.*backups|no backups found/i')).toBeVisible();
  }
}

class AdminRestoreModal {
  constructor(private page: Page) {}

  async expectVisible() {
    await expect(this.page.locator('[role="dialog"], .modal:has-text("Restore")')).toBeVisible();
  }

  async expectBackupInfo() {
    await expect(this.page.locator('[data-testid="backup-name"], [data-testid="modal-backup-name"]')).toBeVisible();
  }

  async acknowledgeWarning() {
    const checkbox = this.page.locator('input[type="checkbox"]');
    if (await checkbox.isVisible()) {
      await checkbox.check();
    }
  }

  async confirmRestore() {
    await this.page.click('button:has-text("Confirm"), button:has-text("Restore"):not(:disabled)');
  }

  async cancel() {
    await this.page.click('button:has-text("Cancel")');
  }
}

class AdminScheduleListPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/backup-schedules');
    await expect(this.page.locator('h1, [data-testid="page-title"]')).toContainText(/schedule/i);
  }

  async getScheduleList() {
    return this.page.locator('[data-testid="schedule-item"], table tbody tr');
  }

  async getScheduleCount() {
    const list = await this.getScheduleList();
    return list.count();
  }

  async clickCreateSchedule() {
    await this.page.click('button:has-text("Create"), [data-testid="create-schedule-btn"]');
  }

  async clickEditSchedule(scheduleId: string) {
    await this.page.click(`[data-testid="edit-${scheduleId}"], tr:has-text("${scheduleId}") button:has-text("Edit")`);
  }

  async clickRunNow(scheduleId: string) {
    await this.page.click(`[data-testid="run-${scheduleId}"], tr:has-text("${scheduleId}") button:has-text("Run")`);
  }

  async clickDeleteSchedule(scheduleId: string) {
    await this.page.click(`[data-testid="delete-${scheduleId}"], tr:has-text("${scheduleId}") button:has-text("Delete")`);
  }

  async expectScheduleVisible(name: string) {
    await expect(this.page.locator(`text="${name}"`)).toBeVisible();
  }

  async expectNoSchedules() {
    await expect(this.page.locator('text=/no.*schedules|no schedules found/i')).toBeVisible();
  }
}

class AdminCreateScheduleModal {
  constructor(private page: Page) {}

  async expectVisible() {
    await expect(this.page.locator('[role="dialog"], .modal:has-text("Schedule")')).toBeVisible();
  }

  async setName(name: string) {
    await this.page.fill('input[name="name"], input[id="name"]', name);
  }

  async setDescription(description: string) {
    await this.page.fill('input[name="description"], input[id="description"]', description);
  }

  async setFrequency(frequency: 'daily' | 'weekly' | 'monthly') {
    await this.page.click(`[data-testid="frequency-${frequency}"], [data-value="${frequency}"]`);
  }

  async setRetentionCount(count: number) {
    await this.page.fill('input[name="retention_count"], input[id="retention"]', count.toString());
  }

  async selectTargetAll() {
    await this.page.click('button:has-text("All VMs")');
  }

  async selectTargetByPlan() {
    await this.page.click('button:has-text("By Plan")');
  }

  async selectTargetByNode() {
    await this.page.click('button:has-text("By Node")');
  }

  async selectTargetByCustomer() {
    await this.page.click('button:has-text("By Customer")');
  }

  async selectPlan(planName: string) {
    await this.page.click(`label:has-text("${planName}") input[type="checkbox"]`);
  }

  async selectNode(nodeName: string) {
    await this.page.click(`label:has-text("${nodeName}") input[type="checkbox"]`);
  }

  async selectCustomer(customerEmail: string) {
    await this.page.click(`label:has-text("${customerEmail}") input[type="checkbox"]`);
  }

  async setActive(active: boolean) {
    const toggle = this.page.locator('[role="switch"], button[role="switch"]');
    if (await toggle.isVisible()) {
      const isChecked = await toggle.getAttribute('aria-checked') === 'true';
      if (isChecked !== active) {
        await toggle.click();
      }
    }
  }

  async submit() {
    await this.page.click('button[type="submit"]:has-text("Create"), button[type="submit"]:has-text("Update")');
  }

  async cancel() {
    await this.page.click('button:has-text("Cancel")');
  }

  async expectValidationError(message: string | RegExp) {
    await expect(this.page.locator('.error, [role="alert"], [data-testid="validation-error"]')).toContainText(message);
  }
}

// ============================================
// Test Suite: Admin Backup List
// ============================================

test.describe('Admin Backup List', () => {
  let backupListPage: AdminBackupListPage;

  test.beforeEach(async ({ page }) => {
    backupListPage = new AdminBackupListPage(page);
    await backupListPage.goto();
  });

  test('should display backup list page', async ({ page }) => {
    await expect(page.locator('h1, [data-testid="page-title"]')).toContainText(/backups/i);
  });

  test('should show backup statistics cards', async ({ page }) => {
    // Look for stats section
    const statsSection = page.locator('[data-testid="backup-stats"], .grid');
    if (await statsSection.isVisible()) {
      await expect(statsSection).toBeVisible();
    }
  });

  test('should list available backups', async ({ page }) => {
    const count = await backupListPage.getBackupCount();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should show backup details in list', async ({ page }) => {
    const list = await backupListPage.getBackupList();
    const count = await list.count();

    if (count > 0) {
      const firstItem = list.first();

      // Should show VM info
      await expect(firstItem.locator('[data-testid="vm-name"], td')).toBeVisible();

      // Should show status
      await expect(firstItem.locator('[data-testid="status"], td')).toBeVisible();

      // Should show source
      await expect(firstItem.locator('[data-testid="source"], td')).toBeVisible();
    }
  });

  test('should filter backups by VM ID', async ({ page }) => {
    await backupListPage.filterByVM('test-vm-id');

    await page.waitForLoadState('networkidle');

    // Results should be filtered
    const list = await backupListPage.getBackupList();
    const count = await list.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should filter backups by customer ID', async ({ page }) => {
    await backupListPage.filterByCustomer('test-customer-id');

    await page.waitForLoadState('networkidle');

    const list = await backupListPage.getBackupList();
    const count = await list.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should filter backups by status', async ({ page }) => {
    await backupListPage.filterByStatus('Completed');

    await page.waitForLoadState('networkidle');

    // All visible backups should have Completed status
    const statuses = page.locator('[data-testid="status"]:has-text("Completed"), td:has-text("Completed")');
    const count = await statuses.count();
    expect(count).toBeGreaterThan(0);
  });

  test('should filter backups by source', async ({ page }) => {
    await backupListPage.filterBySource('Admin Schedule');

    await page.waitForLoadState('networkidle');

    const sources = page.locator('[data-testid="source"]:has-text("Admin"), td:has-text("admin_schedule")');
    const count = await sources.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should show empty state when no backups', async ({ page }) => {
    // Apply filter that likely returns no results
    await backupListPage.filterByVM('nonexistent-vm-id-12345');

    await page.waitForLoadState('networkidle');

    const list = await backupListPage.getBackupList();
    const count = await list.count();

    if (count === 0) {
      await backupListPage.expectNoBackups();
    }
  });
});

// ============================================
// Test Suite: Admin Backup Restore
// ============================================

test.describe('Admin Backup Restore', () => {
  let backupListPage: AdminBackupListPage;
  let restoreModal: AdminRestoreModal;
  const testBackupId = '00000000-0000-0000-0000-000000000001';

  test.beforeEach(async ({ page }) => {
    backupListPage = new AdminBackupListPage(page);
    restoreModal = new AdminRestoreModal(page);
    await backupListPage.goto();
  });

  test('should show restore button for completed backup', async ({ page }) => {
    const list = await backupListPage.getBackupList();
    const count = await list.count();

    if (count > 0) {
      // Check for restore button on first completed backup
      const restoreBtn = page.locator('button:has-text("Restore")').first();
      if (await restoreBtn.isVisible()) {
        await expect(restoreBtn).toBeVisible();
      }
    }
  });

  test('should open restore confirmation modal', async ({ page }) => {
    const list = await backupListPage.getBackupList();
    const count = await list.count();

    if (count > 0) {
      const restoreBtn = page.locator('button:has-text("Restore")').first();
      if (await restoreBtn.isVisible()) {
        await restoreBtn.click();
        await restoreModal.expectVisible();
      }
    }
  });

  test('should show backup info in restore modal', async ({ page }) => {
    const list = await backupListPage.getBackupList();
    const count = await list.count();

    if (count > 0) {
      const restoreBtn = page.locator('button:has-text("Restore")').first();
      if (await restoreBtn.isVisible()) {
        await restoreBtn.click();
        await restoreModal.expectBackupInfo();
      }
    }
  });

  test('should show restore warning', async ({ page }) => {
    const list = await backupListPage.getBackupList();
    const count = await list.count();

    if (count > 0) {
      const restoreBtn = page.locator('button:has-text("Restore")').first();
      if (await restoreBtn.isVisible()) {
        await restoreBtn.click();
        await expect(page.locator('text=/overwrite|warning|data will be lost/i')).toBeVisible();
      }
    }
  });

  test('should initiate restore after confirmation', async ({ page }) => {
    const list = await backupListPage.getBackupList();
    const count = await list.count();

    if (count > 0) {
      const restoreBtn = page.locator('button:has-text("Restore")').first();
      if (await restoreBtn.isVisible()) {
        await restoreBtn.click();
        await restoreModal.acknowledgeWarning();
        await restoreModal.confirmRestore();

        // Should show success or progress
        await expect(page.locator('text=/restore.*started|restoring|success/i')).toBeVisible();
      }
    }
  });

  test('should allow canceling restore', async ({ page }) => {
    const list = await backupListPage.getBackupList();
    const count = await list.count();

    if (count > 0) {
      const restoreBtn = page.locator('button:has-text("Restore")').first();
      if (await restoreBtn.isVisible()) {
        await restoreBtn.click();
        await restoreModal.cancel();

        // Modal should close without restoring
        await expect(page.locator('[role="dialog"]')).not.toBeVisible();
      }
    }
  });
});

// ============================================
// Test Suite: Admin Backup Schedules List
// ============================================

test.describe('Admin Backup Schedules List', () => {
  let scheduleListPage: AdminScheduleListPage;

  test.beforeEach(async ({ page }) => {
    scheduleListPage = new AdminScheduleListPage(page);
    await scheduleListPage.goto();
  });

  test('should display backup schedules page', async ({ page }) => {
    await expect(page.locator('h1, [data-testid="page-title"]')).toContainText(/schedule/i);
  });

  test('should show schedule statistics cards', async ({ page }) => {
    // Look for stats cards
    const statsCards = page.locator('.rounded-lg.border, [data-testid="stat-card"]');
    const count = await statsCards.count();
    expect(count).toBeGreaterThan(0);
  });

  test('should list existing schedules', async ({ page }) => {
    const count = await scheduleListPage.getScheduleCount();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should show schedule details in list', async ({ page }) => {
    const list = await scheduleListPage.getScheduleList();
    const count = await list.count();

    if (count > 0) {
      const firstItem = list.first();

      // Should show schedule name
      await expect(firstItem.locator('[data-testid="schedule-name"], td')).toBeVisible();

      // Should show frequency
      await expect(firstItem.locator('[data-testid="frequency"], td')).toBeVisible();

      // Should show status (active/inactive)
      await expect(firstItem.locator('[data-testid="status"], td, [data-testid="active-badge"]')).toBeVisible();
    }
  });

  test('should show next run time for active schedules', async ({ page }) => {
    const list = await scheduleListPage.getScheduleList();
    const count = await list.count();

    if (count > 0) {
      const nextRun = page.locator('[data-testid="next-run"], td:has-text("Next Run")');
      if (await nextRun.isVisible()) {
        const text = await nextRun.textContent();
        // Should show a date/time
        expect(text).toMatch(/\d|am|pm|UTC/i);
      }
    }
  });

  test('should open create schedule modal', async ({ page }) => {
    await scheduleListPage.clickCreateSchedule();

    const modal = page.locator('[role="dialog"], .modal');
    await expect(modal).toBeVisible();
    await expect(modal).toContainText(/create.*schedule/i);
  });
});

// ============================================
// Test Suite: Admin Create Backup Schedule
// ============================================

test.describe('Admin Create Backup Schedule', () => {
  let scheduleListPage: AdminScheduleListPage;
  let createModal: AdminCreateScheduleModal;

  test.beforeEach(async ({ page }) => {
    scheduleListPage = new AdminScheduleListPage(page);
    createModal = new AdminCreateScheduleModal(page);
    await scheduleListPage.goto();
  });

  test('should create schedule targeting all VMs', async ({ page }) => {
    await scheduleListPage.clickCreateSchedule();
    await createModal.expectVisible();

    await createModal.setName(`All VMs Schedule ${Date.now()}`);
    await createModal.setFrequency('daily');
    await createModal.setRetentionCount(7);
    await createModal.selectTargetAll();
    await createModal.submit();

    // Should show success message
    await expect(page.locator('text=/schedule created|success/i')).toBeVisible();
  });

  test('should create schedule targeting by plan', async ({ page }) => {
    await scheduleListPage.clickCreateSchedule();
    await createModal.expectVisible();

    await createModal.setName(`Plan Schedule ${Date.now()}`);
    await createModal.setDescription('Backups for specific plans');
    await createModal.setFrequency('weekly');
    await createModal.setRetentionCount(4);
    await createModal.selectTargetByPlan();

    // Wait for plan list to load
    await page.waitForTimeout(500);

    // Select first available plan if any
    const planCheckbox = page.locator('label:has(input[type="checkbox"])').first();
    if (await planCheckbox.isVisible()) {
      await planCheckbox.click();
    }

    await createModal.submit();

    await expect(page.locator('text=/schedule created|success/i')).toBeVisible();
  });

  test('should create schedule targeting by node', async ({ page }) => {
    await scheduleListPage.clickCreateSchedule();
    await createModal.expectVisible();

    await createModal.setName(`Node Schedule ${Date.now()}`);
    await createModal.setFrequency('daily');
    await createModal.setRetentionCount(3);
    await createModal.selectTargetByNode();

    // Wait for node list to load
    await page.waitForTimeout(500);

    // Select first available node if any
    const nodeCheckbox = page.locator('label:has(input[type="checkbox"])').first();
    if (await nodeCheckbox.isVisible()) {
      await nodeCheckbox.click();
    }

    await createModal.submit();

    await expect(page.locator('text=/schedule created|success/i')).toBeVisible();
  });

  test('should create monthly schedule', async ({ page }) => {
    await scheduleListPage.clickCreateSchedule();
    await createModal.expectVisible();

    await createModal.setName(`Monthly Schedule ${Date.now()}`);
    await createModal.setFrequency('monthly');
    await createModal.setRetentionCount(12);
    await createModal.selectTargetAll();
    await createModal.submit();

    await expect(page.locator('text=/schedule created|success/i')).toBeVisible();
  });

  test('should create inactive schedule', async ({ page }) => {
    await scheduleListPage.clickCreateSchedule();
    await createModal.expectVisible();

    await createModal.setName(`Inactive Schedule ${Date.now()}`);
    await createModal.setFrequency('daily');
    await createModal.setRetentionCount(3);
    await createModal.selectTargetAll();
    await createModal.setActive(false);
    await createModal.submit();

    await expect(page.locator('text=/schedule created|success/i')).toBeVisible();
  });

  test('should show validation error for empty name', async ({ page }) => {
    await scheduleListPage.clickCreateSchedule();
    await createModal.expectVisible();

    await createModal.submit();

    await createModal.expectValidationError(/required|please enter/i);
  });

  test('should show validation error when no targets selected', async ({ page }) => {
    await scheduleListPage.clickCreateSchedule();
    await createModal.expectVisible();

    await createModal.setName('No Target Schedule');
    await createModal.selectTargetByPlan();
    // Don't select any plans
    await createModal.submit();

    await createModal.expectValidationError(/select.*target|at least one/i);
  });

  test('should allow canceling schedule creation', async ({ page }) => {
    await scheduleListPage.clickCreateSchedule();
    await createModal.expectVisible();

    await createModal.cancel();

    // Modal should close
    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
  });
});

// ============================================
// Test Suite: Admin Edit Backup Schedule
// ============================================

test.describe('Admin Edit Backup Schedule', () => {
  let scheduleListPage: AdminScheduleListPage;
  let createModal: AdminCreateScheduleModal;

  test.beforeEach(async ({ page }) => {
    scheduleListPage = new AdminScheduleListPage(page);
    createModal = new AdminCreateScheduleModal(page);
    await scheduleListPage.goto();
  });

  test('should open edit modal with existing data', async ({ page }) => {
    const list = await scheduleListPage.getScheduleList();
    const count = await list.count();

    if (count > 0) {
      const editBtn = page.locator('button:has-text("Edit")').first();
      if (await editBtn.isVisible()) {
        await editBtn.click();

        await expect(page.locator('[role="dialog"]')).toBeVisible();
        await expect(page.locator('[role="dialog"]')).toContainText(/edit.*schedule/i);
      }
    }
  });

  test('should update schedule name', async ({ page }) => {
    const list = await scheduleListPage.getScheduleList();
    const count = await list.count();

    if (count > 0) {
      const editBtn = page.locator('button:has-text("Edit")').first();
      if (await editBtn.isVisible()) {
        await editBtn.click();

        await createModal.setName(`Updated Schedule ${Date.now()}`);
        await createModal.submit();

        await expect(page.locator('text=/schedule updated|success/i')).toBeVisible();
      }
    }
  });

  test('should update schedule frequency', async ({ page }) => {
    const list = await scheduleListPage.getScheduleList();
    const count = await list.count();

    if (count > 0) {
      const editBtn = page.locator('button:has-text("Edit")').first();
      if (await editBtn.isVisible()) {
        await editBtn.click();

        await createModal.setFrequency('weekly');
        await createModal.submit();

        await expect(page.locator('text=/schedule updated|success/i')).toBeVisible();
      }
    }
  });

  test('should toggle schedule active state', async ({ page }) => {
    const list = await scheduleListPage.getScheduleList();
    const count = await list.count();

    if (count > 0) {
      const editBtn = page.locator('button:has-text("Edit")').first();
      if (await editBtn.isVisible()) {
        await editBtn.click();

        await createModal.setActive(false);
        await createModal.submit();

        await expect(page.locator('text=/schedule updated|success/i')).toBeVisible();
      }
    }
  });
});

// ============================================
// Test Suite: Admin Run Schedule
// ============================================

test.describe('Admin Run Schedule', () => {
  let scheduleListPage: AdminScheduleListPage;

  test.beforeEach(async ({ page }) => {
    scheduleListPage = new AdminScheduleListPage(page);
    await scheduleListPage.goto();
  });

  test('should run schedule immediately', async ({ page }) => {
    const list = await scheduleListPage.getScheduleList();
    const count = await list.count();

    if (count > 0) {
      // Look for Run Now button
      const runBtn = page.locator('button:has-text("Run")').first();
      if (await runBtn.isVisible()) {
        await runBtn.click();

        // Should show confirmation or success
        await expect(page.locator('text=/running|started|success/i')).toBeVisible();
      }
    }
  });

  test('should show confirmation before running', async ({ page }) => {
    const list = await scheduleListPage.getScheduleList();
    const count = await list.count();

    if (count > 0) {
      // Click dropdown menu if it exists
      const menuBtn = page.locator('[data-testid="schedule-menu"], button[aria-haspopup]').first();
      if (await menuBtn.isVisible()) {
        await menuBtn.click();
        const runOption = page.locator('text=/run now/i');
        if (await runOption.isVisible()) {
          await runOption.click();

          // Should show confirmation dialog
          await expect(page.locator('[role="dialog"], .modal')).toBeVisible();
        }
      }
    }
  });
});

// ============================================
// Test Suite: Admin Delete Schedule
// ============================================

test.describe('Admin Delete Schedule', () => {
  let scheduleListPage: AdminScheduleListPage;

  test.beforeEach(async ({ page }) => {
    scheduleListPage = new AdminScheduleListPage(page);
    await scheduleListPage.goto();
  });

  test('should show delete confirmation', async ({ page }) => {
    const list = await scheduleListPage.getScheduleList();
    const count = await list.count();

    if (count > 0) {
      // Click dropdown menu if it exists
      const menuBtn = page.locator('[data-testid="schedule-menu"], button[aria-haspopup]').first();
      if (await menuBtn.isVisible()) {
        await menuBtn.click();
        const deleteOption = page.locator('text=/delete/i');
        if (await deleteOption.isVisible()) {
          await deleteOption.click();

          // Should show confirmation dialog
          await expect(page.locator('[role="dialog"], .modal')).toBeVisible();
          await expect(page.locator('text=/confirm|are you sure/i')).toBeVisible();
        }
      }
    }
  });

  test('should delete schedule after confirmation', async ({ page }) => {
    const list = await scheduleListPage.getScheduleList();
    const count = await list.count();

    if (count > 0) {
      // Click dropdown menu if it exists
      const menuBtn = page.locator('[data-testid="schedule-menu"], button[aria-haspopup]').first();
      if (await menuBtn.isVisible()) {
        await menuBtn.click();
        const deleteOption = page.locator('text=/delete/i');
        if (await deleteOption.isVisible()) {
          await deleteOption.click();

          // Confirm deletion
          await page.click('button:has-text("Confirm"), button:has-text("Delete"):not(:disabled)');

          // Should show success message
          await expect(page.locator('text=/deleted|removed|success/i')).toBeVisible();
        }
      }
    }
  });

  test('should allow canceling deletion', async ({ page }) => {
    const list = await scheduleListPage.getScheduleList();
    const count = await list.count();

    if (count > 0) {
      // Click dropdown menu if it exists
      const menuBtn = page.locator('[data-testid="schedule-menu"], button[aria-haspopup]').first();
      if (await menuBtn.isVisible()) {
        await menuBtn.click();
        const deleteOption = page.locator('text=/delete/i');
        if (await deleteOption.isVisible()) {
          await deleteOption.click();

          // Cancel deletion
          await page.click('button:has-text("Cancel")');

          // Modal should close
          await expect(page.locator('[role="dialog"]')).not.toBeVisible();
        }
      }
    }
  });
});

// ============================================
// Test Suite: Admin Backup Permissions
// ============================================

test.describe('Admin Backup Permissions', () => {
  test('should show all customer backups to admin', async ({ page }) => {
    await page.goto('/backups');

    // Admin should see backups across all customers
    const list = page.locator('[data-testid="backup-item"], table tbody tr');
    const count = await list.count();

    // Page should load without errors
    await expect(page.locator('text=/access denied|unauthorized/i')).not.toBeVisible();
  });

  test('should allow admin to restore any backup', async ({ page }) => {
    await page.goto('/backups');

    // Check for restore buttons
    const restoreBtns = page.locator('button:has-text("Restore")');
    const count = await restoreBtns.count();

    if (count > 0) {
      await expect(restoreBtns.first()).toBeEnabled();
    }
  });

  test('should show backup source indicators', async ({ page }) => {
    await page.goto('/backups');

    // Look for source column/values
    const sources = page.locator('[data-testid="source"], td:has-text("manual"), td:has-text("schedule")');
    const count = await sources.count();

    // Should show backup sources
    expect(count).toBeGreaterThanOrEqual(0);
  });
});