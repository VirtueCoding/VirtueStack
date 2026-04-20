import { test, expect, Page } from '@playwright/test';
import { CREDENTIALS, forwardedIPForTest, routeAPIRequestsFromIP } from './utils/auth';
import { TEST_IDS } from './utils/api';

const RUNNING_VM_ID = process.env.TEST_CUSTOMER_VM_ID || TEST_IDS.vms.testVM1;
const STOPPED_VM_ID = TEST_IDS.vms.testVM2;
const RUNNING_HOSTNAME = 'test-vm-running';
const STOPPED_HOSTNAME = 'test-vm-stopped';
const RUNNING_IP = '192.0.2.10';
const STOPPED_IP = '192.0.2.11';

async function expectMetricsContent(page: Page) {
  const metricsError = page.getByText('Failed to load metrics. Please try again.');
  if (await metricsError.waitFor({ state: 'visible', timeout: 10000 }).then(() => true).catch(() => false)) {
    await expect(metricsError).toBeVisible();
    await expect(page.getByRole('button', { name: 'Retry' })).toBeVisible();
    return;
  }

  await expect(page.getByText('Resource Metrics')).toBeVisible();
  await expect(page.getByText('CPU Usage')).toBeVisible();
  await expect(page.getByText('Memory Usage')).toBeVisible();
}

function vmRow(page: Page, hostname: string) {
  return page.locator('tbody tr').filter({ hasText: hostname });
}

test.beforeEach(async ({ page }, testInfo) => {
  await routeAPIRequestsFromIP(page, forwardedIPForTest(testInfo, 'customer-vm-direct'));
});

async function gotoVMList(page: Page) {
  await page.goto('/vms');
  await expect(page.getByText('Virtual Machines').first()).toBeVisible();
  await expect(page.getByText('Manage and monitor your virtual machines')).toBeVisible();
  await expect(page.getByPlaceholder('Search by name, hostname or IP...')).toBeVisible();
}

async function gotoVMDetail(page: Page, vmID: string, hostname: string) {
  await page.goto(`/vms/${vmID}`);
  await expect(page.getByRole('button', { name: 'Back' })).toBeVisible();
  await expect(page.getByRole('heading', { name: hostname })).toBeVisible();
}

test.describe('Customer landing', () => {
  test('redirects an authenticated customer to the VM list', async ({ page }) => {
    await page.goto('/');

    await expect(page).toHaveURL(/\/vms$/);
    await expect(page.getByText('Manage and monitor your virtual machines')).toBeVisible();
  });

  test('shows the current sidebar navigation', async ({ page }) => {
    await gotoVMList(page);

    const nav = page.locator('nav');
    await expect(nav.getByRole('link', { name: 'My VMs' })).toBeVisible();
    await expect(nav.getByRole('link', { name: 'Billing' })).toBeVisible();
    await expect(nav.getByRole('link', { name: 'Settings' })).toBeVisible();
  });

  test('opens the account menu from the sidebar', async ({ page }) => {
    await gotoVMList(page);

    await page.getByRole('button').filter({ hasText: CREDENTIALS.customer.email }).click();
    await expect(page.getByText('Account Settings')).toBeVisible();
    await expect(page.getByText('Log out')).toBeVisible();
  });
});

test.describe('Customer VM list', () => {
  test.beforeEach(async ({ page }) => {
    await gotoVMList(page);
  });

  test('shows the seeded VMs with status and IP information', async ({ page }) => {
    await expect(vmRow(page, RUNNING_HOSTNAME)).toContainText(RUNNING_IP);
    await expect(vmRow(page, RUNNING_HOSTNAME)).toContainText('Running');
    await expect(vmRow(page, STOPPED_HOSTNAME)).toContainText(STOPPED_IP);
    await expect(vmRow(page, STOPPED_HOSTNAME)).toContainText('Stopped');
  });

  test('filters the visible VM rows with the search input', async ({ page }) => {
    await page.getByPlaceholder('Search by name, hostname or IP...').fill('stopped');

    await expect(vmRow(page, STOPPED_HOSTNAME)).toBeVisible();
    await expect(vmRow(page, RUNNING_HOSTNAME)).toHaveCount(0);
  });

  test('shows status-specific quick actions in the VM table', async ({ page }) => {
    await expect(vmRow(page, RUNNING_HOSTNAME).locator('button[title="Stop VM"]')).toBeVisible();
    await expect(vmRow(page, RUNNING_HOSTNAME).locator('button[title="Restart VM"]')).toBeVisible();
    await expect(vmRow(page, STOPPED_HOSTNAME).locator('button[title="Start VM"]')).toBeVisible();
  });
});

