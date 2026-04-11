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
    this.hostname = page.locator('h1');
    this.status = page.locator('h1').locator('xpath=following-sibling::*[1]');
    this.ipAddresses = page.locator('p.font-mono.text-lg');
    this.cpuUsage = page.locator('text=/Virtual CPUs/i');
    this.memoryUsage = page.locator('text=/RAM allocated/i');
    this.diskUsage = page.locator('text=/Storage/i');
    this.bandwidthUsed = page.locator('text=/Network/i');
    this.bandwidthLimit = page.locator('text=/Metrics/i');
    this.startButton = page.locator('button:has-text("Start")');
    this.stopButton = page.locator('button:has-text("Stop")');
    this.rebootButton = page.locator('button:has-text("Restart")');
    this.consoleButton = page.locator('[role="tab"]:has-text("VNC"), button:has-text("Connect to Console")');
    this.backupsTab = page.locator('[role="tab"]:has-text("Backups & Snapshots")');
    this.settingsTab = page.locator('[role="tab"]:has-text("Settings")');
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
    await this.page.locator('[role="tab"]:has-text("VNC")').click();
    const connectButton = this.page.locator('button:has-text("Connect to Console")');
    if (await connectButton.isVisible().catch(() => false)) {
      await connectButton.click();
    }
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
