/**
 * Customer VM List Page Object Model
 */

import { Page, expect, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

export class CustomerVMListPage extends BasePage {
  readonly pageTitle: Locator;
  readonly pageDescription: Locator;
  readonly searchInput: Locator;
  readonly vmRows: Locator;

  constructor(page: Page) {
    super(page);
    this.pageTitle = page.getByText('Virtual Machines').first();
    this.pageDescription = page.getByText('Manage and monitor your virtual machines');
    this.searchInput = page.getByPlaceholder('Search by name, hostname or IP...');
    this.vmRows = page.locator('tbody tr');
  }

  async goto(): Promise<void> {
    await this.navigate('/vms');
    await expect(this.pageTitle).toBeVisible();
    await expect(this.pageDescription).toBeVisible();
    await expect(this.searchInput).toBeVisible();
  }

  async getVMCount(): Promise<number> {
    return this.vmRows.count();
  }

  async getVMCards(): Promise<Locator> {
    return this.vmRows;
  }

  getRowByHostname(hostname: string): Locator {
    return this.vmRows.filter({ hasText: hostname });
  }

  async searchVM(query: string): Promise<void> {
    await this.searchInput.fill(query);
  }

  async expectVMVisible(hostname: string): Promise<void> {
    await expect(this.getRowByHostname(hostname)).toBeVisible();
  }

  async expectNoVMs(): Promise<void> {
    await expect(this.page.getByText('No Virtual Machines')).toBeVisible();
  }

  async quickStartVM(hostname: string): Promise<void> {
    await this.getRowByHostname(hostname).locator('button[title="Start VM"]').click();
  }

  async quickStopVM(hostname: string): Promise<void> {
    await this.getRowByHostname(hostname).locator('button[title="Stop VM"]').click();
  }
}
