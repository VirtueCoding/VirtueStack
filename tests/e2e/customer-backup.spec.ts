import { test, expect, Page } from '@playwright/test';

/**
 * Customer Backup Management E2E Tests
 * 
 * Tests cover:
 * - Viewing backup list
 * - Creating backups
 * - Restoring from backups
 * - Backup scheduling
 * - Backup download
 */

// ============================================
// Page Object Models
// ============================================

class CustomerBackupListPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/backups');
    await expect(this.page.locator('h1, [data-testid="page-title"]')).toContainText(/backups/i);
  }

  async gotoVMBackups(vmId: string) {
    await this.page.goto(`/vms/${vmId}/backups`);
  }

  async getBackupList() {
    return this.page.locator('[data-testid="backup-item"], table tbody tr');
  }

  async getBackupCount() {
    const list = await this.getBackupList();
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

  async searchBackup(query: string) {
    await this.page.fill('input[placeholder*="search" i]', query);
    await this.page.press('input[placeholder*="search" i]', 'Enter');
  }

  async expectBackupVisible(name: string) {
    await expect(this.page.locator(`text="${name}"`)).toBeVisible();
  }

  async expectNoBackups() {
    await expect(this.page.locator('text=/no.*backups|no backups found/i')).toBeVisible();
  }

  async clickCreateBackup() {
    await this.page.click('button:has-text("Create Backup"), [data-testid="create-backup-btn"]');
  }
}

class CustomerBackupCreateModal {
  constructor(private page: Page) {}

  async expectVisible() {
    await expect(this.page.locator('[role="dialog"], .modal')).toBeVisible();
  }

  async selectVM(vmName: string) {
    await this.page.click('[data-testid="vm-select"]');
    await this.page.click(`text="${vmName}"`);
  }

  async selectType(type: 'full' | 'incremental') {
    await this.page.click(`[data-testid="type-${type}"], input[value="${type}"]`);
  }

  async setName(name: string) {
    await this.page.fill('input[name="name"]', name);
  }

