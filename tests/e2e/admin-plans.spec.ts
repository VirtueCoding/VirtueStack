import type { Page } from '@playwright/test';
import { test, expect } from './fixtures';

const BASIC_PLAN = 'Test Basic';
const PREMIUM_PLAN = 'Test Premium';
const ENTERPRISE_PLAN = 'Test Enterprise';

function planRow(page: Page, name: string) {
  return page.locator('tbody tr').filter({ has: page.getByText(name, { exact: true }) });
}

async function gotoPlans(page: Page) {
  await page.goto('/plans');
  await expect(page.getByRole('heading', { name: 'VM Plans' })).toBeVisible();
  await expect(page.getByText('Manage pricing tiers, VM specifications, and resource limits')).toBeVisible();
  await expect(page.getByPlaceholder('Search plans by name...')).toBeVisible();
  await expect(planRow(page, BASIC_PLAN)).toBeVisible();
}

async function createPlan(page: Page, name: string) {
  await page.getByRole('button', { name: 'Create Plan' }).click();

  const dialog = page.getByRole('dialog');
  await expect(dialog.getByRole('heading', { name: 'Create New Plan' })).toBeVisible();

  await dialog.locator('input#name').fill(name);
  await dialog.locator('input#vcpu').fill('3');
  await dialog.locator('input#memory_mb').fill('3072');
  await dialog.locator('input#disk_gb').fill('40');
  await dialog.locator('input#port_speed_mbps').fill('250');
  await dialog.locator('input#price_monthly').fill('1234');
  await dialog.locator('input#snapshot_limit').fill('5');
  await dialog.locator('input#backup_limit').fill('4');
  await dialog.locator('input#iso_upload_limit').fill('3');
  await dialog.locator('form').evaluate((form) => {
    (form as HTMLFormElement).requestSubmit();
  });

  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(planRow(page, name)).toBeVisible();
}

