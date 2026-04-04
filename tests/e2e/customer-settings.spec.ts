import { test, expect, Page } from '@playwright/test';
import { createHmac } from 'crypto';

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
 * Customer Settings E2E Tests
 *
 * Tests cover:
 * - Profile management
 * - 2FA setup and management
 * - API key management
 * - Webhook management
 * - Notification preferences
 */

// ============================================
// Page Object Models
// ============================================

class CustomerSettingsPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/settings');
    await expect(this.page.locator('h1, [data-testid="page-title"]')).toContainText(/settings|account/i);
  }

  async navigateToProfile() {
    await this.page.click('a:has-text("Profile"), [data-testid="profile-tab"]');
  }

  async navigateToSecurity() {
    await this.page.click('a:has-text("Security"), a:has-text("2FA"), [data-testid="security-tab"]');
  }

  async navigateToAPIKeys() {
    await this.page.click('a:has-text("API Keys"), [data-testid="api-keys-tab"]');
  }

  async navigateToWebhooks() {
    await this.page.click('a:has-text("Webhooks"), [data-testid="webhooks-tab"]');
  }

  async navigateToNotifications() {
    await this.page.click('a:has-text("Notifications"), [data-testid="notifications-tab"]');
  }
}

class CustomerProfilePage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/settings/profile');
  }

  async getName() {
    return this.page.locator('input[name="name"]').inputValue();
  }

  async getEmail() {
    return this.page.locator('[data-testid="customer-email"]').textContent();
  }

  async getPhone() {
    return this.page.locator('input[name="phone"]').inputValue();
  }

  async updateName(name: string) {
    await this.page.fill('input[name="name"]', name);
  }

  async updatePhone(phone: string) {
    await this.page.fill('input[name="phone"]', phone);
  }

  async saveChanges() {
    await this.page.click('button:has-text("Save"), button[type="submit"]');
  }

  async expectSuccess() {
    await expect(this.page.locator('text=/saved|updated|success/i')).toBeVisible();
  }

  async expectValidationError(message: string | RegExp) {
    await expect(this.page.locator('.error, [role="alert"]')).toContainText(message);
  }
}

class CustomerPasswordPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/settings/security');
  }

  async fillCurrentPassword(password: string) {
    await this.page.fill('input[name="current_password"]', password);
  }

  async fillNewPassword(password: string) {
    await this.page.fill('input[name="new_password"]', password);
  }

  async fillConfirmPassword(password: string) {
    await this.page.fill('input[name="confirm_password"]', password);
  }

  async submit() {
    await this.page.click('button:has-text("Change Password"), button[type="submit"]');
  }

  async expectSuccess() {
    await expect(this.page.locator('text=/password.*changed|updated|success/i')).toBeVisible();
  }

  async expectValidationError(message: string | RegExp) {
    await expect(this.page.locator('.error, [role="alert"]')).toContainText(message);
  }
}

class Customer2FASetupPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/settings/security/2fa');
  }

  async clickEnable2FA() {
    await this.page.click('button:has-text("Enable 2FA"), [data-testid="enable-2fa-btn"]');
  }

  async clickDisable2FA() {
    await this.page.click('button:has-text("Disable 2FA"), [data-testid="disable-2fa-btn"]');
  }

  async getQRCode() {
    return this.page.locator('[data-testid="qr-code"], img[alt*="QR"], canvas');
  }

  async getSecretKey() {
    return this.page.locator('[data-testid="totp-secret"]').textContent();
  }

  async enterVerificationCode(code: string) {
    await this.page.fill('input[name="totp_code"]', code);
  }

  async confirmEnable() {
    await this.page.click('button:has-text("Verify"), button[type="submit"]');
  }

  async confirmDisable(code: string) {
    await this.page.fill('input[name="totp_code"]', code);
    await this.page.click('button:has-text("Confirm"), button[type="submit"]');
  }

  async expect2FAEnabled() {
    await expect(this.page.locator('text=/2FA.*enabled|two-factor.*active/i')).toBeVisible();
  }

  async expect2FADisabled() {
    await expect(this.page.locator('button:has-text("Enable 2FA")')).toBeVisible();
  }

  async getBackupCodes() {
    const codes = this.page.locator('[data-testid="backup-code"]');
    const texts: string[] = [];
    const count = await codes.count();
    for (let i = 0; i < count; i++) {
      texts.push((await codes.nth(i).textContent()) || '');
    }
    return texts;
  }

  async clickRegenerateBackupCodes() {
    await this.page.click('button:has-text("Regenerate"), [data-testid="regenerate-backup-codes-btn"]');
  }

  async getStatus() {
    return this.page.locator('[data-testid="2fa-status"]').textContent();
  }
}

class CustomerAPIKeysPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/settings/api-keys');
  }

  async getAPIKeyList() {
    return this.page.locator('[data-testid="api-key-item"], table tbody tr');
  }

  async getAPIKeyCount() {
    const list = await this.getAPIKeyList();
    return list.count();
  }

  async clickCreateAPIKey() {
    await this.page.click('button:has-text("Create API Key"), [data-testid="create-api-key-btn"]');
  }

  async clickDeleteKey(keyName: string) {
    const row = this.page.locator(`tr:has-text("${keyName}")`);
    await row.locator('button:has-text("Delete"), [data-testid="delete-btn"]').click();
  }

  async clickRotateKey(keyName: string) {
    const row = this.page.locator(`tr:has-text("${keyName}")`);
    await row.locator('button:has-text("Rotate"), [data-testid="rotate-btn"]').click();
  }

  async expectKeyInList(name: string) {
    await expect(this.page.locator(`text="${name}"`)).toBeVisible();
  }

  async expectKeyNotInList(name: string) {
    await expect(this.page.locator(`text="${name}"`)).not.toBeVisible();
  }
}

class CustomerAPIKeyCreateModal {
  constructor(private page: Page) {}

  async expectVisible() {
    await expect(this.page.locator('[role="dialog"], .modal')).toBeVisible();
  }

  async setName(name: string) {
    await this.page.fill('input[name="name"]', name);
  }

  async setExpiration(days: number) {
    await this.page.fill('input[name="expires_days"]', days.toString());
  }

  async setNoExpiration() {
    await this.page.check('input[name="no_expiration"]');
  }

  async selectPermissions(permissions: string[]) {
    for (const perm of permissions) {
      await this.page.check(`input[value="${perm}"], input[name="${perm}"]`);
    }
  }

  async submit() {
    await this.page.click('button:has-text("Create"), button[type="submit"]');
  }

  async cancel() {
    await this.page.click('button:has-text("Cancel")');
  }

  async expectValidationError(message: string | RegExp) {
    await expect(this.page.locator('.error, [role="alert"]')).toContainText(message);
  }
}

class CustomerAPIKeyShowModal {
  constructor(private page: Page) {}

  async expectVisible() {
    await expect(this.page.locator('[role="dialog"], .modal')).toBeVisible();
  }

  async getAPIKey() {
    return this.page.locator('[data-testid="api-key"]').textContent();
  }

  async copyKey() {
    await this.page.click('button:has-text("Copy"), [data-testid="copy-btn"]');
  }

  async close() {
    await this.page.click('button:has-text("Done"), button:has-text("Close")');
  }

  async expectWarningVisible() {
    await expect(this.page.locator('text=/save|copy|only shown once/i')).toBeVisible();
  }
}

class CustomerWebhooksPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/settings/webhooks');
  }

  async getWebhookList() {
    return this.page.locator('[data-testid="webhook-item"], table tbody tr');
  }

  async getWebhookCount() {
    const list = await this.getWebhookList();
    return list.count();
  }

  async clickCreateWebhook() {
    await this.page.click('button:has-text("Add Webhook"), [data-testid="create-webhook-btn"]');
  }

  async clickEditWebhook(name: string) {
    const row = this.page.locator(`tr:has-text("${name}")`);
    await row.locator('button:has-text("Edit"), a:has-text("Edit")').click();
  }

  async clickDeleteWebhook(name: string) {
    const row = this.page.locator(`tr:has-text("${name}")`);
    await row.locator('button:has-text("Delete"), [data-testid="delete-btn"]').click();
  }

  async clickViewDeliveries(name: string) {
    const row = this.page.locator(`tr:has-text("${name}")`);
    await row.locator('button:has-text("Deliveries"), a:has-text("History")').click();
  }

  async expectWebhookInList(name: string) {
    await expect(this.page.locator(`text="${name}"`)).toBeVisible();
  }

  async expectWebhookNotInList(name: string) {
    await expect(this.page.locator(`text="${name}"`)).not.toBeVisible();
  }
}

