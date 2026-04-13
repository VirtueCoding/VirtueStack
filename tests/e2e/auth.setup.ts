/**
 * Playwright Authentication Setup
 *
 * This file handles authentication state for E2E tests.
 * It creates authenticated browser states that can be reused across tests.
 *
 * Run: pnpm exec playwright test --project=setup-admin --project=setup-customer
 */

import { test as setup, expect } from '@playwright/test';
import fs from 'fs';
import { generateTOTP, CREDENTIALS } from './utils/auth';
import path from 'path';

const authDir = path.join(__dirname, '.auth');
const adminStoragePath = path.join(authDir, 'admin-storage.json');
const customerStoragePath = path.join(authDir, 'customer-storage.json');
const customerTwoFactorStoragePath = path.join(authDir, 'customer-2fa-storage.json');
const hasSeededCustomerTwoFactorCredentials = Boolean(
  process.env.TEST_CUSTOMER_2FA_EMAIL &&
    (process.env.TEST_CUSTOMER_2FA_PASSWORD || process.env.TEST_CUSTOMER_PASSWORD) &&
    process.env.TEST_CUSTOMER_TOTP_SECRET,
);

function ensureAuthDir(): void {
  fs.mkdirSync(authDir, { recursive: true });
}

setup.setTimeout(120000);

function hasStorageState(path: string): boolean {
  return fs.existsSync(path);
}

function hasFreshAccessToken(path: string): boolean {
  if (!hasStorageState(path)) {
    return false;
  }

  try {
    const storageState = JSON.parse(fs.readFileSync(path, 'utf8')) as {
      cookies?: Array<{
        name?: string;
        path?: string;
        httpOnly?: boolean;
        secure?: boolean;
        expires?: number;
      }>;
    };
    const now = Date.now() / 1000;

    return Boolean(
      storageState.cookies?.some(
        (cookie) =>
          cookie.name === 'vs_access_token' &&
          cookie.path === '/' &&
          cookie.httpOnly === false &&
          cookie.secure === false &&
          typeof cookie.expires === 'number' &&
          cookie.expires > now+60,
      ),
    );
  } catch {
    return false;
  }
}

function hasReusableAccessTokenCookie(): boolean {
  return hasFreshAccessToken(customerStoragePath);
}

// Admin Auth Setup
setup('authenticate as admin', async ({ page }, testInfo) => {
  setup.skip(testInfo.project.name !== 'setup-admin', 'Runs only for setup-admin');
  ensureAuthDir();

  if (hasFreshAccessToken(adminStoragePath)) {
    return;
  }

  const admin = CREDENTIALS.adminWith2FA;

  for (let attempt = 0; attempt < 6; attempt += 1) {
    await page.goto('/login');
    await page.fill('input[name="email"]', admin.email);
    await page.fill('input[name="password"]', admin.password);
    await page.click('button[type="submit"]');

    try {
      await expect(page.locator('input[name="totp_code"], .totp-input input')).toBeVisible();

      const totpCode = generateTOTP(admin.totpSecret);
      await page.fill('input[name="totp_code"]', totpCode);
      await page.click('button[type="submit"]');

      await expect(page).toHaveURL(/\/dashboard|\/admin/);
      await page.context().storageState({ path: adminStoragePath });
      return;
    } catch (err) {
      const bodyText = (await page.locator('body').textContent()) ?? '';
      if (!/too many requests/i.test(bodyText) || attempt === 5) {
        throw err;
      }

      await page.waitForTimeout(15000);
    }
  }
});

// Customer Auth Setup
setup('authenticate as customer', async ({ page }, testInfo) => {
  setup.skip(testInfo.project.name !== 'setup-customer', 'Runs only for setup-customer');
  ensureAuthDir();

  if (hasReusableAccessTokenCookie()) {
    return;
  }

  const customer = CREDENTIALS.customer;

  for (let attempt = 0; attempt < 6; attempt += 1) {
    await page.goto('/login');
    await page.fill('input[name="email"]', customer.email);
    await page.fill('input[name="password"]', customer.password);
    await page.click('button[type="submit"]');

    try {
      await expect(page).toHaveURL(/\/dashboard|\/vms|\/overview/);
      const cookies = await page.context().cookies();
      const accessTokenCookie = cookies.find((cookie) => cookie.name === 'vs_access_token');

      if (accessTokenCookie) {
        await page.context().addCookies([
          {
            ...accessTokenCookie,
            path: '/',
            httpOnly: false,
            secure: false,
            sameSite: 'Lax',
          },
        ]);
      }

      await page.context().storageState({ path: customerStoragePath });
      return;
    } catch (err) {
      const bodyText = (await page.locator('body').textContent()) ?? '';
      if (!/too many requests/i.test(bodyText) || attempt === 5) {
        throw err;
      }

      await page.waitForTimeout(15000);
    }
  }
});

// Customer with 2FA Auth Setup
setup('authenticate as customer with 2FA', async ({ page }, testInfo) => {
  setup.skip(testInfo.project.name !== 'setup-customer', 'Runs only for setup-customer');
  setup.skip(!hasSeededCustomerTwoFactorCredentials, 'Requires seeded customer 2FA credentials');
  ensureAuthDir();

  if (hasFreshAccessToken(customerTwoFactorStoragePath)) {
    return;
  }

  const customer = CREDENTIALS.customerWith2FA;

  for (let attempt = 0; attempt < 6; attempt += 1) {
    await page.goto('/login');
    await page.fill('input[name="email"]', customer.email);
    await page.fill('input[name="password"]', customer.password);
    await page.click('button[type="submit"]');

    try {
      await expect(page.locator('input[name="totp_code"], .totp-input input')).toBeVisible();

      const totpCode = generateTOTP(customer.totpSecret!);
      await page.fill('input[name="totp_code"]', totpCode);
      await page.click('button[type="submit"]');

      await expect(page).toHaveURL(/\/dashboard|\/vms/);
      await page.context().storageState({ path: customerTwoFactorStoragePath });
      return;
    } catch (err) {
      const bodyText = (await page.locator('body').textContent()) ?? '';
      if (!/too many requests/i.test(bodyText) || attempt === 5) {
        throw err;
      }

      await page.waitForTimeout(15000);
    }
  }
});
