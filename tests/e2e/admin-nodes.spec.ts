import type { Page } from '@playwright/test';
import { test, expect } from './fixtures';

const DRAINING_NODE = 'test-node-5';
const OFFLINE_NODE = 'test-node-4';
const FAILED_NODE = 'test-node-1';

function nodeRow(page: Page, hostname: string) {
  return page.locator('tbody tr').filter({ has: page.getByText(hostname, { exact: true }) });
}

async function gotoNodes(page: Page) {
  await page.goto('/nodes');
  await expect(page.getByRole('heading', { name: 'Nodes' })).toBeVisible();
  await expect(page.getByText('Manage hypervisor nodes and cluster capacity')).toBeVisible();
  await expect(page.getByPlaceholder('Search nodes...')).toBeVisible();
  await expect(nodeRow(page, DRAINING_NODE)).toBeVisible();
}

test.describe('Admin nodes', () => {
  test.use({ storageState: '.auth/admin-storage.json' });

  test('renders the current node management table', async ({ page }) => {
    await gotoNodes(page);

    await expect(page.getByRole('link', { name: 'View Failover Requests' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Add Node' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Node Name' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Status' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Location', exact: true })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'VMs' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'CPU Allocation' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'RAM Allocation' })).toBeVisible();
  });

  test('shows the seeded nodes with current status-dependent actions', async ({ page }) => {
    await gotoNodes(page);

    const drainingRow = nodeRow(page, DRAINING_NODE);
    await expect(drainingRow).toContainText('draining');
    await expect(drainingRow.getByRole('button', { name: 'View Details' })).toBeVisible();
    await expect(drainingRow.getByRole('button', { name: 'Edit Node' })).toBeVisible();
    await expect(drainingRow.getByRole('button', { name: 'Initiate Failover' })).toHaveCount(0);

    const offlineRow = nodeRow(page, OFFLINE_NODE);
    await expect(offlineRow).toContainText('offline');
    await expect(offlineRow.getByRole('button', { name: 'Initiate Failover' })).toBeVisible();

    const failedRow = nodeRow(page, FAILED_NODE);
    await expect(failedRow).toContainText('failed');
    await expect(failedRow).toContainText('6 / 32 Cores');
    await expect(failedRow).toContainText('6 / 64 GB');
  });

  test('filters the visible nodes with the current search box', async ({ page }) => {
    await gotoNodes(page);
    await page.getByPlaceholder('Search nodes...').fill('node-4');

    await expect(nodeRow(page, OFFLINE_NODE)).toBeVisible();
    await expect(nodeRow(page, DRAINING_NODE)).toHaveCount(0);
  });

  test('shows the current add-node dialog', async ({ page }) => {
    await gotoNodes(page);
    await page.getByRole('button', { name: 'Add Node' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Register New Node' })).toBeVisible();
    await expect(dialog.getByText('Basic Information')).toBeVisible();
    await expect(dialog.getByText('Resources')).toBeVisible();
    await expect(dialog.getByText('IPMI Configuration (Optional)')).toBeVisible();
    await expect(dialog.locator('input#hostname')).toBeVisible();
    await expect(dialog.locator('input#grpc_address')).toBeVisible();
    await expect(dialog.locator('input#management_ip')).toBeVisible();
    await expect(dialog.getByRole('button', { name: 'Register Node' })).toBeVisible();
  });

  test('shows add-node validation for the current required fields', async ({ page }) => {
    await gotoNodes(page);
    await page.getByRole('button', { name: 'Add Node' }).click();

    const dialog = page.getByRole('dialog');
    await dialog.locator('form').evaluate((form) => {
      (form as HTMLFormElement).requestSubmit();
    });

    await expect(dialog.getByText('Hostname is required')).toBeVisible();
    await expect(dialog.getByText('gRPC address is required')).toBeVisible();
    await expect(dialog.getByText('Management IP is required')).toBeVisible();
  });

  test('validates management IP format in the add-node dialog', async ({ page }) => {
    await gotoNodes(page);
    await page.getByRole('button', { name: 'Add Node' }).click();

    const dialog = page.getByRole('dialog');
    await dialog.locator('input#hostname').fill('node-validation.example.test');
    await dialog.locator('input#grpc_address').fill('10.0.0.200:50051');
    await dialog.locator('input#management_ip').fill('invalid-ip');
    await dialog.locator('form').evaluate((form) => {
      (form as HTMLFormElement).requestSubmit();
    });

    await expect(dialog.getByText(/Must be a valid IP address/i)).toBeVisible();
  });

  test('opens the detail dialog for a seeded node', async ({ page }) => {
    await gotoNodes(page);
    await nodeRow(page, DRAINING_NODE).getByRole('button', { name: 'View Details' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: `Node Details: ${DRAINING_NODE}` })).toBeVisible();
    await expect(dialog).toContainText('draining');
    await expect(dialog).toContainText('10.0.0.105:50051');
    await expect(dialog).toContainText('10.0.0.105');
    await expect(dialog).toContainText('Resources');
    await expect(dialog).toContainText('Storage');

    await dialog.getByRole('button', { name: 'Close' }).first().evaluate((button) => {
      (button as HTMLButtonElement).click();
    });
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('opens the edit dialog with the current node values', async ({ page }) => {
    await gotoNodes(page);
    await nodeRow(page, FAILED_NODE).getByRole('button', { name: 'Edit Node' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: `Edit Node: ${FAILED_NODE}` })).toBeVisible();
    await expect(dialog.locator('input[disabled]')).toHaveValue(FAILED_NODE);
    await expect(dialog.locator('input#edit-grpc_address')).toHaveValue('10.0.0.101:50051');
    await expect(dialog.locator('input#edit-total_vcpu')).toHaveValue('32');
    await expect(dialog.locator('input#edit-total_memory_mb')).toHaveValue('65536');

    await dialog.getByRole('button', { name: 'Cancel' }).evaluate((button) => {
      (button as HTMLButtonElement).click();
    });
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('opens and closes the failover confirmation dialog for an offline node', async ({ page }) => {
    await gotoNodes(page);
    await nodeRow(page, OFFLINE_NODE).getByRole('button', { name: 'Initiate Failover' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Initiate Failover' })).toBeVisible();
    await expect(dialog).toContainText(OFFLINE_NODE);
    await expect(dialog.getByRole('button', { name: 'Confirm Failover' })).toBeVisible();

    await dialog.getByRole('button', { name: 'Cancel' }).evaluate((button) => {
      (button as HTMLButtonElement).click();
    });
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });
});
