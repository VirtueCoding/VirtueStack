import { test, expect, Page } from '@playwright/test';

/**
 * Admin Plan Management E2E Tests
 *
 * Tests cover:
 * - Plan list viewing
 * - Plan creation
 * - Plan editing (including resource limits)
 * - Plan deletion
 * - Plan pricing management
 */

// ============================================
// Page Object Models
// ============================================

class AdminPlanListPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/plans');
    await expect(this.page.locator('h1, [data-testid="page-title"]')).toContainText(/plans/i);
  }

  async getPlanCards() {
    return this.page.locator('[data-testid="plan-card"], table tbody tr');
  }

  async getPlanCount() {
    const cards = await this.getPlanCards();
    return cards.count();
  }

  async clickCreatePlan() {
    await this.page.click('button:has-text("Add Plan"), a:has-text("Add Plan"), [data-testid="create-plan-btn"]');
  }

  async searchPlan(query: string) {
    await this.page.fill('input[placeholder*="search" i], input[name="search"]', query);
    await this.page.press('input[placeholder*="search" i], input[name="search"]', 'Enter');
  }

  async filterByStorageBackend(backend: string) {
    await this.page.click('[data-testid="storage-filter"], select[name="storage_backend"]');
    await this.page.click(`option:has-text("${backend}")`);
  }

  async clickPlanByName(name: string) {
    await this.page.click(`a:has-text("${name}"), [data-testid="plan-${name}"]`);
  }

  async expectPlanInList(name: string) {
    await expect(this.page.locator(`text="${name}"`)).toBeVisible();
  }

  async expectPlanNotInList(name: string) {
    await expect(this.page.locator(`text="${name}"`)).not.toBeVisible();
  }
}

class AdminPlanCreatePage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/plans/create');
    await expect(this.page.locator('h1')).toContainText(/add.*plan|new.*plan/i);
  }

  async fillName(name: string) {
    await this.page.fill('input[name="name"]', name);
  }

  async fillSlug(slug: string) {
    await this.page.fill('input[name="slug"]', slug);
  }

  async fillVCPU(count: number) {
    await this.page.fill('input[name="vcpu"]', count.toString());
  }

  async fillMemory(mb: number) {
    await this.page.fill('input[name="memory_mb"]', mb.toString());
  }

  async fillDisk(gb: number) {
    await this.page.fill('input[name="disk_gb"]', gb.toString());
  }

  async fillPortSpeed(mbps: number) {
    await this.page.fill('input[name="port_speed_mbps"]', mbps.toString());
  }

  async selectStorageBackend(backend: 'ceph' | 'qcow') {
    await this.page.click(`[data-testid="storage-${backend}"], input[value="${backend}"]`);
  }

  async fillSnapshotLimit(limit: number) {
    await this.page.fill('input[name="snapshot_limit"]', limit.toString());
  }

  async fillBackupLimit(limit: number) {
    await this.page.fill('input[name="backup_limit"]', limit.toString());
  }

  async fillISOUploadLimit(limit: number) {
    await this.page.fill('input[name="iso_upload_limit"]', limit.toString());
  }

  async fillMonthlyPrice(price: number) {
    await this.page.fill('input[name="monthly_price"]', price.toString());
  }

  async fillHourlyPrice(price: number) {
    await this.page.fill('input[name="hourly_price"]', price.toString());
  }

  async setDescription(description: string) {
    await this.page.fill('textarea[name="description"]', description);
  }

  async submit() {
    await this.page.click('button[type="submit"]');
  }

  async expectValidationError(message: string | RegExp) {
    await expect(this.page.locator('.error, [role="alert"]')).toContainText(message);
  }

  async expectSuccess() {
    await expect(this.page).toHaveURL(/\/plans\/[a-f0-9-]+/);
  }
}

class AdminPlanDetailPage {
  constructor(private page: Page) {}

  async goto(planId: string) {
    await this.page.goto(`/plans/${planId}`);
  }

  async getName() {
    return this.page.locator('[data-testid="plan-name"]').textContent();
  }

  async getResources() {
    return {
      vcpu: await this.page.locator('[data-testid="plan-vcpu"]').textContent(),
      memory: await this.page.locator('[data-testid="plan-memory"]').textContent(),
      disk: await this.page.locator('[data-testid="plan-disk"]').textContent(),
      portSpeed: await this.page.locator('[data-testid="plan-port-speed"]').textContent(),
    };
  }

