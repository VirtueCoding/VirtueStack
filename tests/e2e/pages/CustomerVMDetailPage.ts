/**
 * Customer VM Detail Page Object Model
 */

import { Page, expect, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

export class CustomerVMDetailPage extends BasePage {
  readonly hostname: Locator;
  readonly status: Locator;
  readonly ipAddresses: Locator;
  readonly cpuUsage: Locator;
  readonly memoryUsage: Locator;
  readonly diskUsage: Locator;
  readonly bandwidthUsed: Locator;
  readonly bandwidthLimit: Locator;
  readonly startButton: Locator;
  readonly stopButton: Locator;
  readonly rebootButton: Locator;
  readonly consoleButton: Locator;
  readonly backupsTab: Locator;
  readonly settingsTab: Locator;

  constructor(page: Page) {
    super(page);
    this.hostname = page.locator('[data-testid="vm-hostname"]');
    this.status = page.locator('[data-testid="vm-status"]');
    this.ipAddresses = page.locator('[data-testid="ip-address"]');
    this.cpuUsage = page.locator('[data-testid="cpu-usage"]');
    this.memoryUsage = page.locator('[data-testid="memory-usage"]');
    this.diskUsage = page.locator('[data-testid="disk-usage"]');
    this.bandwidthUsed = page.locator('[data-testid="bandwidth-used"]');
    this.bandwidthLimit = page.locator('[data-testid="bandwidth-limit"]');
    this.startButton = page.locator('button:has-text("Start"), [data-testid="start-btn"]');
    this.stopButton = page.locator('button:has-text("Stop"), [data-testid="stop-btn"]');
    this.rebootButton = page.locator('button:has-text("Reboot"), [data-testid="reboot-btn"]');
    this.consoleButton = page.locator('button:has-text("Console"), a:has-text("Open Console")');
    this.backupsTab = page.locator('a:has-text("Backups"), [data-testid="backups-tab"]');
    this.settingsTab = page.locator('a:has-text("Settings"), [data-testid="settings-tab"]');
  }

  async goto(vmId: string): Promise<void> {
    await this.navigate(`/vms/${vmId}`);
  }

  async getHostname(): Promise<string | null> {
    return this.hostname.textContent();
  }

  async getStatus(): Promise<string> {
    return (await this.status.textContent()) || '';
  }

  async getIPAddresses(): Promise<string[]> {
    return this.ipAddresses.allInnerTexts();
  }

  async getResourceUsage(): Promise<{
    cpu: string | null;
    memory: string | null;
    disk: string | null;
  }> {
    return {
      cpu: await this.cpuUsage.textContent(),
      memory: await this.memoryUsage.textContent(),
      disk: await this.diskUsage.textContent(),
    };
  }

  async getBandwidthUsage(): Promise<{
    used: string | null;
    limit: string | null;
  }> {
    return {
      used: await this.bandwidthUsed.textContent(),
      limit: await this.bandwidthLimit.textContent(),
    };
  }

  async startVM(): Promise<void> {
    await this.startButton.click();
  }

  async stopVM(): Promise<void> {
    await this.stopButton.click();
  }

  async rebootVM(): Promise<void> {
    await this.rebootButton.click();
  }

  async openConsole(): Promise<void> {
    await this.consoleButton.click();
  }

  async navigateToBackups(): Promise<void> {
    await this.backupsTab.click();
  }

  async navigateToSettings(): Promise<void> {
    await this.settingsTab.click();
  }

  async waitForStatus(expectedStatus: string, timeout = 30000): Promise<void> {
    await expect(this.status).toContainText(expectedStatus, { timeout });
  }

  async expectActionAvailable(action: string): Promise<void> {
    await expect(this.page.locator(`button:has-text("${action}")`)).toBeEnabled();
  }

  async expectActionNotAvailable(action: string): Promise<void> {
    const btn = this.page.locator(`button:has-text("${action}")`);
    if (await btn.isVisible()) {
      await expect(btn).toBeDisabled();
    }
  }
}