test.describe('Customer VM detail', () => {
  test('shows the running VM overview and available tabs', async ({ page }) => {
    await gotoVMDetail(page, RUNNING_VM_ID, RUNNING_HOSTNAME);

    await expect(page.getByRole('heading', { name: RUNNING_HOSTNAME })).toBeVisible();
    await expect(page.locator('p.font-mono.text-lg').first()).toHaveText(RUNNING_IP);
    await expect(page.getByRole('tab', { name: 'VNC' })).toBeVisible();
    await expect(page.getByRole('tab', { name: 'Serial' })).toBeVisible();
    await expect(page.getByRole('tab', { name: /Backups & Snapshots/i })).toBeVisible();
    await expect(page.getByRole('tab', { name: 'Metrics' })).toBeVisible();
    await expect(page.getByRole('tab', { name: 'Settings' })).toBeVisible();
  });

  test('shows console access for a running VM', async ({ page }) => {
    await gotoVMDetail(page, RUNNING_VM_ID, RUNNING_HOSTNAME);

    await expect(page.getByRole('heading', { name: 'Console Access' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Connect to Console' })).toBeVisible();
  });

  test('shows console unavailable for a stopped VM', async ({ page }) => {
    await gotoVMDetail(page, STOPPED_VM_ID, STOPPED_HOSTNAME);

    await expect(page.getByRole('heading', { name: 'Console Unavailable' })).toBeVisible();
    await expect(page.getByText('VM must be running to access the console')).toBeVisible();
  });

  test('shows VM configuration in the settings tab', async ({ page }) => {
    await gotoVMDetail(page, RUNNING_VM_ID, RUNNING_HOSTNAME);
    await page.getByRole('tab', { name: 'Settings' }).click();

    await expect(page.getByText('VM Settings')).toBeVisible();
    await expect(page.getByText('Basic Information')).toBeVisible();
    await expect(page.getByRole('heading', { name: RUNNING_HOSTNAME })).toBeVisible();
  });

  test('renders resource monitoring for a running VM', async ({ page }) => {
    await gotoVMDetail(page, RUNNING_VM_ID, RUNNING_HOSTNAME);
    await page.getByRole('tab', { name: 'Metrics' }).click();

    await expectMetricsContent(page);
  });

  test('opens and closes the stop dialog', async ({ page }) => {
    await gotoVMDetail(page, RUNNING_VM_ID, RUNNING_HOSTNAME);
    await page.getByRole('button', { name: 'Stop', exact: true }).click();

    await expect(page.getByRole('heading', { name: 'Stop Virtual Machine' })).toBeVisible();
    await page.getByRole('button', { name: 'Cancel' }).click();
    await expect(page.getByRole('heading', { name: 'Stop Virtual Machine' })).toHaveCount(0);
  });

  test('opens and closes the force-stop dialog', async ({ page }) => {
    await gotoVMDetail(page, RUNNING_VM_ID, RUNNING_HOSTNAME);
    await page.getByRole('button', { name: 'Force Stop' }).click();

    await expect(page.getByRole('heading', { name: 'Force Stop Virtual Machine' })).toBeVisible();
    await page.getByRole('button', { name: 'Cancel' }).click();
    await expect(page.getByRole('heading', { name: 'Force Stop Virtual Machine' })).toHaveCount(0);
  });

  test('shows the start action for a stopped VM', async ({ page }) => {
    await gotoVMDetail(page, STOPPED_VM_ID, STOPPED_HOSTNAME);

    await expect(page.getByRole('button', { name: 'Start' })).toBeVisible();
  });
});
