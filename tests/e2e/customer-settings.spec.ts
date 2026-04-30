import type { Page } from '@playwright/test';
import { test, expect } from './fixtures';

async function gotoSettings(page: Page) {
  await page.goto('/settings');
  await expect(page.getByRole('heading', { name: 'Account Settings' })).toBeVisible();
  await expect(page.getByText('Manage your account settings, security, and integrations')).toBeVisible();
}

async function openSettingsTab(page: Page, name: string | RegExp) {
  await page.getByRole('tab', { name }).click();
}

test.describe('Customer settings', () => {
  test.describe.configure({ mode: 'serial' });
  test.use({ storageState: '.auth/customer-storage.json' });

  test('renders the current settings tabs and prefilled profile form', async ({ page }) => {
    await gotoSettings(page);

    await expect(page.getByRole('tab', { name: 'Profile' })).toBeVisible();
    await expect(page.getByRole('tab', { name: 'Security' })).toBeVisible();
    await expect(page.getByRole('tab', { name: 'Notifications' })).toBeVisible();
    await expect(page.getByRole('tab', { name: 'API Keys' })).toBeVisible();
    await expect(page.getByRole('tab', { name: 'Webhooks' })).toBeVisible();

    await openSettingsTab(page, 'Profile');
    await expect(page.getByText('Profile Information')).toBeVisible();
    await expect(page.locator('input#name')).toHaveValue('Test Customer');
    await expect(page.locator('input#email')).toHaveValue('customer@test.virtuestack.local');
  });

  test('validates the profile form before submitting', async ({ page }) => {
    await gotoSettings(page);

    await page.locator('input#name').fill('A');
    await page.getByRole('button', { name: 'Save Changes' }).click();

    await expect(page.getByText('Name must be at least 2 characters')).toBeVisible();
  });

  test('shows password confirmation validation on the security tab', async ({ page }) => {
    await gotoSettings(page);
    await openSettingsTab(page, 'Security');

    await expect(page.getByText('Change Password')).toBeVisible();
    await page.locator('input#current-password').fill('Customer123!');
    await page.locator('input#new-password').fill('NewPassword123!');
    await page.locator('input#confirm-password').fill('DifferentPassword123!');
    await page.getByRole('button', { name: 'Update Password' }).click();

    await expect(page.getByText('Passwords do not match')).toBeVisible();
  });

  test('opens and closes the 2FA setup dialog for a customer without 2FA', async ({ page }) => {
    await gotoSettings(page);
    await openSettingsTab(page, 'Security');

    await expect(page.getByText('Enable 2FA above to see the QR code setup')).toBeVisible();
    await page.getByRole('switch').click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Setup Two-Factor Authentication' })).toBeVisible();
    await expect(dialog.getByText(/Scan the QR code with your authenticator app/i)).toBeVisible();

    await dialog.getByRole('button', { name: 'Cancel' }).click();
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('renders notification channels and event toggles', async ({ page }) => {
    await gotoSettings(page);
    await openSettingsTab(page, 'Notifications');

    await expect(page.getByText('Notification Channels')).toBeVisible();
    await expect(page.getByText('Email Notifications')).toBeVisible();
    await expect(page.getByText('Telegram Notifications')).toBeVisible();
    await expect(page.getByText('Event Notifications')).toBeVisible();
    await expect(page.getByText('VM Created')).toBeVisible();
    await expect(page.getByText('Backup Failed')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Save Preferences' })).toBeVisible();
  });

  test('validates required fields on the API key dialog', async ({ page }) => {
    await gotoSettings(page);
    await openSettingsTab(page, 'API Keys');

    await page.getByRole('button', { name: 'Create New Key' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Create API Key' })).toBeVisible();
    await dialog.getByRole('button', { name: 'Create Key' }).click();

    await expect(dialog.getByText('Name is required')).toBeVisible();
    await expect(dialog.getByText('At least one permission is required')).toBeVisible();
  });

  test('validates VM scope selection for API keys', async ({ page }) => {
    await gotoSettings(page);
    await openSettingsTab(page, 'API Keys');

    await page.getByRole('button', { name: 'Create New Key' }).click();

    const dialog = page.getByRole('dialog');
    await dialog.locator('input#key-name').fill('Scoped key validation');
    await dialog.locator('input#vm\\:read').check();
    await dialog.getByLabel('Restrict this key to selected VMs only').check();
    await dialog.getByRole('button', { name: 'Create Key' }).click();

    await expect(dialog.getByText('Select at least one VM to scope this key')).toBeVisible();
  });

  test('validates required fields on the webhook dialog', async ({ page }) => {
    await gotoSettings(page);
    await openSettingsTab(page, 'Webhooks');

    await page.getByRole('button', { name: 'Add Webhook' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Add Webhook' })).toBeVisible();
    await dialog.getByRole('button', { name: 'Add' }).click();

    await expect(dialog.getByText('Invalid URL')).toBeVisible();
    await expect(dialog.getByText('At least one event is required')).toBeVisible();
  });
});

test.describe('Customer settings with 2FA enabled', () => {
  test.describe.configure({ mode: 'serial' });
  test.use({ storageState: '.auth/customer-2fa-storage.json' });

  test('shows enabled 2FA state and backup code access', async ({ page }) => {
    await gotoSettings(page);
    await openSettingsTab(page, 'Security');

    await expect(page.getByText('2FA is enabled')).toBeVisible();
    await page.getByRole('button', { name: 'View Backup Codes' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Backup Codes' })).toBeVisible();
    await expect(dialog.getByText('Save these codes in a secure place. Each code can only be used once.')).toBeVisible();
    await expect(dialog.getByRole('button', { name: 'Download' })).toBeVisible();

    await dialog.getByRole('button', { name: 'Close' }).first().click();
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('opens and closes the disable 2FA confirmation dialog', async ({ page }) => {
    await gotoSettings(page);
    await openSettingsTab(page, 'Security');

    await page.getByRole('switch').click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'Disable Two-Factor Authentication' })).toBeVisible();
    await expect(dialog.getByText(/Enter your current password to confirm disabling 2FA/i)).toBeVisible();

    await dialog.getByRole('button', { name: 'Cancel' }).click();
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });
});
