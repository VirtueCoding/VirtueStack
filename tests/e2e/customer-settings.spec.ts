import { test, expect, type Page } from '@playwright/test';
import { CREDENTIALS } from './utils/auth';

const customerBaseURL = process.env.CUSTOMER_URL || 'http://localhost:3001';
const customerTwoFactorSecret =
  process.env.TEST_CUSTOMER_TOTP_SECRET || CREDENTIALS.customerWith2FA.totpSecret;

class CustomerSettingsPage {
  constructor(private readonly page: Page) {}

  async goto(): Promise<void> {
    await this.page.goto('/settings');
    await expect(
      this.page.getByRole('heading', { name: 'Account Settings' }),
    ).toBeVisible();
  }

  async openTab(name: RegExp): Promise<void> {
    await this.page.getByRole('tab', { name }).click();
  }
}

test.use({
  baseURL: customerBaseURL,
  storageState: '.auth/customer-storage.json',
});

test.describe('Customer profile settings', () => {
  test('shows the seeded profile fields on the tabbed settings page', async ({ page }) => {
    const settings = new CustomerSettingsPage(page);

    await settings.goto();

    await expect(page.locator('#name')).toBeVisible();
    await expect(page.locator('#email')).toHaveValue(CREDENTIALS.customer.email);
    await expect(page.locator('#phone')).toBeVisible();
    await expect(page.getByRole('button', { name: /save changes/i })).toBeVisible();
  });

  test('validates profile edits before submitting', async ({ page }) => {
    const settings = new CustomerSettingsPage(page);

    await settings.goto();
    await page.locator('#name').fill('A');
    await page.getByRole('button', { name: /save changes/i }).click();

    await expect(page.getByText('Name must be at least 2 characters')).toBeVisible();
  });
});

test.describe('Customer security settings', () => {
  test('shows the password and 2FA controls on the security tab', async ({ page }) => {
    const settings = new CustomerSettingsPage(page);

    await settings.goto();
    await settings.openTab(/security/i);

    await expect(page.locator('#current-password')).toBeVisible();
    await expect(page.locator('#new-password')).toBeVisible();
    await expect(page.locator('#confirm-password')).toBeVisible();
    await expect(page.getByText('Enable 2FA above to see the QR code setup')).toBeVisible();
  });

  test('validates mismatched password changes before hitting the API', async ({ page }) => {
    const settings = new CustomerSettingsPage(page);

    await settings.goto();
    await settings.openTab(/security/i);
    await page.locator('#current-password').fill(CREDENTIALS.customer.password);
    await page.locator('#new-password').fill('UpdatedCustomerPass123!');
    await page.locator('#confirm-password').fill('DifferentCustomerPass123!');
    await page.getByRole('button', { name: /update password/i }).click();

    await expect(page.getByText('Passwords do not match')).toBeVisible();
  });

  test('starts 2FA setup from the security tab dialog', async ({ page }) => {
    const settings = new CustomerSettingsPage(page);

    await settings.goto();
    await settings.openTab(/security/i);
    await page.getByRole('switch').click();

    await expect(page.getByRole('dialog')).toBeVisible();
    await expect(page.getByText('Setup Two-Factor Authentication')).toBeVisible();
    await expect(page.locator('#totp-code')).toBeVisible();
  });
});

test.describe('Customer security settings with 2FA enabled', () => {
  test.use({ storageState: '.auth/customer-2fa-storage.json' });

  test.beforeEach(async ({ page }) => {
    test.skip(
      !customerTwoFactorSecret,
      'Requires TEST_CUSTOMER_TOTP_SECRET or the seeded customer 2FA secret',
    );

    const settings = new CustomerSettingsPage(page);
    await settings.goto();
    await settings.openTab(/security/i);
  });

  test('shows the enabled 2FA state and backup-code actions', async ({ page }) => {
    await expect(page.getByText('2FA is enabled')).toBeVisible();
    await expect(page.getByRole('button', { name: /regenerate/i })).toBeVisible();
  });

  test('uses the password confirmation dialog when disabling 2FA', async ({ page }) => {
    await page.getByRole('switch').click();

    await expect(page.getByText('Disable Two-Factor Authentication')).toBeVisible();
    await expect(page.locator('#disable-2fa-password')).toBeVisible();
    await expect(page.getByRole('button', { name: /^disable 2fa$/i })).toBeVisible();
  });
});

test.describe('Customer API keys settings', () => {
  test('shows the API keys tab and create flow', async ({ page }) => {
    const settings = new CustomerSettingsPage(page);

    await settings.goto();
    await settings.openTab(/api keys/i);

    await expect(page.getByRole('button', { name: /create new key/i })).toBeVisible();
    await page.getByRole('button', { name: /create new key/i }).click();

    await expect(page.getByRole('dialog')).toBeVisible();
    await expect(page.getByText('Create API Key')).toBeVisible();
    await expect(page.getByLabel('Name')).toBeVisible();
  });

  test('validates API key creation before submit', async ({ page }) => {
    const settings = new CustomerSettingsPage(page);

    await settings.goto();
    await settings.openTab(/api keys/i);
    await page.getByRole('button', { name: /create new key/i }).click();
    await page.getByRole('button', { name: /^create key$/i }).click();

    await expect(page.getByText('Name is required')).toBeVisible();
    await expect(page.getByText('At least one permission is required')).toBeVisible();
  });
});

test.describe('Customer webhook settings', () => {
  test('shows the webhook tab and add dialog', async ({ page }) => {
    const settings = new CustomerSettingsPage(page);

    await settings.goto();
    await settings.openTab(/webhooks/i);

    await expect(page.getByRole('button', { name: /add webhook/i })).toBeVisible();
    await page.getByRole('button', { name: /add webhook/i }).click();

    await expect(page.getByRole('dialog')).toBeVisible();
    await expect(page.getByText('Add Webhook')).toBeVisible();
    await expect(page.getByLabel('Endpoint URL')).toBeVisible();
    await expect(page.getByLabel('Secret')).toBeVisible();
  });

  test('validates webhook input on the current settings dialog', async ({ page }) => {
    const settings = new CustomerSettingsPage(page);

    await settings.goto();
    await settings.openTab(/webhooks/i);
    await page.getByRole('button', { name: /add webhook/i }).click();
    await page.getByRole('button', { name: /^add$/i }).click();

    await expect(page.getByText('Invalid URL')).toBeVisible();
    await expect(page.getByText('At least one event is required')).toBeVisible();
  });
});

test.describe('Customer notification settings', () => {
  test('shows notification preferences on the tabbed settings page', async ({ page }) => {
    const settings = new CustomerSettingsPage(page);

    await settings.goto();
    await settings.openTab(/notifications/i);

    await expect(page.locator('#email-notifications')).toBeVisible();
    await expect(page.locator('#telegram-notifications')).toBeVisible();
    await expect(page.getByText('Email Notifications')).toBeVisible();
    await expect(page.getByText('Telegram Notifications')).toBeVisible();
    await expect(page.getByRole('button', { name: /save preferences/i })).toBeVisible();
  });
});