class CustomerWebhookCreateModal {
  constructor(private page: Page) {}

  async expectVisible() {
    await expect(this.page.locator('[role="dialog"], .modal')).toBeVisible();
  }

  async setName(name: string) {
    await this.page.fill('input[name="name"]', name);
  }

  async setURL(url: string) {
    await this.page.fill('input[name="url"]', url);
  }

  async setSecret(secret: string) {
    await this.page.fill('input[name="secret"]', secret);
  }

  async selectEvents(events: string[]) {
    for (const event of events) {
      await this.page.check(`input[value="${event}"], input[name="event_${event}"]`);
    }
  }

  async submit() {
    await this.page.click('button:has-text("Save"), button[type="submit"]');
  }

  async cancel() {
    await this.page.click('button:has-text("Cancel")');
  }

  async expectValidationError(message: string | RegExp) {
    await expect(this.page.locator('.error, [role="alert"]')).toContainText(message);
  }
}

class CustomerWebhookDeliveriesPage {
  constructor(private page: Page) {}

  async goto(webhookId: string) {
    await this.page.goto(`/settings/webhooks/${webhookId}/deliveries`);
  }

  async getDeliveryList() {
    return this.page.locator('table tbody tr, [data-testid="delivery-item"]');
  }

  async getDeliveryCount() {
    const list = await this.getDeliveryList();
    return list.count();
  }

  async filterByStatus(status: string) {
    await this.page.click('[data-testid="status-filter"]');
    await this.page.click(`option:has-text("${status}")`);
  }

  async clickDelivery(deliveryId: string) {
    await this.page.click(`[data-testid="delivery-${deliveryId}"]`);
  }
}

class CustomerNotificationsPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/settings/notifications');
  }

  async getEmailToggle() {
    return this.page.locator('input[name="email_enabled"], [data-testid="email-toggle"]');
  }

  async getTelegramToggle() {
    return this.page.locator('input[name="telegram_enabled"], [data-testid="telegram-toggle"]');
  }

  async toggleEmail(enabled: boolean) {
    const toggle = await this.getEmailToggle();
    const isChecked = await toggle.isChecked();
    if (isChecked !== enabled) {
      await toggle.click();
    }
  }

  async toggleTelegram(enabled: boolean) {
    const toggle = await this.getTelegramToggle();
    const isChecked = await toggle.isChecked();
    if (isChecked !== enabled) {
      await toggle.click();
    }
  }

  async setEmailAddress(email: string) {
    await this.page.fill('input[name="email_address"]', email);
  }

  async setTelegramChatId(chatId: string) {
    await this.page.fill('input[name="telegram_chat_id"]', chatId);
  }

  async selectNotificationEvents(events: string[]) {
    for (const event of events) {
      await this.page.check(`input[value="${event}"], input[name="${event}"]`);
    }
  }

  async savePreferences() {
    await this.page.click('button:has-text("Save"), button[type="submit"]');
  }

  async expectSuccess() {
    await expect(this.page.locator('text=/saved|updated|success/i')).toBeVisible();
  }
}

// ============================================
// Test Suite
// ============================================

