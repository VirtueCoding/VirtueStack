/**
 * Customer Dashboard Page Object Model
 */

import { Page, expect, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

export class CustomerDashboardPage extends BasePage {
  readonly pageTitle: Locator;
  readonly totalVMsStat: Locator;
  readonly runningVMsStat: Locator;
  readonly bandwidthUsedStat: Locator;

  constructor(page: Page) {
    super(page);
    this.pageTitle = page.locator('h1, [data-testid="dashboard-title"]');
    this.totalVMsStat = page.locator('[data-testid="total-vms"]');
    this.runningVMsStat = page.locator('[data-testid="running-vms"]');
    this.bandwidthUsedStat = page.locator('[data-testid="bandwidth-used"]');
  }

  async goto(): Promise<void> {
    await this.navigate('/dashboard');
    await expect(this.pageTitle).toContainText(/dashboard|overview/i);
  }

  async getQuickStats(): Promise<{
    totalVMs: string | null;
    runningVMs: string | null;
    bandwidthUsed: string | null;
  }> {
    return {
      totalVMs: await this.totalVMsStat.textContent(),
      runningVMs: await this.runningVMsStat.textContent(),
      bandwidthUsed: await this.bandwidthUsedStat.textContent(),
    };
  }

  async navigateToVMs(): Promise<void> {
    await this.page.click('a:has-text("My Servers"), a[href*="/vms"], nav a:has-text("Servers")');
    await expect(this.page).toHaveURL(/\/vms|\/servers/);
  }

  async navigateToBackups(): Promise<void> {
    await this.page.click('a:has-text("Backups"), a[href*="/backups"]');
    await expect(this.page).toHaveURL(/\/backups/);
  }

  async navigateToSettings(): Promise<void> {
    await this.page.click('a:has-text("Settings"), a[href*="/settings"]');
    await expect(this.page).toHaveURL(/\/settings/);
  }
}