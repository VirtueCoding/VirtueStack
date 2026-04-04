import { test, expect } from '@playwright/test';

test.use({ storageState: { cookies: [], origins: [] } });

const PROFILE_LOAD_ERROR =
  'Unable to load your profile after authentication. Please log in again.';

function unauthorizedResponse() {
  return {
    status: 401,
    contentType: 'application/json',
    body: JSON.stringify({
      error: {
        code: 'UNAUTHORIZED',
        message: 'Unauthorized',
      },
    }),
  };
}

test.describe('Admin authentication profile loading', () => {
  test('does not authenticate when profile loading fails after login', async ({
    page,
  }) => {
    let logoutRequests = 0;

    await page.route('**/api/v1/admin/auth/me', async (route) => {
      await route.fulfill(unauthorizedResponse());
    });

    await page.route('**/api/v1/admin/auth/login', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          data: {
            token_type: 'Bearer',
            expires_in: 900,
          },
        }),
      });
    });

    await page.route('**/api/v1/admin/auth/logout', async (route) => {
      logoutRequests += 1;
      await route.fulfill({ status: 204, body: '' });
    });

    await page.goto('/login');
    await page.fill('input[name="email"]', 'admin@virtuestack.local');
    await page.fill('input[name="password"]', 'AdminTest123!');
    await page.click('button[type="submit"]');

    await expect(page.getByText(PROFILE_LOAD_ERROR)).toBeVisible();
    await expect(page).toHaveURL(/\/login$/);
    await expect(page.locator('input[name="email"]')).toBeVisible();
    await expect.poll(() => logoutRequests).toBe(1);
    await expect
      .poll(() => page.evaluate(() => window.sessionStorage.getItem('admin_auth_state')))
      .toBeNull();
  });

  test('does not authenticate when profile loading fails after 2FA verification', async ({
    page,
  }) => {
    let logoutRequests = 0;

    await page.route('**/api/v1/admin/auth/me', async (route) => {
      await route.fulfill(unauthorizedResponse());
    });

    await page.route('**/api/v1/admin/auth/login', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          data: {
            token_type: 'Bearer',
            expires_in: 900,
            requires_2fa: true,
            temp_token: 'temp-auth-token',
          },
        }),
      });
    });

    await page.route('**/api/v1/admin/auth/verify-2fa', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          data: {
            token_type: 'Bearer',
            expires_in: 900,
          },
        }),
      });
    });

    await page.route('**/api/v1/admin/auth/logout', async (route) => {
      logoutRequests += 1;
      await route.fulfill({ status: 204, body: '' });
    });

    await page.goto('/login');
    await page.fill('input[name="email"]', 'admin@virtuestack.local');
    await page.fill('input[name="password"]', 'AdminTest123!');
    await page.click('button[type="submit"]');

    await expect(page.locator('input[name="totp_code"]')).toBeVisible();

    await page.fill('input[name="totp_code"]', '123456');
    await page.click('button[type="submit"]');

    await expect(page.getByText(PROFILE_LOAD_ERROR)).toBeVisible();
    await expect(page).toHaveURL(/\/login$/);
    await expect(page.locator('input[name="email"]')).toBeVisible();
    await expect.poll(() => logoutRequests).toBe(1);
    await expect
      .poll(() => page.evaluate(() => window.sessionStorage.getItem('admin_auth_state')))
      .toBeNull();
  });
});