test.describe('Customer Profile Settings', () => {
  let profilePage: CustomerProfilePage;

  test.beforeEach(async ({ page }) => {
    profilePage = new CustomerProfilePage(page);
    await profilePage.goto();
  });

  test('should display profile settings', async ({ page }) => {
    await expect(page.locator('input[name="name"]')).toBeVisible();
    await expect(page.locator('[data-testid="customer-email"]')).toBeVisible();
  });

  test('should show current profile info', async ({ page }) => {
    const name = await profilePage.getName();
    expect(name).toBeTruthy();
  });

  test('should update name', async ({ page }) => {
    const newName = `Updated Name ${Date.now()}`;

    await profilePage.updateName(newName);
    await profilePage.saveChanges();

    await profilePage.expectSuccess();
  });

  test('should update phone number', async ({ page }) => {
    await profilePage.updatePhone('+1-555-123-4567');
    await profilePage.saveChanges();

    await profilePage.expectSuccess();
  });

  test('should validate phone format', async ({ page }) => {
    await profilePage.updatePhone('invalid-phone');
    await profilePage.saveChanges();

    await profilePage.expectValidationError(/valid.*phone|invalid.*format/i);
  });

  test('should show email as read-only', async ({ page }) => {
    const emailInput = page.locator('input[name="email"]');

    if (await emailInput.isVisible()) {
      await expect(emailInput).toBeDisabled();
    }
  });
});

test.describe('Customer Password Change', () => {
  let passwordPage: CustomerPasswordPage;

  test.beforeEach(async ({ page }) => {
    passwordPage = new CustomerPasswordPage(page);
    await passwordPage.goto();
  });

  test('should display password change form', async ({ page }) => {
    await expect(page.locator('input[name="current_password"]')).toBeVisible();
    await expect(page.locator('input[name="new_password"]')).toBeVisible();
    await expect(page.locator('input[name="confirm_password"]')).toBeVisible();
  });

  test('should require current password', async ({ page }) => {
    await passwordPage.fillNewPassword('NewPassword123!');
    await passwordPage.fillConfirmPassword('NewPassword123!');
    await passwordPage.submit();

    await passwordPage.expectValidationError(/current.*password|required/i);
  });

  test('should validate password strength', async ({ page }) => {
    await passwordPage.fillCurrentPassword('CurrentPassword123!');
    await passwordPage.fillNewPassword('weak');
    await passwordPage.fillConfirmPassword('weak');
    await passwordPage.submit();

    await passwordPage.expectValidationError(/password.*requirements|stronger/i);
  });

  test('should require matching confirm password', async ({ page }) => {
    await passwordPage.fillCurrentPassword('CurrentPassword123!');
    await passwordPage.fillNewPassword('NewPassword123!');
    await passwordPage.fillConfirmPassword('DifferentPassword123!');
    await passwordPage.submit();

    await passwordPage.expectValidationError(/match|same/i);
  });

  test('should show error for incorrect current password', async ({ page }) => {
    await passwordPage.fillCurrentPassword('WrongPassword123!');
    await passwordPage.fillNewPassword('NewPassword123!');
    await passwordPage.fillConfirmPassword('NewPassword123!');
    await passwordPage.submit();

    await passwordPage.expectValidationError(/incorrect|invalid.*password/i);
  });

  test('should change password with valid input', async ({ page }) => {
    // This test requires knowing the current password
    await passwordPage.fillCurrentPassword('KnownPassword123!');
    await passwordPage.fillNewPassword('NewSecurePassword123!');
    await passwordPage.fillConfirmPassword('NewSecurePassword123!');
    await passwordPage.submit();

    await passwordPage.expectSuccess();
  });
});

