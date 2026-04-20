import type { Page } from '@playwright/test';
import { test, expect } from './fixtures';

const PUBLIC_IP_SET_NAME = 'Test Public IPv4';
const PRIVATE_IP_SET_NAME = 'Test Private IPv4';

function ipSetRow(page: Page, name: string) {
  return page.locator('tbody tr').filter({ has: page.getByText(name, { exact: true }) });
}

function statLabel(page: Page, label: string) {
  return page.locator('p').filter({ hasText: new RegExp(`^${label}$`) });
}

async function gotoIPSets(page: Page) {
  await page.goto('/ip-sets');
  await expect(page.getByRole('heading', { name: 'IP Address Pools' })).toBeVisible();
  await expect(page.getByText('Manage IP address sets and allocations')).toBeVisible();
  await expect(page.getByPlaceholder('Search by name or location...')).toBeVisible();
  await expect(ipSetRow(page, PUBLIC_IP_SET_NAME)).toBeVisible();
}

test.describe('Admin IP sets', () => {
  test.use({ storageState: '.auth/admin-storage.json' });

  test('renders the current IP pools dashboard', async ({ page }) => {
    await gotoIPSets(page);

    await expect(statLabel(page, 'Total Sets')).toBeVisible();
    await expect(statLabel(page, 'Total IPs')).toBeVisible();
    await expect(statLabel(page, 'Available IPs')).toBeVisible();
    await expect(statLabel(page, 'IPv4 / IPv6 Sets')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Create IP Set' })).toBeVisible();
  });

  test('shows the seeded IP sets with CIDR and usage data', async ({ page }) => {
    await gotoIPSets(page);

    const publicRow = ipSetRow(page, PUBLIC_IP_SET_NAME);
    await expect(publicRow).toContainText('IPv4');
    await expect(publicRow).toContainText('192.0.2.0/24');
    await expect(publicRow).toContainText(/used/i);

    const privateRow = ipSetRow(page, PRIVATE_IP_SET_NAME);
    await expect(privateRow).toContainText('10.10.0.0/24');
  });

  test('filters the visible IP sets with the search input', async ({ page }) => {
    await gotoIPSets(page);
    await page.getByPlaceholder('Search by name or location...').fill('Private');

    await expect(ipSetRow(page, PRIVATE_IP_SET_NAME)).toBeVisible();
    await expect(ipSetRow(page, PUBLIC_IP_SET_NAME)).toHaveCount(0);
  });

  test('shows create-dialog validation for required fields', async ({ page }) => {
    await gotoIPSets(page);
    await page.getByRole('button', { name: 'Create IP Set' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Create IP Set' })).toBeVisible();
    await dialog.locator('form').evaluate((form) => {
      (form as HTMLFormElement).requestSubmit();
    });

    await expect(dialog.getByText('Name is required')).toBeVisible();
    await expect(dialog.getByText('Network CIDR is required')).toBeVisible();
    await expect(dialog.getByText('Gateway is required')).toBeVisible();
  });

  test('validates CIDR format in the create dialog', async ({ page }) => {
    await gotoIPSets(page);
    await page.getByRole('button', { name: 'Create IP Set' }).click();

    const dialog = page.getByRole('dialog');
    await dialog.locator('input#name').fill('Validation Pool');
    await dialog.locator('input#network').fill('invalid-cidr');
    await dialog.locator('input#gateway').fill('192.0.2.1');
    await dialog.locator('form').evaluate((form) => {
      (form as HTMLFormElement).requestSubmit();
    });

    await expect(dialog.getByText(/Invalid IPv4 CIDR format/i)).toBeVisible();
  });

  test('opens the detail dialog for a seeded IP set', async ({ page }) => {
    await gotoIPSets(page);
    await ipSetRow(page, PUBLIC_IP_SET_NAME).getByRole('button', { name: 'View' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'IP Set Details' })).toBeVisible();
    await expect(dialog).toContainText(PUBLIC_IP_SET_NAME);
    await expect(dialog).toContainText('192.0.2.0/24');
    await expect(dialog).toContainText('192.0.2.1');
    await expect(dialog.getByRole('button', { name: 'Edit IP Set' })).toBeVisible();

    await dialog.getByRole('button', { name: 'Close' }).first().evaluate((button) => {
      (button as HTMLButtonElement).click();
    });
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('opens the edit dialog with current IP set values', async ({ page }) => {
    await gotoIPSets(page);
    await ipSetRow(page, PUBLIC_IP_SET_NAME).getByRole('button', { name: 'Edit' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByText(`Edit IP Set: ${PUBLIC_IP_SET_NAME}`)).toBeVisible();
    await expect(dialog.locator('input#edit-name')).toHaveValue(PUBLIC_IP_SET_NAME);
    await expect(dialog.locator('input#edit-gateway')).toHaveValue('192.0.2.1');
    await expect(dialog.getByText('192.0.2.0/24')).toBeVisible();

    await dialog.getByRole('button', { name: 'Cancel' }).evaluate((button) => {
      (button as HTMLButtonElement).click();
    });
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });
});
