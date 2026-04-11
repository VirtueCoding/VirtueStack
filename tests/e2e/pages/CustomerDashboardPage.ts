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
  readonly vmRows: Locator;
  readonly statusBadges: Locator;

  constructor(page: Page) {
    super(page);
    this.pageTitle = page.locator('body');
    this.totalVMsStat = page.locator('[data-testid="total-vms"]');
    this.runningVMsStat = page.locator('[data-testid="running-vms"]');
    this.bandwidthUsedStat = page.locator('[data-testid="bandwidth-used"]');
    this.vmRows = page.locator('table tbody tr');
    this.statusBadges = page.locator('table tbody tr td:nth-child(2)');
  }

  async goto(): Promise<void> {
    await this.navigate('/vms');
    await expect(this.pageTitle).toContainText(/virtual machines|no virtual machines/i);
  }

  async getQuickStats(): Promise<{
    totalVMs: string | null;
    runningVMs: string | null;
    bandwidthUsed: string | null;
  }> {
    const statuses = await this.statusBadges.allInnerTexts();

    return {
      totalVMs: String(await this.vmRows.count()),
      runningVMs: String(statuses.filter((status) => /running/i.test(status)).length),
      bandwidthUsed: 'N/A',
    };
  }

  async navigateToVMs(): Promise<void> {
    await this.navigate('/vms');
    await expect(this.page).toHaveURL(/\/vms/);
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