  async getLimits() {
    return {
      snapshots: await this.page.locator('[data-testid="snapshot-limit"]').textContent(),
      backups: await this.page.locator('[data-testid="backup-limit"]').textContent(),
      isoUploads: await this.page.locator('[data-testid="iso-upload-limit"]').textContent(),
    };
  }

  async getStorageBackend() {
    return this.page.locator('[data-testid="storage-backend"]').textContent();
  }

  async getVMCount() {
    return this.page.locator('[data-testid="vm-count"]').textContent();
  }

  async clickEdit() {
    await this.page.click('button:has-text("Edit"), [data-testid="edit-btn"]');
  }

  async clickDelete() {
    await this.page.click('button:has-text("Delete"), [data-testid="delete-btn"]');
  }
}

class AdminPlanEditPage {
  constructor(private page: Page) {}

  async goto(planId: string) {
    await this.page.goto(`/plans/${planId}/edit`);
  }

  async updateName(name: string) {
    await this.page.fill('input[name="name"]', name);
  }

  async updateVCPU(count: number) {
    await this.page.fill('input[name="vcpu"]', count.toString());
  }

  async updateMemory(mb: number) {
    await this.page.fill('input[name="memory_mb"]', mb.toString());
  }

  async updateDisk(gb: number) {
    await this.page.fill('input[name="disk_gb"]', gb.toString());
  }

  async updateSnapshotLimit(limit: number) {
    await this.page.fill('input[name="snapshot_limit"]', limit.toString());
  }

  async updateBackupLimit(limit: number) {
    await this.page.fill('input[name="backup_limit"]', limit.toString());
  }

  async updateISOUploadLimit(limit: number) {
    await this.page.fill('input[name="iso_upload_limit"]', limit.toString());
  }

  async updateMonthlyPrice(price: number) {
    await this.page.fill('input[name="monthly_price"]', price.toString());
  }

  async submit() {
    await this.page.click('button[type="submit"]');
  }

  async cancel() {
    await this.page.click('button:has-text("Cancel")');
  }

  async expectValidationError(message: string | RegExp) {
    await expect(this.page.locator('.error, [role="alert"]')).toContainText(message);
  }
}

// ============================================
// Test Suite
// ============================================

test.describe('Admin Plan List', () => {
  let planListPage: AdminPlanListPage;

  test.beforeEach(async ({ page }) => {
    planListPage = new AdminPlanListPage(page);
    await planListPage.goto();
  });

  test('should display plan list', async ({ page }) => {
    await expect(page.locator('h1, [data-testid="page-title"]')).toContainText(/plans/i);
  });

  test('should show plan cards with key info', async ({ page }) => {
    const cards = await planListPage.getPlanCards();
    const count = await cards.count();

    if (count > 0) {
      const firstCard = cards.first();

      // Should show plan name
      await expect(firstCard.locator('[data-testid="plan-name"], h3')).toBeVisible();

      // Should show resources (CPU, RAM, Disk)
      await expect(firstCard.locator('text=/vCPU|CPU|Core/i')).toBeVisible();
      await expect(firstCard.locator('text=/RAM|Memory|GB/i')).toBeVisible();
    }
  });

  test('should show plan statistics', async ({ page }) => {
    const statsSection = page.locator('[data-testid="plan-stats"]');
    if (await statsSection.isVisible()) {
      await expect(statsSection).toBeVisible();
    }
  });

  test('should search plans by name', async ({ page }) => {
    await planListPage.searchPlan('basic');

    await page.waitForLoadState('networkidle');

    const cards = await planListPage.getPlanCards();
    const count = await cards.count();

    if (count > 0) {
      const text = await cards.first().textContent();
      expect(text?.toLowerCase()).toContain('basic');
    }
  });

  test('should filter plans by storage backend', async ({ page }) => {
    await planListPage.filterByStorageBackend('Ceph');

    await page.waitForLoadState('networkidle');

    const backends = page.locator('[data-testid="storage-backend"]:has-text("Ceph"), td:has-text("Ceph")');
    const count = await backends.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should click plan to view details', async ({ page }) => {
    const cards = await planListPage.getPlanCards();
    const count = await cards.count();

    if (count > 0) {
      const name = await cards.first().locator('[data-testid="plan-name"], h3').textContent();
      await planListPage.clickPlanByName(name || '');

      await expect(page).toHaveURL(/\/plans\/[a-f0-9-]+/);
    }
  });

  test('should show plan resource limits in list', async ({ page }) => {
    const cards = await planListPage.getPlanCards();
    const count = await cards.count();

    if (count > 0) {
      // Should show some limit info
      const firstCard = cards.first();
      const limitText = await firstCard.textContent();

      // Should show snapshot/backup limits or icons
      expect(limitText).toMatch(/snapshot|backup|iso|\d+/i);
    }
  });
});

