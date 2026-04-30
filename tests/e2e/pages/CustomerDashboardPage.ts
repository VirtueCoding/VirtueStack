/**
 * Customer landing page object model.
 */

import { Page, expect, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

export class CustomerDashboardPage extends BasePage {
  readonly pageTitle: Locator;
  readonly pageDescription: Locator;
  readonly totalVMsStat: Locator;
  readonly runningVMsStat: Locator;
  readonly myVMsLink: Locator;

  constructor(page: Page) {
    super(page);
    this.pageTitle = page.getByText('Virtual Machines').first();
    this.pageDescription = page.getByText('Manage and monitor your virtual machines');
    this.totalVMsStat = page.locator('tbody tr');
    this.runningVMsStat = page.locator('tbody tr').filter({ hasText: 'Running' });
    this.myVMsLink = page.getByRole('link', { name: 'My VMs' });
  }

  async goto(): Promise<void> {
    await this.navigate('/');
    await expect(this.page).toHaveURL(/\/vms$/);
    await expect(this.pageDescription).toBeVisible();
  }

  async getQuickStats(): Promise<{
    totalVMs: string | null;
    runningVMs: string | null;
    bandwidthUsed: string | null;
  }> {
    return {
      totalVMs: String(await this.totalVMsStat.count()),
      runningVMs: String(await this.runningVMsStat.count()),
      bandwidthUsed: null,
    };
  }

  async navigateToVMs(): Promise<void> {
    await this.myVMsLink.click();
    await expect(this.page).toHaveURL(/\/vms$/);
  }
}
