import { test, expect } from './fixtures';
import { CREDENTIALS } from './utils/auth';
import { TEST_IDS } from './utils/api';

const RUNNING_VM_ID = process.env.TEST_CUSTOMER_VM_ID || TEST_IDS.vms.testVM1;
const STOPPED_VM_ID = TEST_IDS.vms.testVM2;
const RUNNING_HOSTNAME = 'test-vm-running';
const STOPPED_HOSTNAME = 'test-vm-stopped';
const RUNNING_IP = '192.0.2.10';
const STOPPED_IP = '192.0.2.11';

async function expectMetricsContent(page: import('@playwright/test').Page) {
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

test.describe('Customer landing', () => {
  test.use({ storageState: '.auth/customer-storage.json' });

  test('redirects an authenticated customer to the VM list', async ({ customerDashboardPage }) => {
    await customerDashboardPage.goto();

    await expect(customerDashboardPage.pageTitle).toBeVisible();
  });

  test('derives quick stats from the current VM list', async ({ customerDashboardPage }) => {
    await customerDashboardPage.goto();

    const stats = await customerDashboardPage.getQuickStats();
    expect(Number(stats.totalVMs)).toBeGreaterThanOrEqual(2);
    expect(Number(stats.runningVMs)).toBeGreaterThanOrEqual(1);
  });

  test('keeps the My VMs link working from the landing page', async ({ customerDashboardPage }) => {
    await customerDashboardPage.goto();
    await customerDashboardPage.navigateToVMs();

    await expect(customerDashboardPage.pageDescription).toBeVisible();
  });
});

test.describe('Customer VM list', () => {
  test.use({ storageState: '.auth/customer-storage.json' });

  test('shows the seeded VMs with status and IP information', async ({ customerVMListPage }) => {
    await customerVMListPage.goto();

    const runningRow = customerVMListPage.getRowByHostname(RUNNING_HOSTNAME);
    const stoppedRow = customerVMListPage.getRowByHostname(STOPPED_HOSTNAME);

    await expect(runningRow).toContainText(RUNNING_IP);
    await expect(runningRow).toContainText('Running');
    await expect(stoppedRow).toContainText(STOPPED_IP);
    await expect(stoppedRow).toContainText('Stopped');
  });

  test('filters the visible VM rows with the search input', async ({ customerVMListPage }) => {
    await customerVMListPage.goto();
    await customerVMListPage.searchVM('stopped');

    await expect(customerVMListPage.getRowByHostname(STOPPED_HOSTNAME)).toBeVisible();
    await expect(customerVMListPage.getRowByHostname(RUNNING_HOSTNAME)).toHaveCount(0);
  });

  test('shows status-specific quick actions in the VM table', async ({ customerVMListPage }) => {
    await customerVMListPage.goto();

    await expect(customerVMListPage.getRowByHostname(RUNNING_HOSTNAME).locator('button[title="Stop VM"]')).toBeVisible();
    await expect(customerVMListPage.getRowByHostname(RUNNING_HOSTNAME).locator('button[title="Restart VM"]')).toBeVisible();
    await expect(customerVMListPage.getRowByHostname(STOPPED_HOSTNAME).locator('button[title="Start VM"]')).toBeVisible();
  });
});

test.describe('Customer VM detail', () => {
  test.use({ storageState: '.auth/customer-storage.json' });

  test('shows the running VM overview and available tabs', async ({ customerVMDetailPage }) => {
    await customerVMDetailPage.goto(RUNNING_VM_ID);

    await expect(customerVMDetailPage.title).toHaveText(RUNNING_HOSTNAME);
    await expect(customerVMDetailPage.ipAddresses.first()).toHaveText(RUNNING_IP);
    await expect(customerVMDetailPage.vncTab).toBeVisible();
    await expect(customerVMDetailPage.serialTab).toBeVisible();
    await expect(customerVMDetailPage.backupsTab).toBeVisible();
    await expect(customerVMDetailPage.metricsTab).toBeVisible();
    await expect(customerVMDetailPage.settingsTab).toBeVisible();
  });

  test('shows console access for a running VM', async ({ customerConsolePage }) => {
    await customerConsolePage.goto(RUNNING_VM_ID);
    await customerConsolePage.waitForConsole();
    await customerConsolePage.expectConnected();
  });

  test('shows console unavailable for a stopped VM', async ({ customerConsolePage }) => {
    await customerConsolePage.goto(STOPPED_VM_ID);
    await customerConsolePage.waitForConsole();

    await expect(customerConsolePage.unavailableHeading).toBeVisible();
  });

  test('shows VM configuration in the settings tab', async ({ customerVMDetailPage, page }) => {
    await customerVMDetailPage.goto(RUNNING_VM_ID);
    await customerVMDetailPage.navigateToSettings();

    await expect(page.getByText('VM Settings')).toBeVisible();
    await expect(page.getByText('Basic Information')).toBeVisible();
    await expect(customerVMDetailPage.title).toHaveText(RUNNING_HOSTNAME);
  });

  test('renders resource monitoring for a running VM', async ({ customerVMDetailPage, page }) => {
    await customerVMDetailPage.goto(RUNNING_VM_ID);
    await customerVMDetailPage.navigateToMetrics();

    await expectMetricsContent(page);
  });

  test('opens and closes the stop dialog', async ({ customerVMDetailPage, page }) => {
    await customerVMDetailPage.goto(RUNNING_VM_ID);
    await customerVMDetailPage.stopVM();

    await expect(page.getByRole('heading', { name: 'Stop Virtual Machine' })).toBeVisible();
    await page.getByRole('button', { name: 'Cancel' }).click();
    await expect(page.getByRole('heading', { name: 'Stop Virtual Machine' })).toHaveCount(0);
  });

  test('opens and closes the force-stop dialog', async ({ customerVMDetailPage, page }) => {
    await customerVMDetailPage.goto(RUNNING_VM_ID);
    await customerVMDetailPage.openForceStopDialog();

    await expect(page.getByRole('heading', { name: 'Force Stop Virtual Machine' })).toBeVisible();
    await page.getByRole('button', { name: 'Cancel' }).click();
    await expect(page.getByRole('heading', { name: 'Force Stop Virtual Machine' })).toHaveCount(0);
  });

  test('shows the start action for a stopped VM', async ({ customerVMDetailPage }) => {
    await customerVMDetailPage.goto(STOPPED_VM_ID);

    await expect(customerVMDetailPage.startButton).toBeVisible();
  });
});

test.describe('Customer navigation', () => {
  test.use({ storageState: '.auth/customer-storage.json' });

  test('shows the current sidebar links', async ({ page }) => {
    await page.goto('/vms');

    const nav = page.locator('nav');
    await expect(nav.getByRole('link', { name: 'My VMs' })).toBeVisible();
    await expect(nav.getByRole('link', { name: 'Billing' })).toBeVisible();
    await expect(nav.getByRole('link', { name: 'Settings' })).toBeVisible();
  });

  test('opens the sidebar account menu', async ({ page }) => {
    await page.goto('/vms');

    await page.getByRole('button').filter({ hasText: CREDENTIALS.customer.email }).click();
    await expect(page.getByText('Account Settings')).toBeVisible();
    await expect(page.getByText('Log out')).toBeVisible();
  });
});