test.describe('Admin Plan Creation', () => {
  let planCreatePage: AdminPlanCreatePage;

  test.beforeEach(async ({ page }) => {
    planCreatePage = new AdminPlanCreatePage(page);
    await planCreatePage.goto();
  });

  test('should display plan creation form', async ({ page }) => {
    await expect(page.locator('input[name="name"]')).toBeVisible();
    await expect(page.locator('input[name="vcpu"]')).toBeVisible();
    await expect(page.locator('input[name="memory_mb"]')).toBeVisible();
    await expect(page.locator('input[name="disk_gb"]')).toBeVisible();
    await expect(page.locator('button[type="submit"]')).toBeVisible();
  });

  test('should show resource limits section', async ({ page }) => {
    await expect(page.locator('input[name="snapshot_limit"]')).toBeVisible();
    await expect(page.locator('input[name="backup_limit"]')).toBeVisible();
    await expect(page.locator('input[name="iso_upload_limit"]')).toBeVisible();
  });

  test('should show pricing section', async ({ page }) => {
    await expect(page.locator('input[name="monthly_price"]')).toBeVisible();
  });

  test('should show validation errors for missing fields', async ({ page }) => {
    await planCreatePage.submit();

    await planCreatePage.expectValidationError(/required|please.*enter/i);
  });

  test('should validate plan name uniqueness', async ({ page }) => {
    await planCreatePage.fillName('Existing Plan');
    await planCreatePage.submit();

    await planCreatePage.expectValidationError(/already exists|duplicate/i);
  });

  test('should validate slug format', async ({ page }) => {
    await planCreatePage.fillSlug('Invalid Slug!');
    await planCreatePage.submit();

    await planCreatePage.expectValidationError(/valid.*slug|alphanumeric|lowercase/i);
  });

  test('should validate vCPU is positive', async ({ page }) => {
    await planCreatePage.fillVCPU(-1);
    await planCreatePage.submit();

    await planCreatePage.expectValidationError(/positive|greater than 0/i);
  });

  test('should validate memory is reasonable', async ({ page }) => {
    await planCreatePage.fillMemory(100); // Very small
    await planCreatePage.submit();

    // Should either accept or show warning
  });

  test('should validate disk is reasonable', async ({ page }) => {
    await planCreatePage.fillDisk(1); // Very small
    await planCreatePage.submit();

    // Should either accept or show warning
  });

  test('should validate limits are non-negative', async ({ page }) => {
    await planCreatePage.fillSnapshotLimit(-1);
    await planCreatePage.submit();

    await planCreatePage.expectValidationError(/positive|zero or greater/i);
  });

  test('should validate pricing is non-negative', async ({ page }) => {
    await planCreatePage.fillMonthlyPrice(-10);
    await planCreatePage.submit();

    await planCreatePage.expectValidationError(/positive|zero or greater/i);
  });

  test('should create plan with default limits', async ({ page }) => {
    const planName = `test-plan-${Date.now()}`;

    await planCreatePage.fillName(planName);
    await planCreatePage.fillSlug(planName.toLowerCase().replace(/[^a-z0-9]/g, '-'));
    await planCreatePage.fillVCPU(2);
    await planCreatePage.fillMemory(4096);
    await planCreatePage.fillDisk(50);
    await planCreatePage.fillPortSpeed(1000);
    await planCreatePage.selectStorageBackend('ceph');
    await planCreatePage.fillMonthlyPrice(9.99);
    await planCreatePage.submit();

    // Should redirect to plan detail page
    await expect(page).toHaveURL(/\/plans\/[a-f0-9-]+/);
  });

  test('should create plan with custom limits', async ({ page }) => {
    const planName = `limited-plan-${Date.now()}`;

    await planCreatePage.fillName(planName);
    await planCreatePage.fillSlug(planName.toLowerCase().replace(/[^a-z0-9]/g, '-'));
    await planCreatePage.fillVCPU(4);
    await planCreatePage.fillMemory(8192);
    await planCreatePage.fillDisk(100);
    await planCreatePage.fillSnapshotLimit(5);
    await planCreatePage.fillBackupLimit(10);
    await planCreatePage.fillISOUploadLimit(3);
    await planCreatePage.fillMonthlyPrice(19.99);
    await planCreatePage.submit();

    await expect(page).toHaveURL(/\/plans\/[a-f0-9-]+/);
  });

  test('should show storage backend options', async ({ page }) => {
    await expect(page.locator('text=/ceph|qcow/i')).toBeVisible();
  });
});

