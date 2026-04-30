/**
 * Customer VM Detail Page Object Model
 */

import { Page, expect, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

export class CustomerVMDetailPage extends BasePage {
  readonly backButton: Locator;
  readonly title: Locator;
  readonly ipAddresses: Locator;
  readonly startButton: Locator;
  readonly stopButton: Locator;
  readonly forceStopButton: Locator;
  readonly rebootButton: Locator;
  readonly vncTab: Locator;
  readonly serialTab: Locator;
  readonly networkTab: Locator;
  readonly backupsTab: Locator;
  readonly metricsTab: Locator;
  readonly settingsTab: Locator;
  readonly connectConsoleButton: Locator;

  constructor(page: Page) {
    super(page);
    this.backButton = page.getByRole('button', { name: 'Back' });
    this.title = page.getByRole('heading', { level: 1 });
    this.ipAddresses = page.locator('p.font-mono.text-lg');
    this.startButton = page.getByRole('button', { name: 'Start' });
    this.stopButton = page.getByRole('button', { name: 'Stop', exact: true });
    this.forceStopButton = page.getByRole('button', { name: 'Force Stop' });
    this.rebootButton = page.getByRole('button', { name: 'Restart' });
    this.vncTab = page.getByRole('tab', { name: 'VNC' });
    this.serialTab = page.getByRole('tab', { name: 'Serial' });
    this.networkTab = page.getByRole('tab', { name: 'Network' });
    this.backupsTab = page.getByRole('tab', { name: /Backups & Snapshots/i });
    this.metricsTab = page.getByRole('tab', { name: 'Metrics' });
    this.settingsTab = page.getByRole('tab', { name: 'Settings' });
    this.connectConsoleButton = page.getByRole('button', { name: 'Connect to Console' });
  }

  async goto(vmId: string): Promise<void> {
    await this.navigate(`/vms/${vmId}`);
    await expect(this.backButton).toBeVisible();
  }

  async getHostname(): Promise<string | null> {
    return this.title.textContent();
  }

  async getStatus(): Promise<string> {
    const labels = ['Running', 'Stopped', 'Suspended', 'Error', 'Provisioning', 'Migrating', 'Reinstalling'];
    for (const label of labels) {
      if (await this.page.getByText(new RegExp(`^${label}$`)).isVisible().catch(() => false)) {
        return label;
      }
    }
    return '';
  }

  async getIPAddresses(): Promise<string[]> {
    return this.ipAddresses.allInnerTexts();
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

  async openForceStopDialog(): Promise<void> {
    await this.forceStopButton.click();
  }

  async openConsole(): Promise<void> {
    await this.vncTab.click();
  }

  async navigateToBackups(): Promise<void> {
    await this.backupsTab.click();
  }

  async navigateToMetrics(): Promise<void> {
    await this.metricsTab.click();
  }

  async navigateToSettings(): Promise<void> {
    await this.settingsTab.click();
  }

  async navigateToSerial(): Promise<void> {
    await this.serialTab.click();
  }

  async expectActionAvailable(action: string): Promise<void> {
    await expect(this.page.getByRole('button', { name: action })).toBeEnabled();
  }

  async expectActionNotAvailable(action: string): Promise<void> {
    const button = this.page.getByRole('button', { name: action });
    if (await button.isVisible().catch(() => false)) {
      await expect(button).toBeDisabled();
    }
  }
}
