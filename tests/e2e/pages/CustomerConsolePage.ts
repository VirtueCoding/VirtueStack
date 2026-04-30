/**
 * Customer Console Page Object Model
 */

import { Page, expect, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

export class CustomerConsolePage extends BasePage {
  readonly consoleTab: Locator;
  readonly consoleHeading: Locator;
  readonly connectButton: Locator;
  readonly unavailableHeading: Locator;

  constructor(page: Page) {
    super(page);
    this.consoleTab = page.getByRole('tab', { name: 'VNC' });
    this.consoleHeading = page.getByRole('heading', { name: 'Console Access' });
    this.connectButton = page.getByRole('button', { name: 'Connect to Console' });
    this.unavailableHeading = page.getByRole('heading', { name: 'Console Unavailable' });
  }

  async goto(vmId: string): Promise<void> {
    await this.navigate(`/vms/${vmId}`);
    await expect(this.consoleTab).toBeVisible();
    await this.consoleTab.click();
  }

  async waitForConsole(timeout = 30000): Promise<void> {
    if (await this.connectButton.isVisible({ timeout }).catch(() => false)) {
      return;
    }
    await expect(this.unavailableHeading).toBeVisible({ timeout });
  }

  async expectConnected(): Promise<void> {
    await expect(this.consoleHeading).toBeVisible();
    await expect(this.connectButton).toBeVisible();
  }
}