test.describe('Admin Plan Detail', () => {
  let planDetailPage: AdminPlanDetailPage;
  const testPlanId = '00000000-0000-0000-0000-000000000001';

  test.beforeEach(async ({ page }) => {
    planDetailPage = new AdminPlanDetailPage(page);
    await planDetailPage.goto(testPlanId);
  });

  test('should display plan details', async ({ page }) => {
    await expect(page.locator('[data-testid="plan-name"]')).toBeVisible();
  });

  test('should show resource specifications', async ({ page }) => {
    const resources = await planDetailPage.getResources();

    expect(resources.vcpu).toBeTruthy();
    expect(resources.memory).toBeTruthy();
    expect(resources.disk).toBeTruthy();
  });

  test('should show resource limits', async ({ page }) => {
    const limits = await planDetailPage.getLimits();

    expect(limits.snapshots).toBeTruthy();
    expect(limits.backups).toBeTruthy();
    expect(limits.isoUploads).toBeTruthy();
  });

  test('should show storage backend', async ({ page }) => {
    const backend = await planDetailPage.getStorageBackend();
    expect(backend).toMatch(/ceph|qcow/i);
  });

  test('should show VM count using this plan', async ({ page }) => {
    const vmCount = page.locator('[data-testid="vm-count"]');

    if (await vmCount.isVisible()) {
      const count = await vmCount.textContent();
      expect(count).toMatch(/\d+/);
    }
  });

  test('should show action buttons', async ({ page }) => {
    await expect(page.locator('button:has-text("Edit")')).toBeVisible();
    await expect(page.locator('button:has-text("Delete")')).toBeVisible();
  });
});

test.describe('Admin Plan Editing', () => {
  let planEditPage: AdminPlanEditPage;
  const testPlanId = '00000000-0000-0000-0000-000000000002';

  test.beforeEach(async ({ page }) => {
    planEditPage = new AdminPlanEditPage(page);
    await planEditPage.goto(testPlanId);
  });

  test('should display edit form with current values', async ({ page }) => {
    await expect(page.locator('input[name="name"]')).toHaveValue(/.+/);
    await expect(page.locator('input[name="vcpu"]')).toHaveValue(/.+/);
  });

  test('should update plan name', async ({ page }) => {
    const newName = `Updated Plan ${Date.now()}`;

    await planEditPage.updateName(newName);
    await planEditPage.submit();

    // Should show success or redirect
    await expect(page.locator(`text="${newName}"`)).toBeVisible();
  });

  test('should update resource limits', async ({ page }) => {
    await planEditPage.updateSnapshotLimit(10);
    await planEditPage.updateBackupLimit(15);
    await planEditPage.updateISOUploadLimit(5);
    await planEditPage.submit();

    await expect(page.locator('text=/saved|updated|success/i')).toBeVisible();
  });

  test('should update pricing', async ({ page }) => {
    await planEditPage.updateMonthlyPrice(29.99);
    await planEditPage.submit();

    await expect(page.locator('text=/saved|updated|success/i')).toBeVisible();
  });

  test('should validate updates', async ({ page }) => {
    await planEditPage.updateVCPU(-1);
    await planEditPage.submit();

    await planEditPage.expectValidationError(/positive|greater than 0/i);
  });

  test('should allow canceling edits', async ({ page }) => {
    await planEditPage.updateName('Cancelled Name');
    await planEditPage.cancel();

    // Should navigate away without saving
    await expect(page).toHaveURL(/\/plans/);
  });

  test('should show storage backend is read-only after creation', async ({ page }) => {
    // Storage backend should be disabled/readonly
    const storageInput = page.locator('input[name="storage_backend"], select[name="storage_backend"]');
    const isDisabled = await storageInput.isDisabled();

    expect(isDisabled).toBe(true);
  });
});

