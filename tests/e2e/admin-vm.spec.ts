import { test, expect, Page } from '@playwright/test';
import { TEST_IDS } from './utils/api';

const RUNNING_VM = 'test-vm-running';
const STOPPED_VM = 'test-vm-stopped';
const SUSPENDED_VM = 'test-vm-suspended';

function vmRow(page: Page, name: string) {
  return page.locator('tbody tr').filter({ has: page.getByText(name, { exact: true }) });
}

async function gotoVMs(page: Page) {
  await page.goto('/vms');
  await expect(page.getByRole('heading', { name: 'Virtual Machines' })).toBeVisible();
  await expect(page.getByText('Manage all virtual machines across the cluster')).toBeVisible();
  await expect(page.getByPlaceholder('Search VMs by ID, name, status...')).toBeVisible();
  await expect(vmRow(page, RUNNING_VM)).toBeVisible();
}

async function openCreateDialog(page: Page) {
  await page.getByRole('button', { name: 'Create VM' }).click();
  await expect(page.getByRole('dialog').getByRole('heading', { name: 'Create Virtual Machine' })).toBeVisible();
}

test.describe('Admin VM management', () => {
  test.use({ storageState: '.auth/admin-storage.json' });

  test('renders the current VM page and table', async ({ page }) => {
    await gotoVMs(page);

    await expect(page.getByText('All VMs')).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Name' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Status' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Customer ID' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Node ID' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Created' })).toBeVisible();
  });

  test('shows the current VM summary counts', async ({ page }) => {
    await gotoVMs(page);
    await expect(page.getByText(/\d+ of \d+ VMs displayed/i)).toBeVisible();
  });

  test('filters the visible VMs with the current search input', async ({ page }) => {
    await gotoVMs(page);
    await page.getByPlaceholder('Search VMs by ID, name, status...').fill(RUNNING_VM);

    await expect(vmRow(page, RUNNING_VM)).toBeVisible();
    await expect(vmRow(page, STOPPED_VM)).toHaveCount(0);
    await expect(vmRow(page, SUSPENDED_VM)).toHaveCount(0);
  });

  test('shows the current create VM dialog fields', async ({ page }) => {
    await gotoVMs(page);
    await openCreateDialog(page);

    const dialog = page.getByRole('dialog');
    await expect(dialog.locator('#hostname')).toBeVisible();
    await expect(dialog.locator('#password')).toBeVisible();
    await expect(dialog.getByRole('button', { name: 'Generate' })).toBeVisible();
    await expect(dialog.getByRole('combobox').filter({ hasText: 'Select a customer' })).toBeVisible();
    await expect(dialog.getByRole('combobox').filter({ hasText: /^Select a plan$/ })).toBeVisible();
    await expect(dialog.getByRole('combobox').filter({ hasText: 'Select a template' })).toBeVisible();
  });

  test('shows current create VM validation messages', async ({ page }) => {
    await gotoVMs(page);
    await openCreateDialog(page);

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

  test('opens the current VM details dialog for a seeded VM', async ({ page }) => {
    await gotoVMs(page);
    await vmRow(page, RUNNING_VM).locator('button[title="View Details"]').click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Virtual Machine Details' })).toBeVisible();
    await expect(dialog).toContainText(TEST_IDS.vms.testVM1);
    await expect(dialog).toContainText('running');
    await expect(dialog).toContainText('2 GB');
    await expect(dialog).toContainText('50 GB');
  });

  test('opens the current edit dialog with seeded VM values', async ({ page }) => {
    await gotoVMs(page);
    await vmRow(page, RUNNING_VM).locator('button[title="Edit VM"]').click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: `Edit VM: ${RUNNING_VM}` })).toBeVisible();
    await expect(dialog.locator('#edit-hostname')).toHaveValue(RUNNING_VM);
    await expect(dialog.locator('#edit-vcpu')).toHaveValue('2');
    await expect(dialog.locator('#edit-memory_mb')).toHaveValue('2048');
    await expect(dialog.locator('#edit-disk_gb')).toHaveValue('50');
  });

  test('shows the current delete confirmation dialog', async ({ page }) => {
    await gotoVMs(page);
    await vmRow(page, SUSPENDED_VM).locator('button[title="Delete VM"]').click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Delete Virtual Machine' })).toBeVisible();
    await expect(dialog).toContainText(SUSPENDED_VM);
    await expect(dialog).toContainText('This action cannot be undone');
    await expect(dialog.getByRole('button', { name: 'Delete VM' })).toBeVisible();
  });
});
