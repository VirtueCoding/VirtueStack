/**
 * Customer Login Page Object Model
 */

import { Page, expect, Locator } from '@playwright/test';
import { BasePage } from './BasePage';
import { generateTOTP } from '../utils/auth';

export class CustomerLoginPage extends BasePage {
  readonly emailInput: Locator;
  readonly passwordInput: Locator;
  readonly submitButton: Locator;
  readonly totpInput: Locator;
  readonly errorMessage: Locator;
  readonly forgotPasswordLink: Locator;
  readonly registerLink: Locator;

  constructor(page: Page) {
    super(page);
    this.emailInput = this.getInput('email');
    this.passwordInput = this.getInput('password');
    this.submitButton = page.locator('button[type="submit"]');
    this.totpInput = page.locator('input[name="totp_code"], .totp-input input');
    this.errorMessage = page.locator('[role="alert"], .error-message, [data-testid="error"]');
    this.forgotPasswordLink = page.locator('a:has-text("Forgot password"), a:has-text("Reset password")');
    this.registerLink = page.locator('a:has-text("Register"), a:has-text("Sign up")');
  }

  async goto(): Promise<void> {
    await this.navigate('/login');
    await expect(this.page).toHaveTitle(/Login|VirtueStack/);
  }

  async fillEmail(email: string): Promise<void> {
    await this.emailInput.fill(email);
  }

  async fillPassword(password: string): Promise<void> {
    await this.passwordInput.fill(password);
  }

  async submit(): Promise<void> {
    await this.submitButton.click();
  }

  async login(email: string, password: string): Promise<void> {
    await this.fillEmail(email);
    await this.fillPassword(password);
    await this.submit();
  }

  async expect2FARequired(): Promise<void> {
    await expect(this.totpInput).toBeVisible({ timeout: 10000 });
  }

  async enter2FACode(code: string): Promise<void> {
    await this.totpInput.fill(code);
    await this.submitButton.click();
  }

  async complete2FA(totpSecret: string): Promise<void> {
    const code = generateTOTP(totpSecret);
    await this.enter2FACode(code);
  }

  async expectLoginSuccess(): Promise<void> {
    await expect(this.page).toHaveURL(/\/dashboard|\/vms|\/overview/);
  }

  async expectError(message: string | RegExp): Promise<void> {
    await expect(this.errorMessage).toContainText(message);
  }

  async clickForgotPassword(): Promise<void> {
    await this.forgotPasswordLink.click();
    await expect(this.page).toHaveURL(/\/forgot-password|\/reset-password/);
  }

  async clickRegister(): Promise<void> {
    await this.registerLink.click();
    await expect(this.page).toHaveURL(/\/register|\/signup/);
  }
}