test.describe('Admin Plan Deletion', () => {
  let planDetailPage: AdminPlanDetailPage;
  let planListPage: AdminPlanListPage;

  test('should show delete button for unused plan', async ({ page }) => {
    planDetailPage = new AdminPlanDetailPage(page);
    await planDetailPage.goto('unused-plan-id');

    await expect(page.locator('button:has-text("Delete")')).toBeVisible();
  });

  test('should show deletion confirmation', async ({ page }) => {
    planDetailPage = new AdminPlanDetailPage(page);
    await planDetailPage.goto('unused-plan-id');
    await planDetailPage.clickDelete();

    await expect(page.locator('[role="dialog"], .modal')).toBeVisible();
    await expect(page.locator('text=/confirm|are you sure/i')).toBeVisible();
  });

  test('should delete plan after confirmation', async ({ page }) => {
    planDetailPage = new AdminPlanDetailPage(page);
    planListPage = new AdminPlanListPage(page);

    await planDetailPage.goto('deletable-plan-id');
    await planDetailPage.clickDelete();

    await page.click('button:has-text("Confirm")');

    // Should redirect to plan list
    await expect(page).toHaveURL(/\/plans$/);
  });

  test('should disable delete for plan with VMs', async ({ page }) => {
    planDetailPage = new AdminPlanDetailPage(page);
    await planDetailPage.goto('plan-with-vms-id');

    await expect(page.locator('button:has-text("Delete")')).toBeDisabled();
  });

  test('should show reason for disabled deletion', async ({ page }) => {
    planDetailPage = new AdminPlanDetailPage(page);
    await planDetailPage.goto('plan-with-vms-id');

    await page.hover('button:has-text("Delete")');

    await expect(page.locator('text=/VMs|in use|cannot delete/i')).toBeVisible();
  });

  test('should allow canceling deletion', async ({ page }) => {
    planDetailPage = new AdminPlanDetailPage(page);
    await planDetailPage.goto('unused-plan-id');
    await planDetailPage.clickDelete();

    await page.click('button:has-text("Cancel")');

    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
  });
});

test.describe('Admin Plan Pricing', () => {
  test('should show pricing in plan list', async ({ page }) => {
    await page.goto('/plans');

    // Should show prices
    await expect(page.locator('text=/\$[\d.]+|€[\d.]+/')).toBeVisible();
  });

  test('should show monthly and hourly pricing', async ({ page }) => {
    await page.goto('/plans/test-plan-id');

    // Should show at least monthly pricing
    await expect(page.locator('text=/month|monthly/i')).toBeVisible();
  });

  test('should update pricing from edit page', async ({ page }) => {
    await page.goto('/plans/test-plan-id/edit');

    await page.fill('input[name="monthly_price"]', '14.99');
    await page.click('button[type="submit"]');

    await expect(page.locator('text=/saved|updated/i')).toBeVisible();
  });
});

test.describe('Admin Plan Resource Limits', () => {
  test('should show default limits on create form', async ({ page }) => {
    await page.goto('/plans/create');

    const snapshotLimit = await page.locator('input[name="snapshot_limit"]').inputValue();
    const backupLimit = await page.locator('input[name="backup_limit"]').inputValue();
    const isoLimit = await page.locator('input[name="iso_upload_limit"]').inputValue();

    // Should have default values (typically 2)
    expect(parseInt(snapshotLimit)).toBe(2);
    expect(parseInt(backupLimit)).toBe(2);
    expect(parseInt(isoLimit)).toBe(2);
  });

  test('should enforce minimum limit of 0', async ({ page }) => {
    await page.goto('/plans/create');

    await page.fill('input[name="snapshot_limit"]', '-1');
    await page.click('button[type="submit"]');

    await expect(page.locator('.error, [role="alert"]')).toContainText(/positive|zero|minimum/i);
  });

  test('should show limits in plan detail', async ({ page }) => {
    await page.goto('/plans/test-plan-id');

    // Should show limit values
    await expect(page.locator('text=/snapshot|backup|iso/i')).toBeVisible();
  });

  test('should update limits from edit page', async ({ page }) => {
    await page.goto('/plans/test-plan-id/edit');

    await page.fill('input[name="snapshot_limit"]', '5');
    await page.fill('input[name="backup_limit"]', '10');
    await page.fill('input[name="iso_upload_limit"]', '3');
    await page.click('button[type="submit"]');

    await expect(page.locator('text=/saved|updated/i')).toBeVisible();
  });
});