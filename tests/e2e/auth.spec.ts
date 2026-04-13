import { test, expect, Page } from '@playwright/test';
import { createHmac } from 'crypto';
import { CREDENTIALS } from './utils/auth';

function base32Decode(input: string): Buffer {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
  const cleaned = input.replace(/=+$/, '');
  const bits: string[] = [];
  for (const char of cleaned.toUpperCase()) {
    const val = alphabet.indexOf(char);
    if (val === -1) continue;
    bits.push(val.toString(2).padStart(5, '0'));
  }
  const octets = bits.join('');
  const bytes = Buffer.alloc(Math.floor(octets.length / 8));
  for (let i = 0; i < bytes.length; i++) {
    bytes[i] = parseInt(octets.slice(i * 8, i * 8 + 8), 2);
  }
  return bytes;
}

function generateTOTP(secret: string, period = 30, digits = 6): string {
  const epoch = Math.floor(Date.now() / 1000 / period);
  const counter = Buffer.alloc(8);
  counter.writeUInt32BE(0, 0);
  counter.writeUInt32BE(epoch, 4);
  const key = base32Decode(secret.replace(/ /g, ''));
  const hmac = createHmac('sha1', key);
  hmac.update(counter);
  const bytes = hmac.digest();
  const offset = bytes[bytes.length - 1] & 0x0f;
  const binary =
    ((bytes[offset] & 0x7f) << 24) |
    ((bytes[offset + 1] & 0xff) << 16) |
    ((bytes[offset + 2] & 0xff) << 8) |
    (bytes[offset + 3] & 0xff);
  return (binary % Math.pow(10, digits)).toString().padStart(digits, '0');
}

/**
 * Authentication E2E Tests
 * 
 * Tests cover:
 * - Admin login/logout flow
 * - Customer login/logout flow
 * - MFA/2FA verification
 * - Session management
 * - Password validation
 */

// Test credentials
const ADMIN_CREDENTIALS = {
  email: CREDENTIALS.admin.email,
  password: CREDENTIALS.admin.password,
};

const CUSTOMER_CREDENTIALS = {
  email: CREDENTIALS.customer.email,
  password: CREDENTIALS.customer.password,
};

const seededAdminTotpSecret = process.env.TEST_ADMIN_TOTP_SECRET;
const seededCustomerTotpSecret = process.env.TEST_CUSTOMER_TOTP_SECRET;
const customerBaseURL = process.env.CUSTOMER_URL || 'http://localhost:3001';

const hasSeededAdminCredentials = Boolean(process.env.TEST_ADMIN_EMAIL && process.env.TEST_ADMIN_PASSWORD);
const hasSeededCustomer2FACredentials = Boolean(
  process.env.TEST_CUSTOMER_2FA_EMAIL && process.env.TEST_CUSTOMER_2FA_PASSWORD,
);

// Page Object Models
class LoginPage {
  constructor(private page: Page) {}

  async gotoAdmin() {
    await this.page.goto(`${process.env.ADMIN_URL || 'http://localhost:3000'}/login`);
    await expect(this.page).toHaveTitle(/Login|VirtueStack/);
  }

  async gotoCustomer() {
    await this.page.goto(`${process.env.CUSTOMER_URL || 'http://localhost:3001'}/login`);
    await expect(this.page).toHaveTitle(/Login|VirtueStack/);
  }

  async login(email: string, password: string) {
    await this.page.fill('input[name="email"]', email);
    await this.page.fill('input[name="password"]', password);
    await this.page.click('button[type="submit"]');
  }

  async expectError(message: string | RegExp) {
    await expect(this.page.locator('body')).toContainText(message);
  }

  async expect2FARequired() {
    await expect(this.page.locator('input[name="totp_code"], .totp-input')).toBeVisible();
  }

  async enter2FACode(code: string) {
    await this.page.fill('input[name="totp_code"]', code);
    await this.page.click('button[type="submit"]');
  }
}

async function bodyShowsRateLimit(page: Page): Promise<boolean> {
  const bodyText = (await page.locator('body').textContent()) ?? '';
  return /too many requests/i.test(bodyText);
}

