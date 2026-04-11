import { test as setup, expect } from '@playwright/test';
import fs from 'fs';
import path from 'path';
import { CREDENTIALS, generateTOTP } from './utils/auth';

const authDir = path.join(__dirname, '.auth');
const storageStatePath = path.join(authDir, 'admin-storage.json');

setup.setTimeout(120000);

setup('authenticate admin storage state', async ({ page }) => {
  fs.mkdirSync(authDir, { recursive: true });

  if (fs.existsSync(storageStatePath)) {
    return;
  }

  const admin = CREDENTIALS.admin;

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
      await page.context().storageState({ path: storageStatePath });
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
