/**
 * Customer VM List Page Object Model
 */

import { Page, expect, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

export class CustomerVMListPage extends BasePage {
  readonly pageTitle: Locator;
  readonly searchInput: Locator;
  readonly statusFilter: Locator;
  readonly vmCards: Locator;

  constructor(page: Page) {
    super(page);
    this.pageTitle = page.locator('h1, [data-testid="page-title"]');
    this.searchInput = page.locator('input[placeholder*="search" i], input[name="search"]');
    this.statusFilter = page.locator('[data-testid="status-filter"]');
    this.vmCards = page.locator('[data-testid="vm-card"], .vm-list-item');
  }

  async goto(): Promise<void> {
    await this.navigate('/vms');
    await expect(this.pageTitle).toContainText(/my servers|virtual machines|vms/i);
  }

  async getVMCount(): Promise<number> {
    return this.vmCards.count();
  }

  async getVMCards(): Promise<Locator> {
    return this.vmCards;
  }

  async searchVM(query: string): Promise<void> {
    await this.searchInput.fill(query);
    await this.searchInput.press('Enter');
    await this.page.waitForLoadState('networkidle');
  }

  async filterByStatus(status: string): Promise<void> {
    await this.statusFilter.click();
    await this.page.click(`text="${status}"`);
    await this.page.waitForLoadState('networkidle');
  }

  async clickVMByHostname(hostname: string): Promise<void> {
    await this.page.click(`text="${hostname}"`);
  }

  async expectVMVisible(hostname: string): Promise<void> {
    await expect(this.page.locator(`text="${hostname}"`)).toBeVisible();
  }

  async expectNoVMs(): Promise<void> {
    await expect(this.page.locator('text=/no.*vms|no servers found/i')).toBeVisible();
  }

  async quickStartVM(hostname: string): Promise<void> {
    const card = this.page.locator(`[data-testid="vm-card"]:has-text("${hostname}")`);
    await card.locator('button:has-text("Start")').click();
  }

  async quickStopVM(hostname: string): Promise<void> {
    const card = this.page.locator(`[data-testid="vm-card"]:has-text("${hostname}")`);
    await card.locator('button:has-text("Stop")').click();
  }
}