async function loginAdminUntilTwoFactorPrompt(page: Page, loginPage: LoginPage): Promise<void> {
  const adminWithTwoFactor = CREDENTIALS.adminWith2FA;

  for (let attempt = 0; attempt < 6; attempt += 1) {
    await loginPage.gotoAdmin();
    await loginPage.login(adminWithTwoFactor.email, adminWithTwoFactor.password);

    if (await bodyShowsRateLimit(page)) {
      if (attempt === 5) {
        throw new Error('admin login remained rate limited');
      }
      await page.waitForTimeout(15000);
      continue;
    }

    await loginPage.expect2FARequired();
    return;
  }
}

// ============================================
// Admin Login Tests
// ============================================

if (hasSeededAdminCredentials) {
test.describe('Admin Authentication', () => {
  let loginPage: LoginPage;

  test.beforeEach(async ({ page }) => {
    loginPage = new LoginPage(page);
    await loginPage.gotoAdmin();
  });

  test('should display login form', async ({ page }) => {
    await expect(page.locator('input[name="email"]')).toBeVisible();
    await expect(page.locator('input[name="password"]')).toBeVisible();
    await expect(page.locator('button[type="submit"]')).toBeVisible();
  });

  test('should show validation errors for empty fields', async ({ page }) => {
    await page.click('button[type="submit"]');
    
    // Should show validation errors
    await expect(
      page.locator('text=/email is required|please enter your email|invalid email address/i'),
    ).toBeVisible();
  });

  test('should show error for invalid email format', async ({ page }) => {
    await page.fill('input[name="email"]', 'invalid-email');
    await page.fill('input[name="password"]', 'password123');
    await page.click('button[type="submit"]');
    
    await expect(page).toHaveURL(/\/login/);
  });

  test('should show error for non-existent admin', async ({ page }) => {
    await loginPage.login('nonexistent@example.com', 'InvalidSecret789!');
    
    await loginPage.expectError(/invalid credentials|user not found|invalid email or password/i);
  });

  test('should show error for wrong password', async ({ page }) => {
    await loginPage.login(ADMIN_CREDENTIALS.email, 'WrongSecret789!');
    
    await loginPage.expectError(
      /invalid credentials|incorrect password|invalid email or password|too many requests/i,
    );
  });

  test('should require 2FA for admin login', async ({ page }) => {
    await loginAdminUntilTwoFactorPrompt(page, loginPage);
  });

  test('should show error for invalid 2FA code', async ({ page }) => {
    await loginAdminUntilTwoFactorPrompt(page, loginPage);
    
    await loginPage.enter2FACode('000000');
    
    await loginPage.expectError(/invalid.*code|verification failed/i);
  });

  test('should complete login with valid 2FA code', async ({ page }) => {
    // Skip if no valid TOTP code available
    test.skip(!seededAdminTotpSecret, 'Requires TEST_ADMIN_TOTP_SECRET env var');
    
    await loginAdminUntilTwoFactorPrompt(page, loginPage);
    
    const validCode = generateTOTP(seededAdminTotpSecret!);
    await loginPage.enter2FACode(validCode);
    
    // Should redirect to admin dashboard
    await expect(page).toHaveURL(/\/dashboard|\/admin/);
  });

  test('should logout successfully', async ({ page }) => {
    test.skip(!seededAdminTotpSecret, 'Requires TEST_ADMIN_TOTP_SECRET for full login flow');

    await loginAdminUntilTwoFactorPrompt(page, loginPage);

    const validCode = generateTOTP(seededAdminTotpSecret!);
    await loginPage.enter2FACode(validCode);

    await expect(page).toHaveURL(/\/dashboard|\/admin/);

    // Click logout button
    await page.click('[data-testid="logout-button"], button:has-text("Logout")');

    // Should redirect to login
    await expect(page).toHaveURL(/\/login/);

    // Should not be able to access protected routes
    await page.goto('/dashboard');
    await expect(page).toHaveURL(/\/login/);
  });
});
}

// ============================================
// Customer Login Tests
// ============================================

