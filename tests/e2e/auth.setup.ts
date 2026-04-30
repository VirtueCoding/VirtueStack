/**
 * Playwright Authentication Setup
 *
 * This file handles authentication state for E2E tests.
 * It creates authenticated browser states that can be reused across tests.
 *
 * Run: npx playwright test --project=setup-auth
 */

import fs from 'fs';
import { test as setup, expect, type Page } from '@playwright/test';
import { generateFreshTOTP, CREDENTIALS, forwardedIPForSeed, routeAPIRequestsFromIP } from './utils/auth';
import path from 'path';

const authDir = path.join(__dirname, '.auth');
const adminURL = process.env.ADMIN_URL || 'http://localhost:3000';
const customerURL = process.env.CUSTOMER_URL || 'http://localhost:3001';
const emailInput = 'input[name="email"], input#email';
const passwordInput = 'input[name="password"], input#password';
const totpInput = 'input[name="totp_code"], input#totp_code';

fs.mkdirSync(authDir, { recursive: true });

function escapeRegex(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

async function fillLogin(page: Page, email: string, password: string) {
  await page.fill(emailInput, email);
  await page.fill(passwordInput, password);
  await page.click('button[type="submit"]');
}

// Admin Auth Setup
setup('authenticate as admin', async ({ page }) => {
  const admin = CREDENTIALS.adminWith2FA;

  await routeAPIRequestsFromIP(page, forwardedIPForSeed('setup-auth-admin'));
  await page.goto(`${adminURL}/login`);
  await fillLogin(page, admin.email, admin.password);

  await expect(page.locator(totpInput)).toBeVisible();

  const totpCode = await generateFreshTOTP(admin.totpSecret);
  await page.fill(totpInput, totpCode);
  await page.click('button[type="submit"]');

  await expect(page).toHaveURL(new RegExp(`^${escapeRegex(adminURL)}/?$`));

  await page.context().storageState({ path: path.join(authDir, 'admin-storage.json') });
});

// Customer Auth Setup
setup('authenticate as customer', async ({ page }) => {
  const customer = CREDENTIALS.customer;

  await routeAPIRequestsFromIP(page, forwardedIPForSeed('setup-auth-customer'));
  await page.goto(`${customerURL}/login`);
  await fillLogin(page, customer.email, customer.password);

  await expect(page).toHaveURL(new RegExp(`^${escapeRegex(customerURL)}/vms/?$`));

  await page.context().storageState({ path: path.join(authDir, 'customer-storage.json') });
});

// Customer with 2FA Auth Setup
setup('authenticate as customer with 2FA', async ({ page }) => {
  const customer = CREDENTIALS.customerWith2FA;

  await routeAPIRequestsFromIP(page, forwardedIPForSeed('setup-auth-customer-2fa'));
  await page.goto(`${customerURL}/login`);
  await fillLogin(page, customer.email, customer.password);

  await expect(page.locator(totpInput)).toBeVisible();

  const totpCode = await generateFreshTOTP(customer.totpSecret!);
  await page.fill(totpInput, totpCode);
  await page.click('button[type="submit"]');

  await expect(page).toHaveURL(new RegExp(`^${escapeRegex(customerURL)}/vms/?$`));

  await page.context().storageState({ path: path.join(authDir, 'customer-2fa-storage.json') });
});