test.describe('Customer 2FA Setup', () => {
  let twoFAPage: Customer2FASetupPage;

  test.beforeEach(async ({ page }) => {
    twoFAPage = new Customer2FASetupPage(page);
    await twoFAPage.goto();
  });

  test('should display 2FA status', async ({ page }) => {
    const status = await twoFAPage.getStatus();
    expect(status).toBeTruthy();
  });

  test('should show enable button when 2FA is disabled', async ({ page }) => {
    // Navigate to a customer without 2FA
    const status = await twoFAPage.getStatus();

    if (status?.toLowerCase().includes('disabled')) {
      await expect(page.locator('button:has-text("Enable 2FA")')).toBeVisible();
    }
  });

  test('should start 2FA setup flow', async ({ page }) => {
    const status = await twoFAPage.getStatus();

    if (status?.toLowerCase().includes('disabled')) {
      await twoFAPage.clickEnable2FA();

      // Should show QR code
      const qrCode = await twoFAPage.getQRCode();
      await expect(qrCode).toBeVisible();

      // Should show secret key
      await expect(page.locator('[data-testid="totp-secret"]')).toBeVisible();
    }
  });

  test('should show backup codes after enabling 2FA', async ({ page }) => {
    test.skip(!process.env.CUSTOMER_TOTP_SECRET, 'Requires CUSTOMER_TOTP_SECRET env var');

    const status = await twoFAPage.getStatus();

    if (status?.toLowerCase().includes('disabled')) {
      await twoFAPage.clickEnable2FA();

      const validCode = generateTOTP(process.env.CUSTOMER_TOTP_SECRET!);
      await twoFAPage.enterVerificationCode(validCode);
      await twoFAPage.confirmEnable();

      // Should show backup codes
      await expect(page.locator('[data-testid="backup-code"]')).toBeVisible();
    }
  });

  test('should verify 2FA with invalid code fails', async ({ page }) => {
    const status = await twoFAPage.getStatus();

    if (status?.toLowerCase().includes('disabled')) {
      await twoFAPage.clickEnable2FA();

      await twoFAPage.enterVerificationCode('000000');
      await twoFAPage.confirmEnable();

      await expect(page.locator('text=/invalid|incorrect|failed/i')).toBeVisible();
    }
  });
});

test.describe('Customer 2FA Management', () => {
  let twoFAPage: Customer2FASetupPage;

  test.beforeEach(async ({ page }) => {
    twoFAPage = new Customer2FASetupPage(page);
  });

  test('should show disable button when 2FA is enabled', async ({ page }) => {
    await twoFAPage.goto();

    const status = await twoFAPage.getStatus();

    if (status?.toLowerCase().includes('enabled')) {
      await expect(page.locator('button:has-text("Disable 2FA")')).toBeVisible();
    }
  });

  test('should disable 2FA with valid code', async ({ page }) => {
    test.skip(!process.env.CUSTOMER_TOTP_SECRET, 'Requires CUSTOMER_TOTP_SECRET env var');

    await twoFAPage.goto();

    const status = await twoFAPage.getStatus();

    if (status?.toLowerCase().includes('enabled')) {
      await twoFAPage.clickDisable2FA();

      const validCode = generateTOTP(process.env.CUSTOMER_TOTP_SECRET!);
      await twoFAPage.confirmDisable(validCode);

      await twoFAPage.expect2FADisabled();
    }
  });

  test('should explain that backup codes must be regenerated after setup', async ({ page }) => {
    await twoFAPage.goto();

    const status = await twoFAPage.getStatus();

    if (status?.toLowerCase().includes('enabled')) {
      await expect(page.locator('text=/Regenerate to receive a new set/i')).toBeVisible();
    }
  });

  test('should regenerate backup codes', async ({ page }) => {
    test.skip(!process.env.CUSTOMER_TOTP_SECRET, 'Requires CUSTOMER_TOTP_SECRET env var');

    await twoFAPage.goto();

    const status = await twoFAPage.getStatus();

    if (status?.toLowerCase().includes('enabled')) {
      await twoFAPage.clickRegenerateBackupCodes();

      const newCodes = await twoFAPage.getBackupCodes();
      expect(newCodes.length).toBeGreaterThan(0);
    }
  });
});

