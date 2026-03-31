/**
 * Admin VM Detail Page Object Model
 */

import { Page, expect, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

export class AdminVMDetailPage extends BasePage {
  readonly hostname: Locator;
  readonly status: Locator;
  readonly vcpu: Locator;
  readonly memory: Locator;
  readonly disk: Locator;
  readonly startButton: Locator;
  readonly stopButton: Locator;
  readonly rebootButton: Locator;
  readonly forceStopButton: Locator;
  readonly deleteButton: Locator;
  readonly consoleLink: Locator;
  readonly backupsLink: Locator;
  readonly auditLogLink: Locator;

  constructor(page: Page) {
    super(page);
    this.hostname = page.locator('[data-testid="vm-hostname"], h1');
    this.status = page.locator('[data-testid="vm-status"], .status-badge');
    this.vcpu = page.locator('[data-testid="vm-vcpu"]');
    this.memory = page.locator('[data-testid="vm-memory"]');
    this.disk = page.locator('[data-testid="vm-disk"]');
    this.startButton = page.locator('button:has-text("Start"), [data-testid="start-btn"]');
    this.stopButton = page.locator('button:has-text("Stop"), [data-testid="stop-btn"]');
    this.rebootButton = page.locator('button:has-text("Reboot"), [data-testid="reboot-btn"]');
    this.forceStopButton = page.locator('button:has-text("Force Stop"), [data-testid="force-stop-btn"]');
    this.deleteButton = page.locator('button:has-text("Delete"), [data-testid="delete-btn"]');
    this.consoleLink = page.locator('a:has-text("Console"), [data-testid="console-link"]');
    this.backupsLink = page.locator('a:has-text("Backups"), [data-testid="backups-link"]');
    this.auditLogLink = page.locator('a:has-text("Audit Log"), [data-testid="audit-log-link"]');
  }

  async goto(vmId: string): Promise<void> {
    await this.navigate(`/admin/vms/${vmId}`);
  }

  async getHostname(): Promise<string | null> {
    return this.hostname.textContent();
  }

  async getStatus(): Promise<string> {
    return (await this.status.textContent()) || '';
  }

  async getVMInfo(): Promise<{
    hostname: string | null;
    status: string;
    vcpu: string | null;
    memory: string | null;
    disk: string | null;
  }> {
    return {
      hostname: await this.getHostname(),
      status: await this.getStatus(),
      vcpu: await this.vcpu.textContent(),
      memory: await this.memory.textContent(),
      disk: await this.disk.textContent(),
    };
  }

  async getIPAddresses(): Promise<string[]> {
    const ips = this.page.locator('[data-testid="ip-address"]');
    return ips.allInnerTexts();
  }

  async startVM(): Promise<void> {
    await this.startButton.click();
    await this.waitForStatus('running');
  }

  async stopVM(): Promise<void> {
    await this.stopButton.click();
    await this.waitForStatus('stopped');
  }

  async rebootVM(): Promise<void> {
    await this.rebootButton.click();
  }

  async forceStopVM(): Promise<void> {
    await this.forceStopButton.click();
    await this.waitForStatus('stopped');
  }

  async deleteVM(): Promise<void> {
    await this.deleteButton.click();
    // Confirm deletion in modal
    await this.page.click('button:has-text("Confirm"), [data-testid="confirm-delete"]');
  }

  async cancelDeletion(): Promise<void> {
    await this.deleteButton.click();
    await this.page.click('button:has-text("Cancel")');
  }

  async waitForStatus(expectedStatus: string, timeout = 30000): Promise<void> {
    await expect(this.status).toContainText(expectedStatus, { timeout });
  }

  async navigateToConsole(): Promise<void> {
    await this.consoleLink.click();
  }

  async navigateToBackups(): Promise<void> {
    await this.backupsLink.click();
  }

  async navigateToAuditLog(): Promise<void> {
    await this.auditLogLink.click();
  }

  async expectActionDisabled(action: string): Promise<void> {
    const btn = this.page.locator(`button:has-text("${action}")`);
    await expect(btn).toBeDisabled();
  }

  async expectActionEnabled(action: string): Promise<void> {
    const btn = this.page.locator(`button:has-text("${action}")`);
    await expect(btn).toBeEnabled();
  }

  async expectHostname(expected: string): Promise<void> {
    await expect(this.hostname).toContainText(expected);
  }
}