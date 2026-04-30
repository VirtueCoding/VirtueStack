/**
 * Admin VM List Page Object Model
 */

import { Page, expect, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

export class AdminVMListPage extends BasePage {
  readonly pageTitle: Locator;
  readonly createVMButton: Locator;
  readonly searchInput: Locator;
  readonly vmTable: Locator;
  readonly summaryText: Locator;

  constructor(page: Page) {
    super(page);
    this.pageTitle = page.locator('h1, [data-testid="page-title"]');
    this.createVMButton = page.getByRole('button', { name: 'Create VM' });
    this.searchInput = page.getByPlaceholder('Search VMs by ID, name, status...');
    this.vmTable = page.locator('table');
    this.summaryText = page.getByText(/\d+ of \d+ VMs displayed/i);
  }

  async goto(): Promise<void> {
    await this.navigate('/vms');
    await expect(this.pageTitle).toContainText(/virtual machines|vms/i);
  }

  rowByName(name: string): Locator {
    return this.page.locator('tbody tr').filter({ has: this.page.getByText(name, { exact: true }) });
  }

  async openCreateVMDialog(): Promise<void> {
    await this.createVMButton.click();
    await expect(this.page.getByRole('dialog').getByRole('heading', { name: 'Create Virtual Machine' })).toBeVisible();
  }

  async searchVM(query: string): Promise<void> {
    await this.searchInput.fill(query);
  }

  async getVMRows(): Promise<Locator> {
    return this.page.locator('table tbody tr');
  }

  async getVMCount(): Promise<number> {
    return await this.getVMRows().then(rows => rows.count());
  }

  async expectVMInList(hostname: string): Promise<void> {
    await expect(this.rowByName(hostname)).toBeVisible();
  }

  async expectVMNotInList(hostname: string): Promise<void> {
    await expect(this.rowByName(hostname)).toHaveCount(0);
  }

  async getSummaryText(): Promise<string | null> {
    return this.summaryText.textContent();
  }

  async openViewDialog(name: string): Promise<void> {
    await this.rowByName(name).locator('button[title="View Details"]').click();
    await expect(this.page.getByRole('dialog').getByRole('heading', { name: 'Virtual Machine Details' })).toBeVisible();
  }

  async openEditDialog(name: string): Promise<void> {
    await this.rowByName(name).locator('button[title="Edit VM"]').click();
    await expect(this.page.getByRole('dialog').getByRole('heading', { name: `Edit VM: ${name}` })).toBeVisible();
  }

  async openDeleteDialog(name: string): Promise<void> {
    await this.rowByName(name).locator('button[title="Delete VM"]').click();
    await expect(this.page.getByRole('dialog').getByRole('heading', { name: 'Delete Virtual Machine' })).toBeVisible();
  }

  async expectTableVisible(): Promise<void> {
    await expect(this.vmTable).toBeVisible();
  }
}