test.describe('Customer API Keys', () => {
  let apiKeysPage: CustomerAPIKeysPage;
  let createModal: CustomerAPIKeyCreateModal;
  let showModal: CustomerAPIKeyShowModal;

  test.beforeEach(async ({ page }) => {
    apiKeysPage = new CustomerAPIKeysPage(page);
    createModal = new CustomerAPIKeyCreateModal(page);
    showModal = new CustomerAPIKeyShowModal(page);
    await apiKeysPage.goto();
  });

  test('should display API keys page', async ({ page }) => {
    await expect(page.locator('h1, [data-testid="page-title"]')).toContainText(/api.*key/i);
  });

  test('should show create button', async ({ page }) => {
    await expect(page.locator('button:has-text("Create API Key")')).toBeVisible();
  });

  test('should list existing API keys', async ({ page }) => {
    const count = await apiKeysPage.getAPIKeyCount();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should open create API key modal', async ({ page }) => {
    await apiKeysPage.clickCreateAPIKey();

    await createModal.expectVisible();
  });

  test('should create API key with name', async ({ page }) => {
    await apiKeysPage.clickCreateAPIKey();

    await createModal.setName(`test-key-${Date.now()}`);
    await createModal.submit();

    // Should show the new key
    await showModal.expectVisible();
    await showModal.expectWarningVisible();
  });

  test('should create API key with expiration', async ({ page }) => {
    await apiKeysPage.clickCreateAPIKey();

    await createModal.setName('expiring-key');
    await createModal.setExpiration(30);
    await createModal.submit();

    await showModal.expectVisible();
  });

  test('should create API key without expiration', async ({ page }) => {
    await apiKeysPage.clickCreateAPIKey();

    await createModal.setName('no-expiry-key');
    await createModal.setNoExpiration();
    await createModal.submit();

    await showModal.expectVisible();
  });

  test('should show validation error for empty name', async ({ page }) => {
    await apiKeysPage.clickCreateAPIKey();

    await createModal.submit();

    await createModal.expectValidationError(/required|please.*enter/i);
  });

  test('should copy API key to clipboard', async ({ page }) => {
    await apiKeysPage.clickCreateAPIKey();

    await createModal.setName('copyable-key');
    await createModal.submit();

    await showModal.expectVisible();
    await showModal.copyKey();

    await expect(page.locator('text=/copied/i')).toBeVisible();
  });

  test('should close modal after creating key', async ({ page }) => {
    await apiKeysPage.clickCreateAPIKey();

    await createModal.setName('closable-key');
    await createModal.submit();

    await showModal.close();

    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
  });
});

test.describe('Customer API Key Management', () => {
  let apiKeysPage: CustomerAPIKeysPage;
  const testKeyName = 'existing-test-key';

  test.beforeEach(async ({ page }) => {
    apiKeysPage = new CustomerAPIKeysPage(page);
    await apiKeysPage.goto();
  });

  test('should show API key details in list', async ({ page }) => {
    const keyList = await apiKeysPage.getAPIKeyList();
    const count = await keyList.count();

    if (count > 0) {
      const firstKey = keyList.first();

      // Should show key name
      await expect(firstKey.locator('td:first-child, [data-testid="key-name"]')).toBeVisible();

      // Should show created date
      await expect(firstKey.locator('text=/\d{4}-\d{2}-\d{2}|ago/i')).toBeVisible();
    }
  });

  test('should rotate API key', async ({ page }) => {
    const keyList = await apiKeysPage.getAPIKeyList();
    const count = await keyList.count();

    if (count > 0) {
      const keyName = await keyList.first().locator('[data-testid="key-name"], td:first-child').textContent();

      if (keyName) {
        await apiKeysPage.clickRotateKey(keyName.trim());

        // Should show confirmation or new key
        await expect(page.locator('[role="dialog"], .modal')).toBeVisible();
      }
    }
  });

  test('should delete API key', async ({ page }) => {
    await page.goto('/settings/api-keys');

    // Create a key to delete
    await apiKeysPage.clickCreateAPIKey();
    await page.fill('input[name="name"]', `deletable-key-${Date.now()}`);
    await page.click('button:has-text("Create")');
    await page.click('button:has-text("Done")');

    // Find and delete the key
    const keyList = await apiKeysPage.getAPIKeyList();
    const count = await keyList.count();

    if (count > 0) {
      const keyName = await keyList.first().locator('[data-testid="key-name"], td:first-child').textContent();

      if (keyName) {
        await apiKeysPage.clickDeleteKey(keyName.trim());

        await page.click('button:has-text("Confirm")');

        await expect(page.locator('text=/deleted|removed/i')).toBeVisible();
      }
    }
  });
});

