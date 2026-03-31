/**
 * Customer Console Page Object Model
 */

import { Page, expect, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

export class CustomerConsolePage extends BasePage {
  readonly consoleContainer: Locator;
  readonly ctrlAltDelButton: Locator;
  readonly fullscreenButton: Locator;
  readonly closeButton: Locator;
  readonly statusIndicator: Locator;

  constructor(page: Page) {
    super(page);
    this.consoleContainer = page.locator('[data-testid="console-container"], canvas, #noVNC_canvas');
    this.ctrlAltDelButton = page.locator('button:has-text("Ctrl+Alt+Del"), [data-testid="ctrl-alt-del"]');
    this.fullscreenButton = page.locator('button:has-text("Fullscreen"), [data-testid="fullscreen"]');
    this.closeButton = page.locator('button:has-text("Close"), [data-testid="close-console"]');
    this.statusIndicator = page.locator('text=/connected|ready|disconnected/i');
  }

  async goto(vmId: string): Promise<void> {
    await this.navigate(`/vms/${vmId}/console`);
  }

  async waitForConsole(timeout = 30000): Promise<void> {
    await expect(this.consoleContainer).toBeVisible({ timeout });
  }

  async sendCtrlAltDelete(): Promise<void> {
    await this.ctrlAltDelButton.click();
  }

  async fullscreen(): Promise<void> {
    await this.fullscreenButton.click();
  }

  async close(): Promise<void> {
    await this.closeButton.click();
  }

  async expectConnected(): Promise<void> {
    await expect(this.statusIndicator).toContainText(/connected|ready/i);
  }

  async expectDisconnected(): Promise<void> {
    await expect(this.statusIndicator).toContainText(/disconnected/i);
  }
}