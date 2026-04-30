import type { Page } from '@playwright/test';
import { test, expect } from './fixtures';

const UBUNTU_TEMPLATE = 'Ubuntu 22.04';
const DEBIAN_TEMPLATE = 'Debian 12';
const ROCKY_TEMPLATE = 'Rocky Linux 9';

function templateRow(page: Page, name: string) {
  return page.locator('tbody tr').filter({ has: page.getByText(name, { exact: true }) });
}

async function gotoTemplates(page: Page) {
  await page.goto('/templates');
  await expect(page.getByRole('heading', { name: 'OS Templates' })).toBeVisible();
  await expect(page.getByText('Manage VM templates for OS installation')).toBeVisible();
  await expect(page.getByPlaceholder('Search templates by name or OS family...')).toBeVisible();
  await expect(templateRow(page, UBUNTU_TEMPLATE)).toBeVisible();
}

async function openTemplateActions(page: Page, name: string) {
  await templateRow(page, name).locator('button').click();
}

async function createTemplate(page: Page, name: string) {
  await page.getByRole('button', { name: 'Create Template' }).click();

  const dialog = page.getByRole('dialog');
  await expect(dialog.getByRole('heading', { name: 'Create Template' })).toBeVisible();
  await dialog.locator('input#name').fill(name);
  await dialog.locator('input#os_family').fill('linux');
  await dialog.locator('input#rbd_image').fill(`e2e-${Date.now()}`);
  await dialog.getByRole('button', { name: 'Create' }).evaluate((button) => {
    (button as HTMLButtonElement).click();
  });

  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(templateRow(page, name)).toBeVisible();
}

