import type { Page } from '@playwright/test';
import { test, expect } from './fixtures';

const PRIMARY_CUSTOMER_EMAIL = 'customer@test.virtuestack.local';
const PRIMARY_CUSTOMER_NAME = 'Test Customer';
const TWO_FACTOR_CUSTOMER_EMAIL = '2fa-customer@test.virtuestack.local';
const SUSPENDED_CUSTOMER_EMAIL = 'suspended@test.virtuestack.local';
const SUSPENDED_CUSTOMER_NAME = 'Suspended Customer';

function customerRow(page: Page, email: string) {
  return page.locator('tbody tr').filter({ has: page.getByText(email, { exact: true }) });
}

async function gotoCustomers(page: Page) {
  await page.goto('/customers');
  await expect(page.getByRole('heading', { name: 'Customers' })).toBeVisible();
  await expect(page.getByText('Manage client accounts and subscriptions')).toBeVisible();
  await expect(page.getByPlaceholder('Search customers...')).toBeVisible();
  await expect(customerRow(page, PRIMARY_CUSTOMER_EMAIL)).toBeVisible();
}

test.describe('Admin customers', () => {
  test.use({ storageState: '.auth/admin-storage.json' });

  test('renders the current customer directory page', async ({ page }) => {
    await gotoCustomers(page);

    await expect(page.getByText('Customer Directory')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Add Customer' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Customer' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Status' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'VMs' })).toBeVisible();
  });

  test('shows the seeded customers with status and action buttons', async ({ page }) => {
    await gotoCustomers(page);

    const primaryRow = customerRow(page, PRIMARY_CUSTOMER_EMAIL);
    await expect(primaryRow).toContainText(PRIMARY_CUSTOMER_NAME);
    await expect(primaryRow).toContainText(/active/i);
    await expect(primaryRow.getByRole('button', { name: 'View Profile' })).toBeVisible();
    await expect(primaryRow.getByRole('button', { name: 'Edit Customer' })).toBeVisible();
    await expect(primaryRow.getByRole('button', { name: 'Suspend Account' })).toBeVisible();

    const suspendedRow = customerRow(page, SUSPENDED_CUSTOMER_EMAIL);
    await expect(suspendedRow).toContainText(SUSPENDED_CUSTOMER_NAME);
    await expect(suspendedRow).toContainText(/suspended/i);
    await expect(suspendedRow.getByRole('button', { name: 'Unsuspend Account' })).toBeVisible();
  });

  test('filters the table with the customer search input', async ({ page }) => {
    await gotoCustomers(page);
    await page.getByPlaceholder('Search customers...').fill('2fa-customer');

    await expect(customerRow(page, TWO_FACTOR_CUSTOMER_EMAIL)).toBeVisible();
    await expect(customerRow(page, PRIMARY_CUSTOMER_EMAIL)).toHaveCount(0);
  });

  test('shows the current create-customer validation messages', async ({ page }) => {
    await gotoCustomers(page);
    await page.getByRole('button', { name: 'Add Customer' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Create New Customer' })).toBeVisible();
    await dialog.locator('form').evaluate((form) => {
      (form as HTMLFormElement).requestSubmit();
    });

    await expect(dialog.getByText('Name is required')).toBeVisible();
    await expect(dialog.getByText('Invalid email address')).toBeVisible();
    await expect(dialog.getByText('Password must be at least 8 characters')).toBeVisible();
  });

  test('opens the edit dialog with the current seeded customer data', async ({ page }) => {
    await gotoCustomers(page);
    await customerRow(page, PRIMARY_CUSTOMER_EMAIL).getByRole('button', { name: 'Edit Customer' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Edit Customer' })).toBeVisible();
    await expect(dialog.locator('input#edit-name')).toHaveValue(PRIMARY_CUSTOMER_NAME);
    await expect(dialog.locator('input#edit-email')).toHaveValue(PRIMARY_CUSTOMER_EMAIL);
    await expect(dialog.locator('input#edit-email')).toBeDisabled();
    await expect(dialog.getByRole('button', { name: 'Save Changes' })).toBeVisible();

    await dialog.getByRole('button', { name: 'Cancel' }).evaluate((button) => {
      (button as HTMLButtonElement).click();
    });
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('opens and closes the suspend confirmation dialog', async ({ page }) => {
    await gotoCustomers(page);
    await customerRow(page, PRIMARY_CUSTOMER_EMAIL).getByRole('button', { name: 'Suspend Account' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Suspend Customer' })).toBeVisible();
    await expect(dialog).toContainText(PRIMARY_CUSTOMER_NAME);

    await dialog.getByRole('button', { name: 'Cancel' }).evaluate((button) => {
      (button as HTMLButtonElement).click();
    });
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('opens and closes the unsuspend confirmation dialog', async ({ page }) => {
    await gotoCustomers(page);
    await customerRow(page, SUSPENDED_CUSTOMER_EMAIL).getByRole('button', { name: 'Unsuspend Account' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Unsuspend Customer' })).toBeVisible();
    await expect(dialog).toContainText(SUSPENDED_CUSTOMER_NAME);

    await dialog.getByRole('button', { name: 'Cancel' }).evaluate((button) => {
      (button as HTMLButtonElement).click();
    });
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('opens and closes the delete confirmation dialog', async ({ page }) => {
    await gotoCustomers(page);
    await customerRow(page, PRIMARY_CUSTOMER_EMAIL).getByRole('button', { name: 'Delete Account' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Delete Customer' })).toBeVisible();
    await expect(dialog).toContainText(PRIMARY_CUSTOMER_NAME);
    await expect(dialog.getByRole('button', { name: 'Confirm Delete' })).toBeVisible();

    await dialog.getByRole('button', { name: 'Cancel' }).evaluate((button) => {
      (button as HTMLButtonElement).click();
    });
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });
});
