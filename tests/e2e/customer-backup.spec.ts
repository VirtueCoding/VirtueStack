import { expect, Page, test } from '@playwright/test';
import { TEST_IDS } from './utils/api';

const CUSTOMER_VM_ID = process.env.TEST_CUSTOMER_VM_ID || TEST_IDS.vms.testVM1;

async function gotoBackupsTab(page: Page) {
  await page.goto(`/vms/${CUSTOMER_VM_ID}`);
  await expect(page.getByRole('button', { name: 'Back' })).toBeVisible();
  await expect(page.getByRole('tab', { name: /Backups & Snapshots/i })).toBeVisible();
  await page.getByRole('tab', { name: /Backups & Snapshots/i }).click();
  await expect(page.getByText('Backups & Snapshots').nth(0)).toBeVisible();
}

test.describe('Customer VM backups', () => {
  test('renders the current backups tab', async ({ page }) => {
    await gotoBackupsTab(page);

    await expect(page.getByRole('button', { name: 'Create Backup' })).toBeVisible();
    await expect(page.getByRole('tab', { name: 'All' })).toBeVisible();
    await expect(page.getByRole('tab', { name: 'Full Backups' })).toBeVisible();
    await expect(page.getByRole('tab', { name: 'Snapshots', exact: true })).toBeVisible();
  });

  test('opens and closes the create-backup dialog', async ({ page }) => {
    await gotoBackupsTab(page);
    await page.getByRole('button', { name: 'Create Backup' }).click();

    await expect(page.getByRole('heading', { name: 'Create Backup' })).toBeVisible();
    await expect(page.locator('input#backup-name')).toHaveValue(/Backup /);
    await page.getByRole('button', { name: 'Cancel' }).click();

    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('shows either backup rows or the current empty state', async ({ page }) => {
    await gotoBackupsTab(page);

    const rows = page.locator('div.flex.items-center.justify-between.rounded-lg.border.p-4');
    const emptyState = page.getByText(/No backups found|No full backups found|No snapshots found/i);

    if (await rows.count()) {
      await expect(rows.first()).toBeVisible();
      return;
    }

    await expect(emptyState).toBeVisible();
  });

  test('opens the restore dialog for a completed backup when one is available', async ({ page }) => {
    await gotoBackupsTab(page);

    const enabledRestoreButtons = page.locator('button:has-text("Restore"):not(:disabled)');
    const enabledCount = await enabledRestoreButtons.count();

    test.skip(enabledCount === 0, 'No completed backups are available in the seeded data.');

    await enabledRestoreButtons.first().click();
    await expect(page.getByRole('heading', { name: /Restore Backup|Restore Snapshot/i })).toBeVisible();
    await page.getByRole('button', { name: 'Cancel' }).click();
  });
});