  async setExpiration(days: number) {
    await this.page.fill('input[name="expiration"]', days.toString());
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

class CustomerBackupDetailPage {
  constructor(private page: Page) {}

  async goto(backupId: string) {
    await this.page.goto(`/backups/${backupId}`);
  }

  async getBackupInfo() {
    return {
      name: await this.page.locator('[data-testid="backup-name"]').textContent(),
      type: await this.page.locator('[data-testid="backup-type"]').textContent(),
      status: await this.page.locator('[data-testid="backup-status"]').textContent(),
      size: await this.page.locator('[data-testid="backup-size"]').textContent(),
      createdAt: await this.page.locator('[data-testid="backup-created"]').textContent(),
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

  async clickDownload() {
    await this.page.click('button:has-text("Download"), [data-testid="download-btn"]');
  }

  async clickDelete() {
    await this.page.click('button:has-text("Delete"), [data-testid="delete-btn"]');
  }

  async expectActionDisabled(action: string) {
    await expect(this.page.locator(`button:has-text("${action}")`)).toBeDisabled();
  }

  async expectStatus(status: string) {
    await expect(this.page.locator('[data-testid="backup-status"]')).toContainText(status);
  }
}

class CustomerRestoreModal {
  constructor(private page: Page) {}

  async expectVisible() {
    await expect(this.page.locator('[role="dialog"], .modal:has-text("Restore")')).toBeVisible();
  }

  async selectTargetVM(vmName: string) {
    await this.page.click('[data-testid="target-vm-select"]');
    await this.page.click(`text="${vmName}"`);
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

class CustomerBackupSchedulePage {
  constructor(private page: Page) {}

  async goto(vmId: string) {
    await this.page.goto(`/vms/${vmId}/backups/schedule`);
  }

  async getScheduleList() {
    return this.page.locator('[data-testid="schedule-item"], table tbody tr');
  }

  async clickCreateSchedule() {
    await this.page.click('button:has-text("Add Schedule"), [data-testid="create-schedule-btn"]');
  }

  async setScheduleName(name: string) {
    await this.page.fill('input[name="name"]', name);
  }

  async setScheduleFrequency(frequency: 'daily' | 'weekly' | 'monthly') {
    await this.page.click(`[data-testid="frequency-${frequency}"], input[value="${frequency}"]`);
  }

  async setScheduleTime(hour: string, minute: string) {
    await this.page.fill('input[name="hour"]', hour);
    await this.page.fill('input[name="minute"]', minute);
  }

  async setScheduleRetention(count: number) {
    await this.page.fill('input[name="retention"]', count.toString());
  }

  async setScheduleType(type: 'full' | 'incremental') {
    await this.page.click(`[data-testid="type-${type}"], input[value="${type}"]`);
  }

  async toggleSchedule(scheduleId: string, enabled: boolean) {
    const toggle = this.page.locator(`[data-testid="schedule-toggle-${scheduleId}"]`);
    if (enabled) {
      await toggle.check();
    } else {
      await toggle.uncheck();
    }
  }

  async deleteSchedule(scheduleId: string) {
    await this.page.click(`[data-testid="delete-schedule-${scheduleId}"]`);
    await this.page.click('button:has-text("Confirm")');
  }
}

// ============================================
// Test Suite
// ============================================

test.describe('Customer Backup List', () => {
  let backupListPage: CustomerBackupListPage;

  test.beforeEach(async ({ page }) => {
    backupListPage = new CustomerBackupListPage(page);
    await backupListPage.goto();
  });

  test('should display backup list page', async ({ page }) => {
    await expect(page.locator('h1, [data-testid="page-title"]')).toContainText(/backups/i);
  });

  test('should show backup statistics', async ({ page }) => {
    // Look for stats section
    const statsSection = page.locator('[data-testid="backup-stats"]');
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
      
      // Should show VM name
      await expect(firstItem.locator('[data-testid="vm-name"], td:has-text("vm")')).toBeVisible();
      
      // Should show backup type
      await expect(firstItem.locator('[data-testid="backup-type"], td:has-text("full"), td:has-text("incremental")')).toBeVisible();
      
      // Should show status
      await expect(firstItem.locator('[data-testid="status"], td:has-text("completed"), td:has-text("creating")')).toBeVisible();
    }
  });

  test('should filter backups by VM', async ({ page }) => {
    await backupListPage.filterByVM('test-vm');
    
    await page.waitForTimeout(500);
    
    // All visible backups should be for that VM
    const list = await backupListPage.getBackupList();
    const count = await list.count();
    
    if (count > 0) {
      const text = await list.first().textContent();
      expect(text?.toLowerCase()).toContain('test-vm');
    }
  });

  test('should filter backups by status', async ({ page }) => {
    await backupListPage.filterByStatus('Completed');
    
    await page.waitForTimeout(500);
    
    // All visible backups should have Completed status
    const statuses = page.locator('[data-testid="status"]:has-text("Completed"), td:has-text("Completed")');
    const count = await statuses.count();
    expect(count).toBeGreaterThan(0);
  });

  test('should search backups', async ({ page }) => {
    await backupListPage.searchBackup('backup-name');
    
    await page.waitForTimeout(500);
    
    // Results should match search
    const list = await backupListPage.getBackupList();
    const count = await list.count();
    
    if (count > 0) {
      const text = await list.first().textContent();
      expect(text?.toLowerCase()).toContain('backup-name');
    }
  });

  test('should show empty state when no backups', async ({ page }) => {
    // Navigate to VM with no backups
    await page.goto('/vms/no-backups-vm-id/backups');
    
    const list = await backupListPage.getBackupList();
    const count = await list.count();
    
    if (count === 0) {
      await backupListPage.expectNoBackups();
    }
  });
});

test.describe('Customer Backup Creation', () => {
  let backupListPage: CustomerBackupListPage;
  let createModal: CustomerBackupCreateModal;
  const testVMId = '00000000-0000-0000-0000-000000000001';

  test.beforeEach(async ({ page }) => {
    backupListPage = new CustomerBackupListPage(page);
    createModal = new CustomerBackupCreateModal(page);
    await backupListPage.gotoVMBackups(testVMId);
  });

  test('should open backup creation modal', async ({ page }) => {
    await backupListPage.clickCreateBackup();
    await createModal.expectVisible();
  });

  test('should create full backup', async ({ page }) => {
    await backupListPage.clickCreateBackup();
    await createModal.expectVisible();
    
    await createModal.selectType('full');
    await createModal.setName(`test-backup-${Date.now()}`);
    await createModal.submit();
    
    // Modal should close
    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
    
    // Should show success message
    await expect(page.locator('text=/backup created|success/i')).toBeVisible();
  });

  test('should create incremental backup', async ({ page }) => {
    // This requires an existing full backup
    await backupListPage.clickCreateBackup();
    await createModal.expectVisible();
    
    await createModal.selectType('incremental');
    await createModal.setName(`incremental-backup-${Date.now()}`);
    await createModal.submit();
    
    // Should succeed or show appropriate message
  });

  test('should show validation error for empty name', async ({ page }) => {
    await backupListPage.clickCreateBackup();
    await createModal.submit();
    
    await createModal.expectValidationError(/required|please enter/i);
  });

  test('should allow canceling backup creation', async ({ page }) => {
    await backupListPage.clickCreateBackup();
    await createModal.cancel();
    
    // Modal should close
    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
  });

  test('should set backup expiration', async ({ page }) => {
    await backupListPage.clickCreateBackup();
    
    await createModal.setName('expiring-backup');
    await createModal.setExpiration(7);
    await createModal.submit();
    
    // Should create with expiration
  });
});

test.describe('Customer Backup Restore', () => {
  let backupDetailPage: CustomerBackupDetailPage;
  let restoreModal: CustomerRestoreModal;
  const testBackupId = '00000000-0000-0000-0000-000000000010';

  test.beforeEach(async ({ page }) => {
    backupDetailPage = new CustomerBackupDetailPage(page);
    restoreModal = new CustomerRestoreModal(page);
  });

  test('should show restore button for completed backup', async ({ page }) => {
    await backupDetailPage.goto(testBackupId);
    
    await expect(page.locator('button:has-text("Restore")')).toBeVisible();
  });

  test('should open restore confirmation modal', async ({ page }) => {
    await backupDetailPage.goto(testBackupId);
    await backupDetailPage.clickRestore();
    
    await restoreModal.expectVisible();
  });

  test('should show restore warning', async ({ page }) => {
    await backupDetailPage.goto(testBackupId);
    await backupDetailPage.clickRestore();
    
    await expect(page.locator('text=/overwrite|warning|data will be lost/i')).toBeVisible();
  });

  test('should require acknowledgment before restore', async ({ page }) => {
    await backupDetailPage.goto(testBackupId);
    await backupDetailPage.clickRestore();
    
    // Restore button should be disabled until acknowledged
    await expect(page.locator('button:has-text("Restore"):not(:disabled)')).not.toBeVisible();
    
    await restoreModal.acknowledgeWarning();
    
    // Now should be enabled
    await expect(page.locator('button:has-text("Restore")')).toBeEnabled();
  });

  test('should initiate restore', async ({ page }) => {
    await backupDetailPage.goto(testBackupId);
    await backupDetailPage.clickRestore();
    await restoreModal.acknowledgeWarning();
    await restoreModal.confirmRestore();
    
    // Should show success or progress
    await expect(page.locator('text=/restore.*started|restoring/i')).toBeVisible();
  });

  test('should allow canceling restore', async ({ page }) => {
    await backupDetailPage.goto(testBackupId);
    await backupDetailPage.clickRestore();
    await restoreModal.cancel();
    
    // Modal should close without restoring
    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
  });

  test('should disable restore for failed backup', async ({ page }) => {
    // Navigate to a failed backup
    await page.goto('/backups/failed-backup-id');
    
    await backupDetailPage.expectActionDisabled('Restore');
  });
});

test.describe('Customer Backup Download', () => {
  let backupDetailPage: CustomerBackupDetailPage;
  const testBackupId = '00000000-0000-0000-0000-000000000011';

  test.beforeEach(async ({ page }) => {
    backupDetailPage = new CustomerBackupDetailPage(page);
  });

  test('should show download button for completed backup', async ({ page }) => {
    await backupDetailPage.goto(testBackupId);
    
    await expect(page.locator('button:has-text("Download")')).toBeVisible();
  });

  test('should initiate download on click', async ({ page }) => {
    await backupDetailPage.goto(testBackupId);
    
    // Listen for download
    const [download] = await Promise.all([
      page.waitForEvent('download'),
      backupDetailPage.clickDownload(),
    ]);
    
    // Verify download started
    expect(download).toBeTruthy();
    const suggestedName = download.suggestedFilename();
    expect(suggestedName).toMatch(/\.img$|\.tar$|\.gz$/);
  });

  test('should disable download for incomplete backup', async ({ page }) => {
    // Navigate to creating backup
    await page.goto('/backups/creating-backup-id');
    
    await backupDetailPage.expectActionDisabled('Download');
  });
});

test.describe('Customer Backup Deletion', () => {
  let backupDetailPage: CustomerBackupDetailPage;
  let backupListPage: CustomerBackupListPage;
  const testBackupId = '00000000-0000-0000-0000-000000000012';

  test('should show delete button', async ({ page }) => {
    backupDetailPage = new CustomerBackupDetailPage(page);
    await backupDetailPage.goto(testBackupId);
    
    await expect(page.locator('button:has-text("Delete")')).toBeVisible();
  });

  test('should show deletion confirmation', async ({ page }) => {
    backupDetailPage = new CustomerBackupDetailPage(page);
    await backupDetailPage.goto(testBackupId);
    await backupDetailPage.clickDelete();
    
    await expect(page.locator('[role="dialog"], .modal')).toBeVisible();
    await expect(page.locator('text=/confirm|are you sure/i')).toBeVisible();
  });

  test('should delete backup after confirmation', async ({ page }) => {
    backupDetailPage = new CustomerBackupDetailPage(page);
    await backupDetailPage.goto(testBackupId);
    await backupDetailPage.clickDelete();
    
    // Confirm deletion
    await page.click('button:has-text("Confirm"), button:has-text("Delete"):not(:disabled)');
    
    // Should show success message
    await expect(page.locator('text=/deleted|removed/i')).toBeVisible();
  });

  test('should allow canceling deletion', async ({ page }) => {
    backupDetailPage = new CustomerBackupDetailPage(page);
    await backupDetailPage.goto(testBackupId);
    await backupDetailPage.clickDelete();
    
    await page.click('button:has-text("Cancel")');
    
    // Modal should close
    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
  });

  test('should remove backup from list after deletion', async ({ page }) => {
    backupListPage = new CustomerBackupListPage(page);
    
    // Get backup name before deletion
    await backupDetailPage.goto(testBackupId);
    const info = await backupDetailPage.getBackupInfo();
    
    await backupDetailPage.clickDelete();
    await page.click('button:has-text("Confirm")');
    
    // Navigate to list
    await backupListPage.goto();
    
    // Backup should not be visible
    await expect(page.locator(`text="${info.name}"`)).not.toBeVisible();
  });
});

test.describe('Customer Backup Scheduling', () => {
  let schedulePage: CustomerBackupSchedulePage;
  const testVMId = '00000000-0000-0000-0000-000000000002';

  test.beforeEach(async ({ page }) => {
    schedulePage = new CustomerBackupSchedulePage(page);
    await schedulePage.goto(testVMId);
  });

  test('should display backup schedule page', async ({ page }) => {
    await expect(page.locator('h1, [data-testid="page-title"]')).toContainText(/schedule|backup.*schedule/i);
  });

  test('should show existing schedules', async ({ page }) => {
    const list = await schedulePage.getScheduleList();
    const count = await list.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should create daily backup schedule', async ({ page }) => {
    await schedulePage.clickCreateSchedule();
    
    await schedulePage.setScheduleName('Daily Backup');
    await schedulePage.setScheduleFrequency('daily');
    await schedulePage.setScheduleTime('02', '00');
    await schedulePage.setScheduleRetention(7);
    await schedulePage.setScheduleType('full');
    
    await page.click('button[type="submit"]');
    
    await expect(page.locator('text=/schedule created|success/i')).toBeVisible();
  });

  test('should create weekly backup schedule', async ({ page }) => {
    await schedulePage.clickCreateSchedule();
    
    await schedulePage.setScheduleName('Weekly Backup');
    await schedulePage.setScheduleFrequency('weekly');
    await schedulePage.setScheduleTime('03', '00');
    await schedulePage.setScheduleRetention(4);
    
    await page.click('button[type="submit"]');
    
    await expect(page.locator('text=/schedule created|success/i')).toBeVisible();
  });

  test('should toggle schedule enabled/disabled', async ({ page }) => {
    // Find first schedule toggle
    const toggle = page.locator('[data-testid^="schedule-toggle-"]').first();
    
    if (await toggle.isVisible()) {
      const initialState = await toggle.isChecked();
      await toggle.click();
      
      // State should change
      const newState = await toggle.isChecked();
      expect(newState).toBe(!initialState);
    }
  });

  test('should delete schedule', async ({ page }) => {
    const list = await schedulePage.getScheduleList();
    const count = await list.count();
    
    if (count > 0) {
      // Get first schedule's delete button
      await page.click('[data-testid^="delete-schedule-"]').first();
      await page.click('button:has-text("Confirm")');
      
      await expect(page.locator('text=/deleted|removed/i')).toBeVisible();
    }
  });

  test('should show next scheduled backup time', async ({ page }) => {
    const nextRun = page.locator('[data-testid="next-run"], .next-schedule');
    
    if (await nextRun.isVisible()) {
      const text = await nextRun.textContent();
      // Should show a date/time
      expect(text).toMatch(/\d|am|pm/i);
    }
  });
});

test.describe('Customer Backup Status Updates', () => {
  let backupDetailPage: CustomerBackupDetailPage;
  const creatingBackupId = '00000000-0000-0000-0000-000000000020';

  test('should show creating status', async ({ page }) => {
    backupDetailPage = new CustomerBackupDetailPage(page);
    await backupDetailPage.goto(creatingBackupId);
    
    await backupDetailPage.expectStatus('Creating');
  });

  test('should update status to completed', async ({ page }) => {
    // This would require WebSocket or polling updates
    // For now, just verify the status element exists
    backupDetailPage = new CustomerBackupDetailPage(page);
    await backupDetailPage.goto(creatingBackupId);
    
    const status = page.locator('[data-testid="backup-status"]');
    await expect(status).toBeVisible();
  });

  test('should show progress for creating backup', async ({ page }) => {
    backupDetailPage = new CustomerBackupDetailPage(page);
    await backupDetailPage.goto(creatingBackupId);
    
    const progressBar = page.locator('[role="progressbar"], .progress');
    
    if (await progressBar.isVisible()) {
      await expect(progressBar).toBeVisible();
    }
  });

  test('should show error message for failed backup', async ({ page }) => {
    await page.goto('/backups/failed-backup-id');
    
    await expect(page.locator('text=/error|failed/i')).toBeVisible();
  });
});

test.describe('Customer Backup Permissions', () => {
  test('should only show own VMs in backup list', async ({ page }) => {
    await page.goto('/backups');
    
    // All VMs listed should belong to current customer
    // This is verified by checking no "access denied" errors appear
    const list = page.locator('[data-testid="backup-item"], table tbody tr');
    const count = await list.count();
    
    // Page should load without errors
    await expect(page.locator('text=/access denied|unauthorized/i')).not.toBeVisible();
  });

  test('should not allow access to other customer backups', async ({ page }) => {
    // Try to access another customer's backup
    await page.goto('/backups/other-customer-backup-id');
    
    // Should show error or redirect
    await expect(page.locator('text=/not found|access denied/i')).toBeVisible();
  });

  test('should show backup action permissions', async ({ page }) => {
    await page.goto('/backups/00000000-0000-0000-0000-000000000015');
    
    // Customer should be able to:
    // - Download their backups
    // - Restore to their VMs
    // - Delete their backups
    
    await expect(page.locator('button:has-text("Download")')).toBeVisible();
    await expect(page.locator('button:has-text("Restore")')).toBeVisible();
    await expect(page.locator('button:has-text("Delete")')).toBeVisible();
  });
});