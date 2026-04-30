/**
 * Admin VM Management E2E Tests (Refactored)
 *
 * Uses the Page Object Model pattern for better maintainability.
 */

import { test, expect } from './fixtures';
import { TEST_IDS } from './utils/api';

const RUNNING_VM = 'test-vm-running';
const STOPPED_VM = 'test-vm-stopped';
const SUSPENDED_VM = 'test-vm-suspended';

// ============================================
// Admin VM List Tests
// ============================================

test.describe('Admin VM List', () => {
  test.use({ storageState: '.auth/admin-storage.json' });

  test('should display VM list page', async ({ adminVMListPage }) => {
    await adminVMListPage.goto();
    await adminVMListPage.expectTableVisible();
  });

  test('should show the current VM summary counts', async ({ adminVMListPage }) => {
    await adminVMListPage.goto();
    const summaryText = await adminVMListPage.getSummaryText();
    expect(summaryText).toMatch(/\d+ of \d+ VMs displayed/i);
  });

  test('should search VMs by hostname', async ({ adminVMListPage }) => {
    await adminVMListPage.goto();
    await adminVMListPage.searchVM(RUNNING_VM);
    await adminVMListPage.expectVMInList(RUNNING_VM);
    await adminVMListPage.expectVMNotInList(STOPPED_VM);
    await adminVMListPage.expectVMNotInList(SUSPENDED_VM);
  });

  test('should open the current create VM dialog', async ({ adminVMListPage, page }) => {
    await adminVMListPage.goto();
    await adminVMListPage.openCreateVMDialog();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Create Virtual Machine' })).toBeVisible();
    await expect(dialog.getByText('Create a new VM manually.')).toBeVisible();
    await expect(dialog.locator('#hostname')).toBeVisible();
    await expect(dialog.locator('#password')).toBeVisible();
  });

  test('should show current create VM validation messages', async ({ adminVMListPage, page }) => {
    await adminVMListPage.goto();
    await adminVMListPage.openCreateVMDialog();

    const dialog = page.getByRole('dialog');
    await expect(dialog.locator('#hostname')).toBeVisible();
    await dialog.getByRole('button', { name: 'Create VM' }).evaluate((button) => {
      (button as HTMLButtonElement).click();
    });

    await expect(dialog).toContainText('Must select a valid customer');
    await expect(dialog).toContainText('Must select a valid plan');
    await expect(dialog).toContainText('Must select a valid template');
    await expect(dialog).toContainText('Hostname is required');
    await expect(dialog).toContainText('Password must be at least 12 characters');
  });
});

// ============================================
// Admin VM Detail Tests
// ============================================

test.describe('Admin VM Detail', () => {
  test.use({ storageState: '.auth/admin-storage.json' });

  test('should open the current VM details dialog', async ({ adminVMListPage, page }) => {
    await adminVMListPage.goto();
    await adminVMListPage.openViewDialog(RUNNING_VM);

    const dialog = page.getByRole('dialog');
    await expect(dialog).toContainText(TEST_IDS.vms.testVM1);
    await expect(dialog).toContainText('running');
    await expect(dialog).toContainText('2');
    await expect(dialog).toContainText('2 GB');
    await expect(dialog).toContainText('50 GB');
  });

  test('should open the current edit dialog with seeded VM values', async ({ adminVMListPage, page }) => {
    await adminVMListPage.goto();
    await adminVMListPage.openEditDialog(RUNNING_VM);

    const dialog = page.getByRole('dialog');
    await expect(dialog.locator('#edit-hostname')).toHaveValue(RUNNING_VM);
    await expect(dialog.locator('#edit-vcpu')).toHaveValue('2');
    await expect(dialog.locator('#edit-memory_mb')).toHaveValue('2048');
    await expect(dialog.locator('#edit-disk_gb')).toHaveValue('50');
  });
});

// ============================================
// Admin VM Deletion Tests
// ============================================

test.describe('Admin VM Deletion', () => {
  test.use({ storageState: '.auth/admin-storage.json' });

  test('should show the current deletion confirmation dialog', async ({ adminVMListPage, page }) => {
    await adminVMListPage.goto();
    await adminVMListPage.openDeleteDialog(SUSPENDED_VM);

    const dialog = page.getByRole('dialog');
    await expect(dialog).toContainText(SUSPENDED_VM);
    await expect(dialog).toContainText('This action cannot be undone');
    await expect(dialog.getByRole('button', { name: 'Delete VM' })).toBeVisible();
  });
});