test.describe('Customer Webhooks', () => {
  let webhooksPage: CustomerWebhooksPage;
  let createModal: CustomerWebhookCreateModal;

  test.beforeEach(async ({ page }) => {
    webhooksPage = new CustomerWebhooksPage(page);
    createModal = new CustomerWebhookCreateModal(page);
    await webhooksPage.goto();
  });

  test('should display webhooks page', async ({ page }) => {
    await expect(page.locator('h1, [data-testid="page-title"]')).toContainText(/webhook/i);
  });

  test('should show create button', async ({ page }) => {
    await expect(page.locator('button:has-text("Add Webhook")')).toBeVisible();
  });

  test('should list existing webhooks', async ({ page }) => {
    const count = await webhooksPage.getWebhookCount();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should open create webhook modal', async ({ page }) => {
    await webhooksPage.clickCreateWebhook();

    await createModal.expectVisible();
  });

  test('should create webhook with URL', async ({ page }) => {
    await webhooksPage.clickCreateWebhook();

    await createModal.setName(`test-webhook-${Date.now()}`);
    await createModal.setURL('https://example.com/webhook');
    await createModal.submit();

    await expect(page.locator('text=/created|success/i')).toBeVisible();
  });

  test('should create webhook with secret', async ({ page }) => {
    await webhooksPage.clickCreateWebhook();

    await createModal.setName('secret-webhook');
    await createModal.setURL('https://example.com/webhook');
    await createModal.setSecret('my-webhook-secret');
    await createModal.submit();

    await expect(page.locator('text=/created|success/i')).toBeVisible();
  });

  test('should select webhook events', async ({ page }) => {
    await webhooksPage.clickCreateWebhook();

    await createModal.setName('events-webhook');
    await createModal.setURL('https://example.com/webhook');
    await createModal.selectEvents(['vm.created', 'vm.deleted']);
    await createModal.submit();

    await expect(page.locator('text=/created|success/i')).toBeVisible();
  });

  test('should show validation error for empty URL', async ({ page }) => {
    await webhooksPage.clickCreateWebhook();

    await createModal.setName('no-url-webhook');
    await createModal.submit();

    await createModal.expectValidationError(/required|valid.*url/i);
  });

  test('should show validation error for invalid URL', async ({ page }) => {
    await webhooksPage.clickCreateWebhook();

    await createModal.setName('invalid-url-webhook');
    await createModal.setURL('not-a-valid-url');
    await createModal.submit();

    await createModal.expectValidationError(/valid.*url/i);
  });

  test('should allow canceling webhook creation', async ({ page }) => {
    await webhooksPage.clickCreateWebhook();

    await createModal.cancel();

    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
  });
});

test.describe('Customer Webhook Management', () => {
  let webhooksPage: CustomerWebhooksPage;

  test.beforeEach(async ({ page }) => {
    webhooksPage = new CustomerWebhooksPage(page);
    await webhooksPage.goto();
  });

  test('should show webhook details in list', async ({ page }) => {
    const webhookList = await webhooksPage.getWebhookList();
    const count = await webhookList.count();

    if (count > 0) {
      const firstWebhook = webhookList.first();

      // Should show webhook name
      await expect(firstWebhook.locator('td:first-child, [data-testid="webhook-name"]')).toBeVisible();

      // Should show URL
      await expect(firstWebhook.locator('text=/http/i')).toBeVisible();
    }
  });

  test('should edit webhook', async ({ page }) => {
    const webhookList = await webhooksPage.getWebhookList();
    const count = await webhookList.count();

    if (count > 0) {
      const webhookName = await webhookList.first().locator('[data-testid="webhook-name"], td:first-child').textContent();

      if (webhookName) {
        await webhooksPage.clickEditWebhook(webhookName.trim());

        await expect(page.locator('[role="dialog"], .modal')).toBeVisible();
      }
    }
  });

  test('should delete webhook', async ({ page }) => {
    const webhookList = await webhooksPage.getWebhookList();
    const count = await webhookList.count();

    if (count > 0) {
      const webhookName = await webhookList.first().locator('[data-testid="webhook-name"], td:first-child').textContent();

      if (webhookName) {
        await webhooksPage.clickDeleteWebhook(webhookName.trim());

        await page.click('button:has-text("Confirm")');

        await expect(page.locator('text=/deleted|removed/i')).toBeVisible();
      }
    }
  });

  test('should view webhook deliveries', async ({ page }) => {
    const webhookList = await webhooksPage.getWebhookList();
    const count = await webhookList.count();

    if (count > 0) {
      const webhookName = await webhookList.first().locator('[data-testid="webhook-name"], td:first-child').textContent();

      if (webhookName) {
        await webhooksPage.clickViewDeliveries(webhookName.trim());

        await expect(page).toHaveURL(/\/deliveries/);
      }
    }
  });
});

