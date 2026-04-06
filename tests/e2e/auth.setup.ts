/**
 * Playwright Authentication Setup
 *
 * This file handles authentication state for E2E tests.
 * It creates authenticated browser states that can be reused across tests.
 *
 * Run: pnpm exec playwright test --project=setup-admin --project=setup-customer
 */

import { test as setup, expect } from '@playwright/test';
import { generateTOTP, CREDENTIALS } from './utils/auth';
import path from 'path';

const authDir = path.join(__dirname, '.auth');

// Admin Auth Setup
setup('authenticate as admin', async ({ page }) => {
  const admin = CREDENTIALS.adminWith2FA;

  await page.goto('/login');

  // Fill login form
  await page.fill('input[name="email"]', admin.email);
  await page.fill('input[name="password"]', admin.password);
  await page.click('button[type="submit"]');

  // Wait for 2FA prompt
  await expect(page.locator('input[name="totp_code"], .totp-input input')).toBeVisible();

  // Enter TOTP code
  const totpCode = generateTOTP(admin.totpSecret);
  await page.fill('input[name="totp_code"]', totpCode);
  await page.click('button[type="submit"]');

  // Wait for redirect to dashboard
  await expect(page).toHaveURL(/\/dashboard|\/admin/);

  // Save auth state
  await page.context().storageState({ path: path.join(authDir, 'admin-storage.json') });
});

// Customer Auth Setup
setup('authenticate as customer', async ({ page }) => {
  const customer = CREDENTIALS.customer;

  await page.goto('/login');

  // Fill login form
  await page.fill('input[name="email"]', customer.email);
  await page.fill('input[name="password"]', customer.password);
  await page.click('button[type="submit"]');

  // Wait for redirect to dashboard (customer without 2FA)
  await expect(page).toHaveURL(/\/dashboard|\/vms|\/overview/);

  // Save auth state
  await page.context().storageState({ path: path.join(authDir, 'customer-storage.json') });
});

// Customer with 2FA Auth Setup
setup('authenticate as customer with 2FA', async ({ page }) => {
  const customer = CREDENTIALS.customerWith2FA;

  await page.goto('/login');

  // Fill login form
  await page.fill('input[name="email"]', customer.email);
  await page.fill('input[name="password"]', customer.password);
  await page.click('button[type="submit"]');

  // Wait for 2FA prompt
  await expect(page.locator('input[name="totp_code"], .totp-input input')).toBeVisible();

  // Enter TOTP code
  const totpCode = generateTOTP(customer.totpSecret!);
  await page.fill('input[name="totp_code"]', totpCode);
  await page.click('button[type="submit"]');

  // Wait for redirect to dashboard
  await expect(page).toHaveURL(/\/dashboard|\/vms/);

  // Save auth state
  await page.context().storageState({ path: path.join(authDir, 'customer-2fa-storage.json') });
});
