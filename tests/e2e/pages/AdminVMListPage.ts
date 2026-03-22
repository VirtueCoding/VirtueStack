/**
 * Admin VM List Page Object Model
 */

import { Page, expect, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

export class AdminVMListPage extends BasePage {
  readonly pageTitle: Locator;
  readonly createVMButton: Locator;
  readonly searchInput: Locator;
  readonly statusFilter: Locator;
  readonly nodeFilter: Locator;
  readonly vmTable: Locator;
  readonly paginationInfo: Locator;
  readonly nextPageButton: Locator;
  readonly prevPageButton: Locator;
  readonly pageSizeSelect: Locator;

  constructor(page: Page) {
    super(page);
    this.pageTitle = page.locator('h1, [data-testid="page-title"]');
    this.createVMButton = page.locator('button:has-text("Create VM"), a:has-text("Create VM"), [data-testid="create-vm-btn"]');
    this.searchInput = page.locator('input[placeholder*="search" i], input[name="search"]');
    this.statusFilter = page.locator('[data-testid="status-filter"], select[name="status"]');
    this.nodeFilter = page.locator('[data-testid="node-filter"], select[name="node"]');
    this.vmTable = page.locator('table');
    this.paginationInfo = page.locator('[data-testid="pagination-info"], .pagination-info');
    this.nextPageButton = page.locator('button:has-text("Next"), [data-testid="next-page"]');
    this.prevPageButton = page.locator('button:has-text("Previous"), [data-testid="prev-page"]');
    this.pageSizeSelect = page.locator('[data-testid="page-size-select"], select[name="pageSize"]');
  }

  async goto(): Promise<void> {
    await this.navigate('/admin/vms');
    await expect(this.pageTitle).toContainText(/virtual machines|vms/i);
  }

  async clickCreateVM(): Promise<void> {
    await this.createVMButton.click();
    await expect(this.page).toHaveURL(/\/admin\/vms\/create/);
  }

  async searchVM(query: string): Promise<void> {
    await this.searchInput.fill(query);
    await this.searchInput.press('Enter');
    await this.page.waitForLoadState('networkidle');
  }

  async getVMRows(): Promise<Locator> {
    return this.page.locator('table tbody tr, [data-testid="vm-row"]');
  }

  async getVMCount(): Promise<number> {
    return await this.getVMRows().then(rows => rows.count());
  }

  async clickVMById(vmId: string): Promise<void> {
    await this.page.click(`a[href*="${vmId}"], [data-testid="vm-${vmId}"]`);
  }

  async clickVMByHostname(hostname: string): Promise<void> {
    await this.page.click(`text="${hostname}"`);
  }

  async expectVMInList(hostname: string): Promise<void> {
    await expect(this.page.locator(`text="${hostname}"`)).toBeVisible();
  }

  async expectVMNotInList(hostname: string): Promise<void> {
    await expect(this.page.locator(`text="${hostname}"`)).not.toBeVisible();
  }

  async filterByStatus(status: string): Promise<void> {
    await this.statusFilter.click();
    await this.page.click(`option:has-text("${status}")`);
    await this.page.waitForLoadState('networkidle');
  }

  async filterByNode(nodeName: string): Promise<void> {
    await this.nodeFilter.click();
    await this.page.click(`option:has-text("${nodeName}")`);
    await this.page.waitForLoadState('networkidle');
  }

  async goToNextPage(): Promise<void> {
    await this.nextPageButton.click();
    await this.page.waitForLoadState('networkidle');
  }

  async goToPreviousPage(): Promise<void> {
    await this.prevPageButton.click();
    await this.page.waitForLoadState('networkidle');
  }

  async changePageSize(size: number): Promise<void> {
    await this.pageSizeSelect.click();
    await this.page.click(`option:has-text("${size}")`);
    await this.page.waitForLoadState('networkidle');
  }

  async getPaginationText(): Promise<string | null> {
    return this.paginationInfo.textContent();
  }

  async expectTableVisible(): Promise<void> {
    await expect(this.vmTable).toBeVisible();
  }
}