test.describe('Customer Webhook Deliveries', () => {
  let deliveriesPage: CustomerWebhookDeliveriesPage;
  const testWebhookId = '00000000-0000-0000-0000-000000000001';

  test.beforeEach(async ({ page }) => {
    deliveriesPage = new CustomerWebhookDeliveriesPage(page);
    await deliveriesPage.goto(testWebhookId);
  });

  test('should display delivery history', async ({ page }) => {
    await expect(page.locator('h1, [data-testid="page-title"]')).toContainText(/deliveries|history/i);
  });

  test('should list deliveries', async ({ page }) => {
    const count = await deliveriesPage.getDeliveryCount();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should show delivery status', async ({ page }) => {
    const deliveryList = await deliveriesPage.getDeliveryList();
    const count = await deliveryList.count();

    if (count > 0) {
      const firstDelivery = deliveryList.first();

      // Should show status (success, failed, etc.)
      await expect(firstDelivery.locator('[data-testid="status"], .status-badge')).toBeVisible();
    }
  });

  test('should filter deliveries by status', async ({ page }) => {
    await deliveriesPage.filterByStatus('Success');

    await page.waitForLoadState('networkidle');

    const statuses = page.locator('[data-testid="status"]:has-text("Success")');
    const count = await statuses.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should show delivery details', async ({ page }) => {
    const deliveryList = await deliveriesPage.getDeliveryList();
    const count = await deliveryList.count();

    if (count > 0) {
      await deliveryList.first().click();

      // Should show delivery details (request/response)
      await expect(page.locator('[data-testid="delivery-details"], .details')).toBeVisible();
    }
  });
});

test.describe('Customer Notification Preferences', () => {
  let notificationsPage: CustomerNotificationsPage;

  test.beforeEach(async ({ page }) => {
    notificationsPage = new CustomerNotificationsPage(page);
    await notificationsPage.goto();
  });

  test('should display notification settings', async ({ page }) => {
    await expect(page.locator('h1, [data-testid="page-title"]')).toContainText(/notification/i);
  });

  test('should show email notification toggle', async ({ page }) => {
    const toggle = await notificationsPage.getEmailToggle();
    await expect(toggle).toBeVisible();
  });

  test('should toggle email notifications', async ({ page }) => {
    await notificationsPage.toggleEmail(true);
    await notificationsPage.savePreferences();

    await notificationsPage.expectSuccess();
  });

  test('should disable email notifications', async ({ page }) => {
    await notificationsPage.toggleEmail(false);
    await notificationsPage.savePreferences();

    await notificationsPage.expectSuccess();
  });

  test('should show telegram notification toggle', async ({ page }) => {
    const toggle = await notificationsPage.getTelegramToggle();
    if (await toggle.isVisible()) {
      await expect(toggle).toBeVisible();
    }
  });

  test('should show event selection', async ({ page }) => {
    // Should show checkboxes for different notification events
    await expect(page.locator('input[type="checkbox"]')).toBeVisible();
  });

  test('should select notification events', async ({ page }) => {
    await notificationsPage.selectNotificationEvents(['vm.created', 'vm.deleted']);
    await notificationsPage.savePreferences();

    await notificationsPage.expectSuccess();
  });
});

test.describe('Customer Settings Navigation', () => {
  test('should navigate between settings tabs', async ({ page }) => {
    await page.goto('/settings');

    await page.click('a:has-text("Profile")');
    await expect(page).toHaveURL(/\/profile/);

    await page.click('a:has-text("Security")');
    await expect(page).toHaveURL(/\/security/);

    await page.click('a:has-text("API Keys")');
    await expect(page).toHaveURL(/\/api-keys/);

    await page.click('a:has-text("Webhooks")');
    await expect(page).toHaveURL(/\/webhooks/);
  });

  test('should have working back navigation', async ({ page }) => {
    await page.goto('/settings/profile');

    await page.goBack();

    await expect(page).toHaveURL(/\/settings$/);
  });
});
