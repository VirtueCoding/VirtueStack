import type { Page } from '@playwright/test';
import { test, expect } from './fixtures';
import { TEST_IDS } from './utils/api';

const CUSTOMER_VM_ID = process.env.TEST_CUSTOMER_VM_ID || TEST_IDS.vms.testVM1;
const CUSTOMER_VM_HOSTNAME = 'test-vm-running';
const SEEDED_SNAPSHOT_NAME = 'Pre-upgrade Snapshot';
const SEEDED_FULL_BACKUP_NAME = 'Daily Full Backup';

function backupRow(page: Page, name: string) {
  return page.locator('div.flex.items-center.justify-between.rounded-lg.border.p-4').filter({ hasText: name });
}

async function gotoSnapshotFilter(page: Page) {
  await page.goto(`/vms/${CUSTOMER_VM_ID}`);
  await expect(page.getByRole('heading', { name: CUSTOMER_VM_HOSTNAME })).toBeVisible();

  await page.getByRole('tab', { name: /Backups & Snapshots/i }).click();
  await expect(page.getByText('Manage full backups and point-in-time snapshots')).toBeVisible();

  await page.getByRole('tab', { name: 'Snapshots', exact: true }).click();
  await expect(backupRow(page, SEEDED_SNAPSHOT_NAME)).toBeVisible();
}

test.describe('Customer VM snapshots', () => {
  test.use({ storageState: '.auth/customer-storage.json' });

  test('renders the snapshot filter within the backups manager', async ({ page }) => {
    await gotoSnapshotFilter(page);

    await expect(page.getByRole('tab', { name: 'All' })).toBeVisible();
    await expect(page.getByRole('tab', { name: 'Full Backups' })).toBeVisible();
    await expect(page.getByRole('tab', { name: 'Snapshots', exact: true })).toBeVisible();
  });

  test('shows the seeded snapshot row with snapshot-specific labels', async ({ page }) => {
    await gotoSnapshotFilter(page);

    const row = backupRow(page, SEEDED_SNAPSHOT_NAME);
    await expect(row).toContainText('Snapshot');
    await expect(row.getByRole('button', { name: 'Restore' })).toBeVisible();
  });

  test('hides full backups when the snapshot filter is active', async ({ page }) => {
    await gotoSnapshotFilter(page);

    await expect(backupRow(page, SEEDED_SNAPSHOT_NAME)).toBeVisible();
    await expect(backupRow(page, SEEDED_FULL_BACKUP_NAME)).toHaveCount(0);
  });

  test('opens and closes the restore snapshot dialog', async ({ page }) => {
    await gotoSnapshotFilter(page);
    await backupRow(page, SEEDED_SNAPSHOT_NAME).getByRole('button', { name: 'Restore' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Restore Snapshot' })).toBeVisible();
    await expect(dialog).toContainText(SEEDED_SNAPSHOT_NAME);

    await dialog.getByRole('button', { name: 'Cancel' }).click();
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('opens and closes the delete snapshot dialog', async ({ page }) => {
    await gotoSnapshotFilter(page);
    await backupRow(page, SEEDED_SNAPSHOT_NAME).locator('button').last().click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Delete Snapshot' })).toBeVisible();
    await expect(dialog).toContainText(SEEDED_SNAPSHOT_NAME);

    await dialog.getByRole('button', { name: 'Cancel' }).click();
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });
});
