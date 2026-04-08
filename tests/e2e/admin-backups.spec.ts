import { expect, Locator, Page, test } from '@playwright/test';

function scheduleName() {
  return `E2E schedule ${Date.now()}`;
}

async function gotoBackupSchedules(page: Page) {
  await page.goto('/backup-schedules');
  await expect(page.getByRole('heading', { name: 'Backup Schedules', exact: true })).toBeVisible();
}

async function openCreateSchedule(page: Page) {
  const createButton = page.getByRole('button', { name: /New Schedule|Create Schedule/i }).first();
  await createButton.click();
  await expect(page.getByText(/Create Backup Schedule/i)).toBeVisible();
}

function createScheduleDialog(page: Page): Locator {
  return page.getByRole('dialog').filter({ hasText: 'Create Backup Schedule' });
}

function scheduleRow(page: Page, name: string): Locator {
  return page.locator('table tbody tr').filter({ hasText: name });
}

test.describe('Admin backup schedules', () => {
  test('renders the backup schedules page', async ({ page }) => {
    await gotoBackupSchedules(page);

    await expect(page.getByText('Backup Schedule Directory')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Backup Schedules', exact: true })).toBeVisible();
    await expect(page.getByPlaceholder('Search schedules or targets')).toBeVisible();
  });

  test('shows the current modal validation rules', async ({ page }) => {
    await gotoBackupSchedules(page);
    await openCreateSchedule(page);
    const dialog = createScheduleDialog(page);
    const submitButton = dialog.getByRole('button', { name: 'Create Schedule' });
    await submitButton.evaluate((element: HTMLElement) => {
      element.click();
    });

    await expect(page.getByText('Name is required')).toBeVisible();
    await expect(page.getByText(/Select at least one target or enable/i)).toBeVisible();
  });

  test('creates and deletes a schedule by targeting all VMs', async ({ page }) => {
    const name = scheduleName();

    await gotoBackupSchedules(page);
    await openCreateSchedule(page);
    const dialog = createScheduleDialog(page);
    await dialog.locator('input#schedule-name').fill(name);
    await dialog.locator('textarea#schedule-description').fill('Created by the Playwright E2E suite.');

    const targetAllToggle = dialog.getByLabel('Target all VMs');
    await targetAllToggle.evaluate((element: HTMLElement) => {
      element.click();
    });

    const submitButton = dialog.getByRole('button', { name: 'Create Schedule' });
    await submitButton.evaluate((element: HTMLElement) => {
      element.click();
    });

    await expect(page.getByText(/Schedule created/i)).toBeVisible();
    await expect(scheduleRow(page, name)).toBeVisible();

    await scheduleRow(page, name).locator('button').nth(2).click();
    await page.getByRole('button', { name: 'Delete Schedule' }).evaluate((element: HTMLElement) => {
      element.click();
    });

    await expect(page.getByText(/Schedule deleted/i)).toBeVisible();
    await expect(scheduleRow(page, name)).toHaveCount(0);
  });
});