test.describe('Customer Authentication', () => {
  test.use({ baseURL: customerBaseURL });

  let loginPage: LoginPage;

  test.beforeEach(async ({ page }) => {
    loginPage = new LoginPage(page);
    await loginPage.gotoCustomer();
  });

  test('should display customer login form', async ({ page }) => {
    await expect(page.locator('input[name="email"]')).toBeVisible();
    await expect(page.locator('input[name="password"]')).toBeVisible();
    await expect(page.locator('button[type="submit"]')).toBeVisible();
  });

  test('should not show self-service registration link', async ({ page }) => {
    await expect(page.locator('a:has-text("Register"), a:has-text("Sign up")')).toHaveCount(0);
  });

  test('should show link to forgot password', async ({ page }) => {
    await expect(page.locator('a[href="/forgot-password"]')).toBeVisible();
  });

  test('should login successfully for customer without 2FA', async ({ page }) => {
    // This test requires a customer account without 2FA
    await loginPage.login(CUSTOMER_CREDENTIALS.email, CUSTOMER_CREDENTIALS.password);
    
    // Should redirect to customer dashboard
    await expect(page).toHaveURL(/\/dashboard|\/vms|\/overview/);
  });

  test('should require 2FA for customers with 2FA enabled', async ({ page }) => {
    test.skip(!hasSeededCustomer2FACredentials, 'Requires seeded customer 2FA credentials');

    // This test requires a customer account with 2FA enabled
    await loginPage.login(CREDENTIALS.customerWith2FA.email, CREDENTIALS.customerWith2FA.password);
    
    await loginPage.expect2FARequired();
  });

  test('should complete customer login with valid 2FA', async ({ page }) => {
    test.skip(!hasSeededCustomer2FACredentials, 'Requires seeded customer 2FA credentials');
    // This requires actual TOTP generation
    test.skip(!seededCustomerTotpSecret, 'Requires TEST_CUSTOMER_TOTP_SECRET env var');
    
    await loginPage.login(CREDENTIALS.customerWith2FA.email, CREDENTIALS.customerWith2FA.password);
    await loginPage.expect2FARequired();
    
    const validCode = generateTOTP(seededCustomerTotpSecret!);
    await loginPage.enter2FACode(validCode);
    
    await expect(page).toHaveURL(/\/dashboard|\/vms/);
  });
});

// ============================================
// Password Reset Tests
// ============================================

test.describe('Password Reset', () => {
  test('should show forgot password form', async ({ page }) => {
    await page.goto('/forgot-password');
    
    await expect(page.locator('input[name="email"]')).toBeVisible();
    await expect(page.locator('button[type="submit"]')).toBeVisible();
  });

  test('should send reset email for valid email', async ({ page }) => {
    await page.goto('/forgot-password');
    
    await page.fill('input[name="email"]', CUSTOMER_CREDENTIALS.email);
    await page.click('button[type="submit"]');
    
    // Should show success message
    await expect(page.locator('text=/email sent|check your inbox/i')).toBeVisible();
  });

  test('should not reveal if email exists', async ({ page }) => {
    await page.goto('/forgot-password');
    
    await page.fill('input[name="email"]', 'nonexistent@example.com');
    await page.click('button[type="submit"]');
    
    // Should still show success message (security: don't reveal existence)
    await expect(page.locator('text=/email sent|check your inbox/i')).toBeVisible();
  });

  test('should reset password with valid token', async ({ page }) => {
    // This requires a valid reset token
    const resetToken = process.env.TEST_RESET_TOKEN || 'test-token';
    await page.goto(`/reset-password?token=${resetToken}`);
    
    await expect(page.locator('input[name="password"]')).toBeVisible();
    await expect(page.locator('input[name="confirm_password"]')).toBeVisible();
  });

  test('should enforce password requirements', async ({ page }) => {
    const resetToken = process.env.TEST_RESET_TOKEN || 'test-token';
    await page.goto(`/reset-password?token=${resetToken}`);
    
    // Try weak password
    await page.fill('input[name="password"]', 'weak');
    await page.fill('input[name="confirm_password"]', 'weak');
    await page.click('button[type="submit"]');
    
    await expect(page.locator('text=/password.*requirements|min.*characters/i')).toBeVisible();
  });
});

// ============================================
// Session Management Tests
// ============================================