test.describe('Admin templates', () => {
  test.use({ storageState: '.auth/admin-storage.json' });

  test('renders the current templates page and table', async ({ page }) => {
    await gotoTemplates(page);

    await expect(page.getByRole('button', { name: 'Build from ISO' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Create Template' })).toBeVisible();
    await expect(page.getByText('All Templates')).toBeVisible();
    await expect(page.getByText('Total Templates')).toBeVisible();
    await expect(page.getByText('Active Templates', { exact: true })).toBeVisible();
    await expect(page.getByText('Inactive Templates', { exact: true })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Name' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'OS Family' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'RBD Image' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Status' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Created' })).toBeVisible();
  });

  test('shows the seeded templates with current row data', async ({ page }) => {
    await gotoTemplates(page);

    const ubuntuRow = templateRow(page, UBUNTU_TEMPLATE);
    await expect(ubuntuRow).toContainText('Linux');
    await expect(ubuntuRow).toContainText('ubuntu-22.04');
    await expect(ubuntuRow).toContainText(/active/i);

    const debianRow = templateRow(page, DEBIAN_TEMPLATE);
    await expect(debianRow).toContainText('debian-12');
    await expect(debianRow).toContainText(/active/i);

    const rockyRow = templateRow(page, ROCKY_TEMPLATE);
    await expect(rockyRow).toContainText('rocky-9');
    await expect(rockyRow).toContainText(/active/i);
  });

  test('filters the visible templates with the current search box', async ({ page }) => {
    await gotoTemplates(page);
    await page.getByPlaceholder('Search templates by name or OS family...').fill('debian');

    await expect(templateRow(page, DEBIAN_TEMPLATE)).toBeVisible();
    await expect(templateRow(page, UBUNTU_TEMPLATE)).toHaveCount(0);
    await expect(templateRow(page, ROCKY_TEMPLATE)).toHaveCount(0);
  });

  test('shows the current build-from-iso dialog', async ({ page }) => {
    await gotoTemplates(page);
    await page.getByRole('button', { name: 'Build from ISO' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Build Template from ISO' })).toBeVisible();
    await expect(dialog.locator('input#build-name')).toBeVisible();
    await expect(dialog.locator('input#build-os_version')).toBeVisible();
    await expect(dialog.locator('input#build-iso_url')).toBeVisible();
    await expect(dialog.locator('input#build-node_id')).toBeVisible();
    await expect(dialog.getByRole('button', { name: 'Start Build' })).toBeDisabled();

    await dialog.getByRole('button', { name: 'Cancel' }).evaluate((button) => {
      (button as HTMLButtonElement).click();
    });
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('shows the current create-template dialog and name gating', async ({ page }) => {
    await gotoTemplates(page);
    await page.getByRole('button', { name: 'Create Template' }).click();

    const dialog = page.getByRole('dialog');
    const createButton = dialog.getByRole('button', { name: 'Create' });
    await expect(dialog.getByRole('heading', { name: 'Create Template' })).toBeVisible();
    await expect(dialog.locator('input#name')).toBeVisible();
    await expect(dialog.locator('input#os_family')).toHaveValue('debian');
    await expect(dialog.locator('input#rbd_image')).toBeVisible();
    await expect(createButton).toBeDisabled();

    await dialog.locator('input#name').fill('Draft Template');
    await expect(createButton).toBeEnabled();

    await dialog.getByRole('button', { name: 'Cancel' }).evaluate((button) => {
      (button as HTMLButtonElement).click();
    });
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('opens the edit dialog with the current seeded template values', async ({ page }) => {
    await gotoTemplates(page);
    await openTemplateActions(page, UBUNTU_TEMPLATE);
    await page.getByRole('menuitem', { name: 'Edit' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: `Edit Template: ${UBUNTU_TEMPLATE}` })).toBeVisible();
    await expect(dialog.locator('input#edit-name')).toHaveValue(UBUNTU_TEMPLATE);
    await expect(dialog.locator('input#edit-os_family')).toHaveValue('Linux');
    await expect(dialog.locator('input#edit-os_version')).toHaveValue('22.04');
    await expect(dialog.locator('input#edit-rbd_image')).toHaveValue('ubuntu-22.04');
    await expect(dialog.locator('input#edit-min_disk_gb')).toHaveValue('20');

    await dialog.getByRole('button', { name: 'Cancel' }).evaluate((button) => {
      (button as HTMLButtonElement).click();
    });
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('opens the import and cache-status dialogs for a seeded template', async ({ page }) => {
    await gotoTemplates(page);

    await openTemplateActions(page, UBUNTU_TEMPLATE);
    await page.getByRole('menuitem', { name: 'Import' }).click();

    let dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Import Template' })).toBeVisible();
    await dialog.getByRole('button', { name: 'Cancel' }).evaluate((button) => {
      (button as HTMLButtonElement).click();
    });
    await expect(page.getByRole('dialog')).toHaveCount(0);

    await openTemplateActions(page, UBUNTU_TEMPLATE);
    await page.getByRole('menuitem', { name: 'Cache Status' }).click();

    dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Cache Status' })).toBeVisible();
    await expect(dialog).toContainText('Ceph templates are accessed directly from the shared pool');
    await expect(dialog).toContainText('No cache entries found');
    await dialog.getByRole('button', { name: 'Close' }).first().evaluate((button) => {
      (button as HTMLButtonElement).click();
    });
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('creates, edits, and deletes a template through the current dialogs', async ({ page }) => {
    const createdTemplateName = `E2E Template ${Date.now()}`;
    const updatedTemplateName = `${createdTemplateName} Updated`;

    await gotoTemplates(page);
    await createTemplate(page, createdTemplateName);

    await page.getByPlaceholder('Search templates by name or OS family...').fill(createdTemplateName);
    const createdRow = templateRow(page, createdTemplateName);
    await expect(createdRow).toContainText(/linux/i);
    await expect(createdRow).toContainText(/active/i);

    await openTemplateActions(page, createdTemplateName);
    await page.getByRole('menuitem', { name: 'Edit' }).click();

    const editDialog = page.getByRole('dialog');
    await expect(editDialog.getByRole('heading', { name: `Edit Template: ${createdTemplateName}` })).toBeVisible();
    await editDialog.locator('input#edit-name').fill(updatedTemplateName);
    await editDialog.locator('input#edit-os_version').fill('1.0');
    await editDialog.locator('textarea#edit-description').fill('Updated by E2E test');
    await editDialog.locator('form').evaluate((form) => {
      (form as HTMLFormElement).requestSubmit();
    });

    await expect(page.getByRole('dialog')).toHaveCount(0);
    await page.getByPlaceholder('Search templates by name or OS family...').fill(updatedTemplateName);
    await expect(templateRow(page, updatedTemplateName)).toBeVisible();

    await openTemplateActions(page, updatedTemplateName);
    await page.getByRole('menuitem', { name: 'Delete' }).click();

    const deleteDialog = page.getByRole('dialog');
    await expect(deleteDialog.getByRole('heading', { name: 'Delete Template' })).toBeVisible();
    await expect(deleteDialog).toContainText(updatedTemplateName);
    await deleteDialog.getByRole('button', { name: 'Delete' }).evaluate((button) => {
      (button as HTMLButtonElement).click();
    });

    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(templateRow(page, updatedTemplateName)).toHaveCount(0);
  });
});
