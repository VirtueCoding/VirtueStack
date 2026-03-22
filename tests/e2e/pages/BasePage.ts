/**
 * Base Page Object Model
 *
 * Provides common methods and selectors for all page objects.
 */

import { Page, Locator, expect } from '@playwright/test';

export abstract class BasePage {
  constructor(protected page: Page) {}

  /**
   * Navigate to a URL and wait for page load
   */
  async navigate(path: string): Promise<void> {
    await this.page.goto(path);
    await this.page.waitForLoadState('domcontentloaded');
  }

  /**
   * Wait for navigation to complete
   */
  async waitForNavigation(expectedUrl: RegExp | string, timeout = 30000): Promise<void> {
    await expect(this.page).toHaveURL(expectedUrl, { timeout });
  }

  /**
   * Get a locator with data-testid
   */
  getByTestId(testId: string): Locator {
    return this.page.locator(`[data-testid="${testId}"]`);
  }

  /**
   * Get a button by text
   */
  getButton(text: string): Locator {
    return this.page.locator(`button:has-text("${text}")`);
  }

  /**
   * Get a link by text
   */
  getLink(text: string): Locator {
    return this.page.locator(`a:has-text("${text}")`);
  }

  /**
   * Get an input by name
   */
  getInput(name: string): Locator {
    return this.page.locator(`input[name="${name}"]`);
  }

  /**
   * Get a select by name
   */
  getSelect(name: string): Locator {
    return this.page.locator(`select[name="${name}"]`);
  }

  /**
   * Fill an input field
   */
  async fillInput(name: string, value: string): Promise<void> {
    await this.page.fill(`input[name="${name}"]`, value);
  }

  /**
   * Click a button and wait for response
   */
  async clickAndWait(selector: string, urlPattern: string | RegExp): Promise<void> {
    await Promise.all([
      this.page.waitForResponse(resp =>
        typeof urlPattern === 'string'
          ? resp.url().includes(urlPattern)
          : urlPattern.test(resp.url())
      ),
      this.page.click(selector),
    ]);
  }

  /**
   * Wait for an element to be visible
   */
  async waitForElement(selector: string, timeout = 10000): Promise<Locator> {
    const locator = this.page.locator(selector);
    await expect(locator).toBeVisible({ timeout });
    return locator;
  }

  /**
   * Wait for an element to be hidden
   */
  async waitForElementHidden(selector: string, timeout = 10000): Promise<void> {
    await expect(this.page.locator(selector)).toBeHidden({ timeout });
  }

  /**
   * Check if element is visible
   */
  async isElementVisible(selector: string): Promise<boolean> {
    try {
      return await this.page.locator(selector).isVisible();
    } catch {
      return false;
    }
  }

  /**
   * Get text content
   */
  async getText(selector: string): Promise<string | null> {
    return this.page.locator(selector).textContent();
  }

  /**
   * Take a screenshot
   */
  async screenshot(filename: string): Promise<void> {
    await this.page.screenshot({ path: `artifacts/${filename}` });
  }

  /**
   * Expect a toast/notification message
   */
  async expectToast(message: string | RegExp): Promise<void> {
    const toast = this.page.locator('[role="alert"], [data-testid="toast"], .toast, .notification');
    await expect(toast).toContainText(message);
  }

  /**
   * Expect an error message
   */
  async expectError(message: string | RegExp): Promise<void> {
    const error = this.page.locator('[role="alert"], .error-message, [data-testid="error"]');
    await expect(error).toContainText(message);
  }

  /**
   * Dismiss any modal/dialog
   */
  async dismissModal(): Promise<void> {
    const closeBtn = this.page.locator('[role="dialog"] button:has-text("Close"), [role="dialog"] button[aria-label="Close"]');
    if (await closeBtn.isVisible().catch(() => false)) {
      await closeBtn.click();
    }
  }
}