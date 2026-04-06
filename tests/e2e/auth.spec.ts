import { test, expect, type Page } from '@playwright/test';
import { CREDENTIALS, generateTOTP } from './utils/auth';

const adminBaseURL = process.env.ADMIN_URL || 'http://localhost:3000';
const customerBaseURL = process.env.CUSTOMER_URL || 'http://localhost:3001';
const adminTwoFactorSecret =
  process.env.TEST_ADMIN_TOTP_SECRET || CREDENTIALS.adminWith2FA.totpSecret;
const customerTwoFactorSecret =
  process.env.TEST_CUSTOMER_TOTP_SECRET || CREDENTIALS.customerWith2FA.totpSecret;

function localPart(email: string): string {
  return email.split('@')[0] || email;
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

class LoginPage {
  constructor(private readonly page: Page) {}

  async goto(): Promise<void> {
    await this.page.goto('/login');
  }

  async submitCredentials(email: string, password: string): Promise<void> {
    await this.page.locator('#email').fill(email);
    await this.page.locator('#password').fill(password);
    await this.page.getByRole('button', { name: /^sign in$/i }).click();
  }

  async expectLoginForm(): Promise<void> {
    await expect(this.page.locator('#email')).toBeVisible();
    await expect(this.page.locator('#password')).toBeVisible();
    await expect(this.page.getByRole('button', { name: /^sign in$/i })).toBeVisible();
  }

  async expectTwoFactorPrompt(): Promise<void> {
    await expect(this.page.getByText('Two-Factor Authentication')).toBeVisible();
    await expect(this.page.locator('#totp_code')).toBeVisible();
  }

  async submitTwoFactorCode(code: string): Promise<void> {
    await this.page.locator('#totp_code').fill(code);
    await this.page.getByRole('button', { name: /^verify$/i }).click();
  }
}

async function logoutFromUserMenu(page: Page, email: string): Promise<void> {
  const trigger = page
    .getByRole('button', { name: new RegExp(escapeRegExp(localPart(email)), 'i') })
    .last();

  await expect(trigger).toBeVisible();
  await trigger.click();
  await page.getByRole('menuitem', { name: /log out/i }).click();
}

test.use({ storageState: { cookies: [], origins: [] } });

test.describe('Admin authentication', () => {
  test.use({ baseURL: adminBaseURL });

  test('shows the seeded admin login form', async ({ page }) => {
    const loginPage = new LoginPage(page);

    await loginPage.goto();

    await expect(page.getByText('Admin Login')).toBeVisible();
    await loginPage.expectLoginForm();
  });

  test('validates admin login form input before submission', async ({ page }) => {
    const loginPage = new LoginPage(page);

    await loginPage.goto();
    await loginPage.submitCredentials('invalid-email', 'short-pass');

    await expect(page.getByText('Invalid email address')).toBeVisible();
    await expect(page.getByText('Password must be at least 12 characters')).toBeVisible();
  });

  test('rejects invalid seeded admin credentials', async ({ page }) => {
    const loginPage = new LoginPage(page);

    await loginPage.goto();
    await loginPage.submitCredentials(CREDENTIALS.adminWith2FA.email, 'WrongAdminPass987!');

    await expect(page.getByText('Invalid email or password. Please try again.')).toBeVisible();
  });

  test('requires 2FA for the seeded admin account', async ({ page }) => {
    const loginPage = new LoginPage(page);

    await loginPage.goto();
    await loginPage.submitCredentials(
      CREDENTIALS.adminWith2FA.email,
      CREDENTIALS.adminWith2FA.password,
    );

    await loginPage.expectTwoFactorPrompt();
  });

  test('rejects an invalid admin 2FA code', async ({ page }) => {
    const loginPage = new LoginPage(page);

    await loginPage.goto();
    await loginPage.submitCredentials(
      CREDENTIALS.adminWith2FA.email,
      CREDENTIALS.adminWith2FA.password,
    );
    await loginPage.expectTwoFactorPrompt();
    await loginPage.submitTwoFactorCode('000000');

    await expect(page.getByText('Invalid 2FA code. Please try again.')).toBeVisible();
  });

  test('signs in and logs out with the seeded admin 2FA account', async ({ page }) => {
    const loginPage = new LoginPage(page);

    await loginPage.goto();
    await loginPage.submitCredentials(
      CREDENTIALS.adminWith2FA.email,
      CREDENTIALS.adminWith2FA.password,
    );
    await loginPage.expectTwoFactorPrompt();
    await loginPage.submitTwoFactorCode(generateTOTP(adminTwoFactorSecret));

    await expect(page).toHaveURL(new URL('/', adminBaseURL).toString());

    await logoutFromUserMenu(page, CREDENTIALS.adminWith2FA.email);

    await expect(page).toHaveURL(new URL('/login', adminBaseURL).toString());
    await expect(page.getByText('Admin Login')).toBeVisible();
  });
});

test.describe('Customer authentication', () => {
  test.use({ baseURL: customerBaseURL });

  test('shows the seeded customer login form and password reset link', async ({ page }) => {
    const loginPage = new LoginPage(page);

    await loginPage.goto();

    await expect(page.getByText('Welcome back')).toBeVisible();
    await loginPage.expectLoginForm();
    await expect(
      page.getByRole('link', { name: /forgot your password\?/i }),
    ).toBeVisible();
  });

  test('rejects invalid seeded customer credentials', async ({ page }) => {
    const loginPage = new LoginPage(page);

    await loginPage.goto();
    await loginPage.submitCredentials(CREDENTIALS.customer.email, 'WrongCustomerPass987!');

    await expect(page.getByText('Invalid email or password. Please try again.')).toBeVisible();
  });

  test('signs in and logs out with the seeded customer account', async ({ page }) => {
    const loginPage = new LoginPage(page);

    await loginPage.goto();
    await loginPage.submitCredentials(CREDENTIALS.customer.email, CREDENTIALS.customer.password);

    await expect(page).toHaveURL(new URL('/vms', customerBaseURL).toString());

    await logoutFromUserMenu(page, CREDENTIALS.customer.email);

    await expect(page).toHaveURL(new URL('/login', customerBaseURL).toString());
    await expect(page.getByText('Welcome back')).toBeVisible();
  });

  test('requires 2FA for the seeded customer 2FA account', async ({ page }) => {
    const loginPage = new LoginPage(page);

    await loginPage.goto();
    await loginPage.submitCredentials(
      CREDENTIALS.customerWith2FA.email,
      CREDENTIALS.customerWith2FA.password,
    );

    await loginPage.expectTwoFactorPrompt();
  });

  test('signs in with the seeded customer 2FA account', async ({ page }) => {
    const loginPage = new LoginPage(page);

    await loginPage.goto();
    await loginPage.submitCredentials(
      CREDENTIALS.customerWith2FA.email,
      CREDENTIALS.customerWith2FA.password,
    );
    await loginPage.expectTwoFactorPrompt();
    await loginPage.submitTwoFactorCode(generateTOTP(customerTwoFactorSecret));

    await expect(page).toHaveURL(new URL('/vms', customerBaseURL).toString());
  });
});

test.describe('Customer password reset pages', () => {
  test.use({ baseURL: customerBaseURL });

  test('navigates from login to the forgot password page', async ({ page }) => {
    await page.goto('/login');
    await page.getByRole('link', { name: /forgot your password\?/i }).click();

    await expect(page).toHaveURL(new URL('/forgot-password', customerBaseURL).toString());
    await expect(page.getByLabel('Email Address')).toBeVisible();
    await expect(page.getByRole('button', { name: /send reset link/i })).toBeVisible();
  });

  test('keeps forgot password responses generic for unknown emails', async ({ page }) => {
    await page.goto('/forgot-password');
    await page.getByLabel('Email Address').fill('missing@example.com');
    await page.getByRole('button', { name: /send reset link/i }).click();

    await expect(
      page.getByText('If an account with that email exists'),
    ).toBeVisible();
  });

  test('shows the invalid reset link state without a token', async ({ page }) => {
    await page.goto('/reset-password');

    await expect(page.getByText('Invalid Reset Link')).toBeVisible();
    await expect(page.getByText('Request New Reset Link')).toBeVisible();
  });

  test('shows the reset password form when a token is present', async ({ page }) => {
    await page.goto('/reset-password?token=test-reset-token');

    await expect(page.getByLabel('New Password')).toBeVisible();
    await expect(page.getByLabel('Confirm Password')).toBeVisible();
    await expect(page.getByRole('button', { name: /^reset password$/i })).toBeVisible();
  });
});
