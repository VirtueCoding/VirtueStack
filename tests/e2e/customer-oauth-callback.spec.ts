import { test, expect } from '@playwright/test';

test.use({ storageState: { cookies: [], origins: [] } });

test.describe('Customer OAuth callback', () => {
  test('logs out partial oauth session when profile loading fails', async ({
    page,
    baseURL,
  }) => {
    const appBaseURL = baseURL ?? 'http://localhost:3001';
    const state = 'oauth-test-state';
    let callbackRequests = 0;
    let logoutRequests = 0;

    await page.addInitScript(
      ({ oauthState, redirectURI }) => {
        window.sessionStorage.setItem(
          'oauth_pkce_state',
          JSON.stringify({
            codeVerifier: 'test-verifier',
            state: oauthState,
            provider: 'google',
            redirectURI,
            timestamp: Date.now(),
          }),
        );
      },
      { oauthState: state, redirectURI: `${appBaseURL}/auth/callback` },
    );

    await page.route('**/api/v1/customer/auth/csrf', async (route) => {
      await route.fulfill({ status: 204, body: '' });
    });

    await page.route('**/api/v1/customer/auth/oauth/google/callback', async (route) => {
      callbackRequests += 1;
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

    await page.route('**/api/v1/customer/profile', async (route) => {
      await route.fulfill({
        status: 401,
        contentType: 'application/json',
        body: JSON.stringify({
          error: {
            code: 'UNAUTHORIZED',
            message: 'Unauthorized',
          },
        }),
      });
    });

    await page.route('**/api/v1/customer/auth/logout', async (route) => {
      logoutRequests += 1;
      await route.fulfill({ status: 204, body: '' });
    });

    await page.goto(`/auth/callback?code=test-code&state=${state}`);

    await expect(
      page.getByText(/Unable to load your profile after authentication\. Please log in again\./i),
    ).toBeVisible();
    await expect(page.getByRole('button', { name: /Back to Login/i })).toBeVisible();
    await expect(page).toHaveURL(new RegExp(`/auth/callback\\?code=test-code&state=${state}$`));
    await expect.poll(() => callbackRequests).toBe(1);
    await expect.poll(() => logoutRequests).toBe(1);
    await expect
      .poll(() => page.evaluate(() => window.sessionStorage.getItem('customer_auth_state')))
      .toBeNull();
  });
});
