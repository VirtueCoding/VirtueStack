/**
 * Admin Dashboard Page Object Model
 */

import { Page, expect, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

export class AdminDashboardPage extends BasePage {
  readonly pageTitle: Locator;
  readonly totalVMsStat: Locator;
  readonly runningVMsStat: Locator;
  readonly totalNodesStat: Locator;
  readonly totalCustomersStat: Locator;

  constructor(page: Page) {
    super(page);
    this.pageTitle = page.locator('h1, [data-testid="dashboard-title"]');
    this.totalVMsStat = page.locator('[data-testid="total-vms"]');
    this.runningVMsStat = page.locator('[data-testid="running-vms"]');
    this.totalNodesStat = page.locator('[data-testid="total-nodes"]');
    this.totalCustomersStat = page.locator('[data-testid="total-customers"]');
  }

  async goto(): Promise<void> {
    await this.navigate('/admin/dashboard');
    await expect(this.pageTitle).toContainText(/dashboard/i);
  }

  async getQuickStats(): Promise<{
    totalVMs: string | null;
    runningVMs: string | null;
    totalNodes: string | null;
    totalCustomers: string | null;
  }> {
    return {
      totalVMs: await this.totalVMsStat.textContent(),
      runningVMs: await this.runningVMsStat.textContent(),
      totalNodes: await this.totalNodesStat.textContent(),
      totalCustomers: await this.totalCustomersStat.textContent(),
    };
  }

  async navigateToVMs(): Promise<void> {
    await this.page.click('a:has-text("Virtual Machines"), a[href*="/admin/vms"]');
    await expect(this.page).toHaveURL(/\/admin\/vms/);
  }

  async navigateToNodes(): Promise<void> {
    await this.page.click('a:has-text("Nodes"), a[href*="/admin/nodes"]');
    await expect(this.page).toHaveURL(/\/admin\/nodes/);
  }

  async navigateToCustomers(): Promise<void> {
    await this.page.click('a:has-text("Customers"), a[href*="/admin/customers"]');
    await expect(this.page).toHaveURL(/\/admin\/customers/);
  }
}