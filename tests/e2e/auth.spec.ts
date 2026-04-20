import { expect, Page, test, type TestInfo } from '@playwright/test';
import { CREDENTIALS, forwardedIPForTest, generateFreshTOTP, generateTOTP, routeAPIRequestsFromIP } from './utils/auth';

const ADMIN_URL = process.env.ADMIN_URL || 'http://localhost:3000';
const CUSTOMER_URL = process.env.CUSTOMER_URL || 'http://localhost:3001';
const EMAIL_INPUT = 'input[name="email"], input#email';
const PASSWORD_INPUT = 'input[name="password"], input#password';
const TOTP_INPUT = 'input[name="totp_code"], input#totp_code';

function escapeRegex(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function urlRegex(url: string, suffix = ''): RegExp {
  return new RegExp(`^${escapeRegex(url)}${suffix}$`);
}

function uniqueInvalidEmail(scope: string): string {
  return `${scope}.${Date.now()}.${Math.random().toString(16).slice(2)}@example.test`;
}

async function gotoLogin(page: Page, portal: 'admin' | 'customer') {
  const url = portal === 'admin' ? ADMIN_URL : CUSTOMER_URL;
  await page.goto(`${url}/login`);
  await expect(page).toHaveTitle(/VirtueStack/i);
}

async function submitCredentials(page: Page, email: string, password: string) {
  await page.fill(EMAIL_INPUT, email);
  await page.fill(PASSWORD_INPUT, password);
  await page.click('button[type="submit"]');
}

async function expect2FAPrompt(page: Page) {
  await expect(page.getByText(/Two-Factor Authentication/i)).toBeVisible();
  await expect(page.locator(TOTP_INPUT)).toBeVisible();
}

async function submit2FA(page: Page, secret: string) {
  await page.fill(TOTP_INPUT, await generateFreshTOTP(secret));
  await page.click('button[type="submit"]');
}

async function routeAuthTraffic(page: Page, testInfo: TestInfo, scope: string) {
  await routeAPIRequestsFromIP(page, forwardedIPForTest(testInfo, scope));
}

async function openUserMenu(page: Page) {
  const trigger = page.locator('div.border-t').last().locator('button').first();
  await expect(trigger).toBeVisible();
  await trigger.click();
}

async function logout(page: Page) {
  await openUserMenu(page);
  const logoutItem = page.getByRole('menuitem', { name: 'Log out' });
  await expect(logoutItem).toBeVisible();
  await logoutItem.evaluate((element: HTMLElement) => {
    element.click();
  });
}

test.describe('Admin authentication', () => {
  test.beforeEach(async ({ page }, testInfo) => {
    await routeAuthTraffic(page, testInfo, 'admin-auth');
    await gotoLogin(page, 'admin');
  });

  test('shows the current admin login form', async ({ page }) => {
    await expect(page.getByText('Admin Login')).toBeVisible();
    await expect(page.locator(EMAIL_INPUT)).toBeVisible();
    await expect(page.locator(PASSWORD_INPUT)).toBeVisible();
    await expect(page.getByRole('button', { name: 'Sign In' })).toBeVisible();
  });

  test('shows current client-side validation messages', async ({ page }) => {
    await page.click('button[type="submit"]');

    await expect(page.getByText('Invalid email address')).toBeVisible();
    await expect(page.getByText('Password must be at least 12 characters')).toBeVisible();
  });

  test('rejects invalid admin credentials', async ({ page }) => {
    await submitCredentials(page, uniqueInvalidEmail('admin-invalid'), 'WrongPassword123!');

    await expect(page.getByText('Invalid email or password. Please try again.')).toBeVisible();
    await expect(page).toHaveURL(urlRegex(ADMIN_URL, '/login/?'));
  });

  test('prompts for 2FA when the admin account requires it', async ({ page }) => {
    await submitCredentials(page, CREDENTIALS.adminWith2FA.email, CREDENTIALS.adminWith2FA.password);
    await expect2FAPrompt(page);
  });

  test('rejects an invalid admin 2FA code', async ({ page }) => {
    await submitCredentials(page, CREDENTIALS.adminWith2FA.email, CREDENTIALS.adminWith2FA.password);
    await expect2FAPrompt(page);

    await page.fill(TOTP_INPUT, '000000');
    await page.click('button[type="submit"]');

    await expect(page.getByText(/Invalid 2FA code|2FA verification failed/i)).toBeVisible();
  });

  test('logs in and out through the current admin shell', async ({ page }) => {
    await submitCredentials(page, CREDENTIALS.adminWith2FA.email, CREDENTIALS.adminWith2FA.password);
    await expect2FAPrompt(page);
    await submit2FA(page, CREDENTIALS.adminWith2FA.totpSecret);

    await expect(page).toHaveURL(urlRegex(ADMIN_URL, '/?'));
    await logout(page);
    await expect(page).toHaveURL(urlRegex(ADMIN_URL, '/login/?'));
  });
});

test.describe('Customer authentication', () => {
  test.beforeEach(async ({ page }, testInfo) => {
    await routeAuthTraffic(page, testInfo, 'customer-auth');
    await gotoLogin(page, 'customer');
  });

  test('shows the current customer login form', async ({ page }) => {
    await expect(page.getByText('Welcome back')).toBeVisible();
    await expect(page.locator(EMAIL_INPUT)).toBeVisible();
    await expect(page.locator(PASSWORD_INPUT)).toBeVisible();
    await expect(page.getByRole('button', { name: 'Sign In' })).toBeVisible();
    await expect(page.getByRole('link', { name: /Forgot your password/i })).toBeVisible();
  });

  test('rejects invalid customer credentials', async ({ page }) => {
    await submitCredentials(page, uniqueInvalidEmail('customer-invalid'), 'WrongPassword123!');

    await expect(page.getByText('Invalid email or password. Please try again.')).toBeVisible();
    await expect(page).toHaveURL(urlRegex(CUSTOMER_URL, '/login/?'));
  });

  test('logs in and out with the standard customer account', async ({ page }) => {
    await submitCredentials(page, CREDENTIALS.customer.email, CREDENTIALS.customer.password);

    await expect(page).toHaveURL(urlRegex(CUSTOMER_URL, '/vms/?'));
    await logout(page);
    await expect(page).toHaveURL(urlRegex(CUSTOMER_URL, '/login/?'));
  });

  test('prompts for 2FA when the customer account requires it', async ({ page }) => {
    await submitCredentials(page, CREDENTIALS.customerWith2FA.email, CREDENTIALS.customerWith2FA.password);
    await expect2FAPrompt(page);
  });

  test('rejects an invalid customer 2FA code', async ({ page }) => {
    await submitCredentials(page, CREDENTIALS.customerWith2FA.email, CREDENTIALS.customerWith2FA.password);
    await expect2FAPrompt(page);

    await page.fill(TOTP_INPUT, '000000');
    await page.click('button[type="submit"]');

    await expect(page.getByText(/Invalid 2FA code|2FA verification failed/i)).toBeVisible();
  });

  test('completes customer 2FA login with a valid TOTP code', async ({ page }) => {
    await submitCredentials(page, CREDENTIALS.customerWith2FA.email, CREDENTIALS.customerWith2FA.password);
    await expect2FAPrompt(page);
    await submit2FA(page, CREDENTIALS.customerWith2FA.totpSecret!);

    await expect(page).toHaveURL(urlRegex(CUSTOMER_URL, '/vms/?'));
  });
});

test.describe('Password reset', () => {
  test.beforeEach(async ({ page }, testInfo) => {
    await routeAuthTraffic(page, testInfo, 'password-reset');
  });

  test('shows the forgot-password form', async ({ page }) => {
    await page.goto(`${CUSTOMER_URL}/forgot-password`);

    await expect(page.getByText('Forgot Password')).toBeVisible();
    await expect(page.locator(EMAIL_INPUT)).toBeVisible();
    await expect(page.getByRole('button', { name: 'Send Reset Link' })).toBeVisible();
  });

  test('shows the generic success message after requesting a reset', async ({ page }) => {
    await page.goto(`${CUSTOMER_URL}/forgot-password`);
    await page.fill(EMAIL_INPUT, CREDENTIALS.customer.email);
    await page.click('button[type="submit"]');

    await expect(page.getByText('Check Your Email')).toBeVisible();
    await expect(
      page.getByText(/If an account with that email exists, you will receive an email/i)
    ).toBeVisible();
  });

  test('shows the reset form when a token is present', async ({ page }) => {
    await page.goto(`${CUSTOMER_URL}/reset-password?token=test-token`);

    await expect(page.getByText('Reset Password').first()).toBeVisible();
    await expect(page.locator('input[name="new_password"], input#new_password')).toBeVisible();
    await expect(page.locator('input[name="confirm_password"], input#confirm_password')).toBeVisible();
  });

  test('shows the invalid-link state when the reset token is missing', async ({ page }) => {
    await page.goto(`${CUSTOMER_URL}/reset-password`);

    await expect(page.getByText('Invalid Reset Link')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Request New Reset Link' })).toBeVisible();
  });

  test('enforces the current reset-password validation', async ({ page }) => {
    await page.goto(`${CUSTOMER_URL}/reset-password?token=test-token`);
    await page.fill('input[name="new_password"], input#new_password', 'weak');
    await page.fill('input[name="confirm_password"], input#confirm_password', 'weak');
    await page.click('button[type="submit"]');

    await expect(page.getByText('Password must be at least 12 characters').first()).toBeVisible();
  });
});

test.describe('Session handling', () => {
  test.beforeEach(async ({ page }, testInfo) => {
    await routeAuthTraffic(page, testInfo, 'session-handling');
  });

  test('keeps the customer session across a reload', async ({ page }) => {
    await gotoLogin(page, 'customer');
    await submitCredentials(page, CREDENTIALS.customer.email, CREDENTIALS.customer.password);
    await expect(page).toHaveURL(urlRegex(CUSTOMER_URL, '/vms/?'));

    await page.reload();

    await expect(page).toHaveURL(urlRegex(CUSTOMER_URL, '/vms/?'));
  });

  test('redirects back to login after cookies are cleared', async ({ page }) => {
    await gotoLogin(page, 'customer');
    await submitCredentials(page, CREDENTIALS.customer.email, CREDENTIALS.customer.password);
    await expect(page).toHaveURL(urlRegex(CUSTOMER_URL, '/vms/?'));

    await page.context().clearCookies();
    await page.goto(`${CUSTOMER_URL}/vms`);

    await expect(page).toHaveURL(urlRegex(CUSTOMER_URL, '/login/?'));
  });
});

test.describe('Authentication hardening', () => {
  test.beforeEach(async ({ page }, testInfo) => {
    await routeAuthTraffic(page, testInfo, 'auth-hardening');
  });

  test('rejects a password-based SQL injection attempt', async ({ page }) => {
    await gotoLogin(page, 'admin');
    await submitCredentials(page, uniqueInvalidEmail('admin-sqli'), "' OR '1'='1' --");

    await expect(page.getByText('Invalid email or password. Please try again.')).toBeVisible();
    await expect(page).toHaveURL(urlRegex(ADMIN_URL, '/login/?'));
  });
});