test.describe('Admin plans', () => {
  test.use({ storageState: '.auth/admin-storage.json' });

  test('renders the current plans page and table', async ({ page }) => {
    await gotoPlans(page);

    await expect(page.getByRole('button', { name: 'Create Plan' })).toBeVisible();
    await expect(page.getByText('All Plans')).toBeVisible();
    await expect(page.getByText('Total Plans')).toBeVisible();
    await expect(page.getByText('Active Plans', { exact: true })).toBeVisible();
    await expect(page.getByText('Inactive Plans', { exact: true })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Name' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Status' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'vCPU' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Memory' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Disk' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Bandwidth' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Price/Month' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Snapshots' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Backups' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'ISOs' })).toBeVisible();
  });

  test('shows the seeded plans with current resource data', async ({ page }) => {
    await gotoPlans(page);

    const basicRow = planRow(page, BASIC_PLAN);
    await expect(basicRow).toContainText(/active/i);
    await expect(basicRow).toContainText('1');
    await expect(basicRow).toContainText('1 GB');
    await expect(basicRow).toContainText('25 GB');
    await expect(basicRow).toContainText('100 Mbps');
    await expect(basicRow).toContainText('$10.00');
    await expect(basicRow).toContainText('2');

    const premiumRow = planRow(page, PREMIUM_PLAN);
    await expect(premiumRow).toContainText('4');
    await expect(premiumRow).toContainText('4 GB');
    await expect(premiumRow).toContainText('100 GB');
    await expect(premiumRow).toContainText('$40.00');

    const enterpriseRow = planRow(page, ENTERPRISE_PLAN);
    await expect(enterpriseRow).toContainText('8');
    await expect(enterpriseRow).toContainText('8 GB');
    await expect(enterpriseRow).toContainText('160 GB');
    await expect(enterpriseRow).toContainText('$80.00');
  });

  test('filters the visible plans with the current search box', async ({ page }) => {
    await gotoPlans(page);
    await page.getByPlaceholder('Search plans by name...').fill('premium');

    await expect(planRow(page, PREMIUM_PLAN)).toBeVisible();
    await expect(planRow(page, BASIC_PLAN)).toHaveCount(0);
    await expect(planRow(page, ENTERPRISE_PLAN)).toHaveCount(0);
  });

  test('shows the current create-plan dialog', async ({ page }) => {
    await gotoPlans(page);
    await page.getByRole('button', { name: 'Create Plan' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Create New Plan' })).toBeVisible();
    await expect(dialog.getByText('Basic Information')).toBeVisible();
    await expect(dialog.getByText('Resources')).toBeVisible();
    await expect(dialog.getByText('Network')).toBeVisible();
    await expect(dialog.getByText('Pricing (in cents)')).toBeVisible();
    await expect(dialog.getByText('Resource Limits per VM')).toBeVisible();
    await expect(dialog.getByText('Settings')).toBeVisible();
    await expect(dialog.locator('input#name')).toBeVisible();
    await expect(dialog.locator('input#slug')).toBeVisible();
    await expect(dialog.locator('input#vcpu')).toHaveValue('1');
    await expect(dialog.locator('input#memory_mb')).toHaveValue('1024');
    await expect(dialog.locator('input#disk_gb')).toHaveValue('20');
    await expect(dialog.getByRole('button', { name: 'Create Plan' })).toBeVisible();
  });

  test('shows the current create-plan validation messages', async ({ page }) => {
    await gotoPlans(page);
    await page.getByRole('button', { name: 'Create Plan' }).click();

    const dialog = page.getByRole('dialog');
    await dialog.locator('form').evaluate((form) => {
      (form as HTMLFormElement).requestSubmit();
    });

    await expect(dialog.getByText('Name is required')).toBeVisible();
    await expect(dialog.getByText('Slug is required')).toBeVisible();
  });

  test('validates slug format in the create-plan dialog', async ({ page }) => {
    await gotoPlans(page);
    await page.getByRole('button', { name: 'Create Plan' }).click();

    const dialog = page.getByRole('dialog');
    await dialog.locator('input#name').fill('Invalid Slug Test');
    await dialog.locator('input#slug').fill('Invalid Slug!');
    await dialog.locator('form').evaluate((form) => {
      (form as HTMLFormElement).requestSubmit();
    });

    await expect(dialog.getByText('Slug must be lowercase alphanumeric with hyphens')).toBeVisible();
  });

  test('opens the edit dialog with the current seeded plan values', async ({ page }) => {
    await gotoPlans(page);
    await planRow(page, PREMIUM_PLAN).getByRole('button', { name: 'Edit' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: `Edit Plan: ${PREMIUM_PLAN}` })).toBeVisible();
    await expect(dialog.locator('input#edit-name')).toHaveValue(PREMIUM_PLAN);
    await expect(dialog.locator('input#edit-slug')).toHaveValue('test-premium');
    await expect(dialog.locator('input#edit-vcpu')).toHaveValue('4');
    await expect(dialog.locator('input#edit-memory_mb')).toHaveValue('4096');
    await expect(dialog.locator('input#edit-disk_gb')).toHaveValue('100');
    await expect(dialog.locator('input#edit-price_monthly')).toHaveValue('4000');
    await expect(dialog.locator('input#edit-snapshot_limit')).toHaveValue('4');

    await dialog.getByRole('button', { name: 'Cancel' }).evaluate((button) => {
      (button as HTMLButtonElement).click();
    });
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('shows the delete warning for a seeded plan that is in use', async ({ page }) => {
    await gotoPlans(page);
    await planRow(page, BASIC_PLAN).getByRole('button', { name: 'Delete' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Delete Plan' })).toBeVisible();
    await expect(dialog).toContainText(BASIC_PLAN);
    await expect(dialog).toContainText(/Warning:/);
    await expect(dialog).toContainText(/associated/i);
    await expect(dialog.getByRole('button', { name: 'Delete' })).toBeDisabled();

    await dialog.getByRole('button', { name: 'Cancel' }).evaluate((button) => {
      (button as HTMLButtonElement).click();
    });
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('creates, edits, and deletes a plan through the current dialogs', async ({ page }) => {
    const createdPlanName = `E2E Plan ${Date.now()}`;
    const updatedPlanName = `${createdPlanName} Updated`;

    await gotoPlans(page);
    await createPlan(page, createdPlanName);

    await page.getByPlaceholder('Search plans by name...').fill(createdPlanName);
    const createdRow = planRow(page, createdPlanName);
    await expect(createdRow).toContainText('$12.34');
    await expect(createdRow).toContainText('3 GB');
    await expect(createdRow).toContainText('40 GB');

    await createdRow.getByRole('button', { name: 'Edit' }).click();
    const editDialog = page.getByRole('dialog');
    await expect(editDialog.getByRole('heading', { name: `Edit Plan: ${createdPlanName}` })).toBeVisible();
    await editDialog.locator('input#edit-name').fill(updatedPlanName);
    await editDialog.locator('input#edit-price_monthly').fill('2345');
    await editDialog.locator('input#edit-backup_limit').fill('6');
    await editDialog.locator('form').evaluate((form) => {
      (form as HTMLFormElement).requestSubmit();
    });

    await expect(page.getByRole('dialog')).toHaveCount(0);
    await page.getByPlaceholder('Search plans by name...').fill(updatedPlanName);
    const updatedRow = planRow(page, updatedPlanName);
    await expect(updatedRow).toBeVisible();
    await expect(updatedRow).toContainText('$23.45');
    await expect(updatedRow).toContainText('6');

    await updatedRow.getByRole('button', { name: 'Delete' }).click();
    const deleteDialog = page.getByRole('dialog');
    await expect(deleteDialog.getByRole('heading', { name: 'Delete Plan' })).toBeVisible();
    await expect(deleteDialog.getByRole('button', { name: 'Delete' })).toBeEnabled();
    await deleteDialog.getByRole('button', { name: 'Delete' }).evaluate((button) => {
      (button as HTMLButtonElement).click();
    });

    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(planRow(page, updatedPlanName)).toHaveCount(0);
  });
});
