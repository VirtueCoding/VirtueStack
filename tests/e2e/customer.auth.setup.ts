import { test as setup, expect } from '@playwright/test';
import fs from 'fs';
import path from 'path';
import { CREDENTIALS } from './utils/auth';

const authDir = path.join(__dirname, '.auth');
const storageStatePath = path.join(authDir, 'customer-storage.json');

setup.setTimeout(120000);

function hasReusableAccessTokenCookie(): boolean {
  if (!fs.existsSync(storageStatePath)) {
    return false;
  }

  try {
    const storageState = JSON.parse(fs.readFileSync(storageStatePath, 'utf8')) as {
      cookies?: Array<{ name?: string; path?: string; httpOnly?: boolean; secure?: boolean }>;
    };

    return Boolean(
      storageState.cookies?.some(
        (cookie) =>
          cookie.name === 'vs_access_token' &&
          cookie.path === '/' &&
          cookie.httpOnly === false &&
          cookie.secure === false,
      ),
    );
  } catch {
    return false;
  }
}

setup('authenticate customer storage state', async ({ page }) => {
  fs.mkdirSync(authDir, { recursive: true });

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