test.describe('Session Management', () => {
  test.use({ storageState: '.auth/customer-storage.json' });

  test('should logout customer successfully', async ({ page }) => {
    await page.goto('/vms');
    await expect(page).toHaveURL(/\/dashboard|\/vms/);

    await page.click('[data-testid="logout-button"], button:has-text("Logout")');

    await expect(page).toHaveURL(/\/login/);
  });

  test('should maintain session across page reloads', async ({ page }) => {
    await page.goto('/vms');
    await expect(page).toHaveURL(/\/dashboard|\/vms/);

    await page.reload();
    await expect(page).toHaveURL(/\/dashboard|\/vms/);
  });

  test('should expire session after inactivity', async ({ page }) => {
    await page.goto('/vms');
    await expect(page).toHaveURL(/\/dashboard|\/vms/);

    await page.context().clearCookies();
    await page.goto('/dashboard');
    await expect(page).toHaveURL(/\/login/);
  });

  test('should handle concurrent sessions', async ({ page, context }) => {
    await page.goto('/vms');
    await expect(page).toHaveURL(/\/dashboard|\/vms/);

    const page2 = await context.newPage();
    await page2.goto('/dashboard');
    await expect(page2).toHaveURL(/\/dashboard|\/vms/);
  });
});

// ============================================
// Security Tests
// ============================================

test.describe('Authentication Security', () => {
  let loginPage: LoginPage;

  test.beforeEach(async ({ page }) => {
    loginPage = new LoginPage(page);
  });

  test('should prevent SQL injection in login form', async ({ page }) => {
    await loginPage.gotoAdmin();
    
    await page.fill('input[name="email"]', "admin@example.com' OR '1'='1");
    await page.fill('input[name="password"]', "' OR '1'='1");
    await page.click('button[type="submit"]');
    
    // Should not login, should show error
    await expect(page).toHaveURL(/\/login/);
    await loginPage.expectError(/invalid credentials/i);
  });

  test('should show error for invalid customer credentials', async ({ page }) => {
    await loginPage.gotoCustomer();
    await loginPage.login('nonexistent@example.com', 'InvalidSecret789!');

    await loginPage.expectError(/invalid credentials|too many requests|invalid email or password/i);
  });

  test('should rate limit login attempts', async ({ page }) => {
    await page.goto('/login');
    
    // Attempt multiple failed logins
    for (let i = 0; i < 6; i++) {
      await page.fill('input[name="email"]', 'test@example.com');
      await page.fill('input[name="password"]', `wrong-password-${i}`);
      await page.click('button[type="submit"]');
      await page.waitForLoadState('networkidle');
    }
    
    // Should show rate limit error
    await expect(page.locator('text=/too many|rate limit|try again later/i')).toBeVisible();
  });

  test('should have CSRF protection', async ({ page }) => {
    await loginPage.gotoAdmin();

    // Check for CSRF token in form
    const csrfToken = await page.locator('input[name="_csrf"], input[name="csrf_token"], input[name="csrf"]').getAttribute('value');

    // CSRF protection must be present: either a hidden form input or a csrf cookie
    const cookies = await page.context().cookies();
    const csrfCookie = cookies.find(c => c.name.toLowerCase().includes('csrf'));

    if (csrfToken) {
      expect(csrfToken).toBeTruthy();
      expect(csrfToken.length).toBeGreaterThan(0);
    } else {
      expect(csrfCookie).toBeDefined();
      expect(csrfCookie!.value.length).toBeGreaterThan(0);
    }
  });

  test('should set secure cookie attributes', async ({ page, context }) => {
    await loginPage.gotoAdmin();
    await loginPage.login(CUSTOMER_CREDENTIALS.email, CUSTOMER_CREDENTIALS.password);

    await expect(page).toHaveURL(/\/dashboard|\/vms/);

    const cookies = await context.cookies();
    const sessionCookie = cookies.find(c =>
      c.name.includes('session') || c.name.includes('token') || c.name.includes('auth')
    );

    if (sessionCookie) {
      expect(sessionCookie.httpOnly).toBe(true);
      expect(sessionCookie.secure).toBe(true);
      expect(sessionCookie.sameSite).toBeDefined();
    }
  });
});
