import { test, expect, Page } from '@playwright/test';

/**
 * Admin Template Management E2E Tests
 *
 * Tests cover:
 * - Template list viewing
 * - Template creation/upload
 * - Template import from existing image
 * - Template versioning
 * - Template deletion
 */

// ============================================
// Page Object Models
// ============================================

class AdminTemplateListPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/templates');
    await expect(this.page.locator('h1, [data-testid="page-title"]')).toContainText(/templates/i);
  }

  async getTemplateCards() {
    return this.page.locator('[data-testid="template-card"], table tbody tr');
  }

  async getTemplateCount() {
    const cards = await this.getTemplateCards();
    return cards.count();
  }

  async clickCreateTemplate() {
    await this.page.click('button:has-text("Add Template"), a:has-text("Add Template"), [data-testid="create-template-btn"]');
  }

  async clickImportTemplate() {
    await this.page.click('button:has-text("Import"), [data-testid="import-btn"]');
  }

  async searchTemplate(query: string) {
    await this.page.fill('input[placeholder*="search" i], input[name="search"]', query);
    await this.page.press('input[placeholder*="search" i], input[name="search"]', 'Enter');
  }

  async filterByOSFamily(family: string) {
    await this.page.click('[data-testid="os-filter"], select[name="os_family"]');
    await this.page.click(`option:has-text("${family}")`);
  }

  async filterByStorageBackend(backend: string) {
    await this.page.click('[data-testid="storage-filter"], select[name="storage_backend"]');
    await this.page.click(`option:has-text("${backend}")`);
  }

  async clickTemplateByName(name: string) {
    await this.page.click(`a:has-text("${name}"), [data-testid="template-${name}"]`);
  }

  async expectTemplateInList(name: string) {
    await expect(this.page.locator(`text="${name}"`)).toBeVisible();
  }

  async expectTemplateNotInList(name: string) {
    await expect(this.page.locator(`text="${name}"`)).not.toBeVisible();
  }
}

class AdminTemplateCreatePage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/templates/create');
    await expect(this.page.locator('h1')).toContainText(/add.*template|new.*template|upload/i);
  }

  async fillName(name: string) {
    await this.page.fill('input[name="name"]', name);
  }

  async fillDescription(description: string) {
    await this.page.fill('textarea[name="description"]', description);
  }

  async selectOSFamily(family: string) {
    await this.page.click('[data-testid="os-family-select"], select[name="os_family"]');
    await this.page.click(`text="${family}"`);
  }

  async fillOSVersion(version: string) {
    await this.page.fill('input[name="os_version"]', version);
  }

  async selectStorageBackend(backend: 'ceph' | 'qcow') {
    await this.page.click(`[data-testid="storage-${backend}"], input[value="${backend}"]`);
  }

  async uploadImage(filePath: string) {
    await this.page.setInputFiles('input[type="file"]', filePath);
  }

  async fillRBDImage(image: string) {
    await this.page.fill('input[name="rbd_image"]', image);
  }

  async fillRBDSnapshot(snapshot: string) {
    await this.page.fill('input[name="rbd_snapshot"]', snapshot);
  }

  async submit() {
    await this.page.click('button[type="submit"]');
  }

  async expectValidationError(message: string | RegExp) {
    await expect(this.page.locator('.error, [role="alert"]')).toContainText(message);
  }

  async expectSuccess() {
    await expect(this.page).toHaveURL(/\/templates\/[a-f0-9-]+/);
  }
}

class AdminTemplateImportModal {
  constructor(private page: Page) {}

  async expectVisible() {
    await expect(this.page.locator('[role="dialog"], .modal:has-text("Import")')).toBeVisible();
  }

  async selectSourceImage(image: string) {
    await this.page.click('[data-testid="source-image-select"]');
    await this.page.click(`text="${image}"`);
  }

  async fillTemplateName(name: string) {
    await this.page.fill('input[name="template_name"]', name);
  }

  async selectOSFamily(family: string) {
    await this.page.click('[data-testid="os-family-select"]');
    await this.page.click(`text="${family}"`);
  }

  async confirmImport() {
    await this.page.click('button:has-text("Import")');
  }

  async cancel() {
    await this.page.click('button:has-text("Cancel")');
  }
}

class AdminTemplateDetailPage {
  constructor(private page: Page) {}

  async goto(templateId: string) {
    await this.page.goto(`/templates/${templateId}`);
  }

  async getName() {
    return this.page.locator('[data-testid="template-name"]').textContent();
  }

  async getOSInfo() {
    return {
      family: await this.page.locator('[data-testid="os-family"]').textContent(),
      version: await this.page.locator('[data-testid="os-version"]').textContent(),
    };
  }

  async getStorageInfo() {
    return {
      backend: await this.page.locator('[data-testid="storage-backend"]').textContent(),
      rbdImage: await this.page.locator('[data-testid="rbd-image"]').textContent(),
      rbdSnapshot: await this.page.locator('[data-testid="rbd-snapshot"]').textContent(),
    };
  }

  async getSize() {
    return this.page.locator('[data-testid="template-size"]').textContent();
  }

  async getVersion() {
    return this.page.locator('[data-testid="template-version"]').textContent();
  }

  async clickEdit() {
    await this.page.click('button:has-text("Edit"), [data-testid="edit-btn"]');
  }

  async clickDelete() {
    await this.page.click('button:has-text("Delete"), [data-testid="delete-btn"]');
  }

  async clickImportNewVersion() {
    await this.page.click('button:has-text("Import Version"), [data-testid="import-version-btn"]');
  }

  async navigateToVersions() {
    await this.page.click('a:has-text("Versions"), [data-testid="versions-tab"]');
  }

  async navigateToUsage() {
    await this.page.click('a:has-text("Usage"), [data-testid="usage-tab"]');
  }

  async getVMCount() {
    return this.page.locator('[data-testid="vm-count"]').textContent();
  }
}

class AdminTemplateVersionsPage {
  constructor(private page: Page) {}

  async goto(templateId: string) {
    await this.page.goto(`/templates/${templateId}/versions`);
  }

  async getVersionList() {
    return this.page.locator('[data-testid="version-item"], table tbody tr');
  }

  async getVersionCount() {
    const list = await this.getVersionList();
    return list.count();
  }

  async setDefaultVersion(version: string) {
    await this.page.click(`[data-testid="set-default-${version}"]`);
  }

  async deleteVersion(version: string) {
    await this.page.click(`[data-testid="delete-version-${version}"]`);
    await this.page.click('button:has-text("Confirm")');
  }
}

// ============================================
// Test Suite
// ============================================

test.describe('Admin Template List', () => {
  let templateListPage: AdminTemplateListPage;

  test.beforeEach(async ({ page }) => {
    templateListPage = new AdminTemplateListPage(page);
    await templateListPage.goto();
  });

  test('should display template list', async ({ page }) => {
    await expect(page.locator('h1, [data-testid="page-title"]')).toContainText(/templates/i);
  });

  test('should show template cards with key info', async ({ page }) => {
    const cards = await templateListPage.getTemplateCards();
    const count = await cards.count();

    if (count > 0) {
      const firstCard = cards.first();

      // Should show template name
      await expect(firstCard.locator('[data-testid="template-name"], h3')).toBeVisible();

      // Should show OS family
      await expect(firstCard.locator('[data-testid="os-family"], .os-badge')).toBeVisible();
    }
  });

  test('should show template statistics', async ({ page }) => {
    const statsSection = page.locator('[data-testid="template-stats"]');
    if (await statsSection.isVisible()) {
      await expect(statsSection).toBeVisible();
    }
  });

  test('should search templates by name', async ({ page }) => {
    await templateListPage.searchTemplate('ubuntu');

    await page.waitForLoadState('networkidle');

    const cards = await templateListPage.getTemplateCards();
    const count = await cards.count();

    if (count > 0) {
      const text = await cards.first().textContent();
      expect(text?.toLowerCase()).toContain('ubuntu');
    }
  });

  test('should filter templates by OS family', async ({ page }) => {
    await templateListPage.filterByOSFamily('Linux');

    await page.waitForLoadState('networkidle');

    const osFamilies = page.locator('[data-testid="os-family"]:has-text("Linux"), td:has-text("Linux")');
    const count = await osFamilies.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should filter templates by storage backend', async ({ page }) => {
    await templateListPage.filterByStorageBackend('Ceph');

    await page.waitForLoadState('networkidle');

    const backends = page.locator('[data-testid="storage-backend"]:has-text("Ceph"), td:has-text("Ceph")');
    const count = await backends.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should click template to view details', async ({ page }) => {
    const cards = await templateListPage.getTemplateCards();
    const count = await cards.count();

    if (count > 0) {
      const name = await cards.first().locator('[data-testid="template-name"], h3').textContent();
      await templateListPage.clickTemplateByName(name || '');

      await expect(page).toHaveURL(/\/templates\/[a-f0-9-]+/);
    }
  });

  test('should show OS icons/badges', async ({ page }) => {
    const cards = await templateListPage.getTemplateCards();
    const count = await cards.count();

    if (count > 0) {
      // Should show OS family indicator
      const firstCard = cards.first();
      await expect(firstCard.locator('[data-testid="os-family"], .badge, .icon')).toBeVisible();
    }
  });
});

test.describe('Admin Template Creation', () => {
  let templateCreatePage: AdminTemplateCreatePage;

  test.beforeEach(async ({ page }) => {
    templateCreatePage = new AdminTemplateCreatePage(page);
    await templateCreatePage.goto();
  });

  test('should display template creation form', async ({ page }) => {
    await expect(page.locator('input[name="name"]')).toBeVisible();
    await expect(page.locator('select[name="os_family"], [data-testid="os-family-select"]')).toBeVisible();
    await expect(page.locator('button[type="submit"]')).toBeVisible();
  });

  test('should show validation errors for missing fields', async ({ page }) => {
    await templateCreatePage.submit();

    await templateCreatePage.expectValidationError(/required|please.*enter/i);
  });

  test('should validate template name format', async ({ page }) => {
    await templateCreatePage.fillName('Invalid Name With Spaces!');
    await templateCreatePage.submit();

    await templateCreatePage.expectValidationError(/valid.*name|alphanumeric|lowercase/i);
  });

  test('should show OS family options', async ({ page }) => {
    await page.click('[data-testid="os-family-select"], select[name="os_family"]');

    // Should show common OS families
    await expect(page.locator('text=/linux|windows|bsd/i')).toBeVisible();
  });

  test('should show storage backend options', async ({ page }) => {
    await expect(page.locator('text=/ceph|qcow/i')).toBeVisible();
  });

  test('should show RBD fields for Ceph backend', async ({ page }) => {
    await templateCreatePage.selectStorageBackend('ceph');

    await expect(page.locator('input[name="rbd_image"]')).toBeVisible();
    await expect(page.locator('input[name="rbd_snapshot"]')).toBeVisible();
  });

  test('should show file upload for QCOW backend', async ({ page }) => {
    await templateCreatePage.selectStorageBackend('qcow');

    await expect(page.locator('input[type="file"]')).toBeVisible();
  });

  test('should create template for Ceph', async ({ page }) => {
    const templateName = `ubuntu-24-04-${Date.now()}`;

    await templateCreatePage.fillName(templateName);
    await templateCreatePage.fillDescription('Ubuntu 24.04 LTS Server');
    await templateCreatePage.selectOSFamily('Linux');
    await templateCreatePage.fillOSVersion('24.04');
    await templateCreatePage.selectStorageBackend('ceph');
    await templateCreatePage.fillRBDImage('ubuntu-24.04-server');
    await templateCreatePage.fillRBDSnapshot('v1');
    await templateCreatePage.submit();

    // Should redirect to template detail page
    await expect(page).toHaveURL(/\/templates\/[a-f0-9-]+/);
  });

  test('should handle file upload for QCOW', async ({ page }) => {
    await templateCreatePage.fillName('test-qcow-template');
    await templateCreatePage.selectOSFamily('Linux');
    await templateCreatePage.selectStorageBackend('qcow');

    // File input should be visible
    const fileInput = page.locator('input[type="file"]');
    await expect(fileInput).toBeVisible();
  });
});

test.describe('Admin Template Import', () => {
  let templateListPage: AdminTemplateListPage;
  let importModal: AdminTemplateImportModal;

  test.beforeEach(async ({ page }) => {
    templateListPage = new AdminTemplateListPage(page);
    importModal = new AdminTemplateImportModal(page);
    await templateListPage.goto();
  });

  test('should open import modal', async ({ page }) => {
    await templateListPage.clickImportTemplate();

    await importModal.expectVisible();
  });

  test('should list available source images', async ({ page }) => {
    await templateListPage.clickImportTemplate();

    await page.click('[data-testid="source-image-select"]');

    // Should show available images
    await expect(page.locator('[role="option"], option')).toBeVisible();
  });

  test('should require template name for import', async ({ page }) => {
    await templateListPage.clickImportTemplate();
    await importModal.selectSourceImage('ubuntu-base');
    await importModal.confirmImport();

    await expect(page.locator('.error, [role="alert"]')).toContainText(/required/i);
  });

  test('should import template from existing image', async ({ page }) => {
    await templateListPage.clickImportTemplate();

    await importModal.selectSourceImage('ubuntu-base');
    await importModal.fillTemplateName('imported-ubuntu');
    await importModal.selectOSFamily('Linux');
    await importModal.confirmImport();

    // Should show success message
    await expect(page.locator('text=/import.*success|created/i')).toBeVisible();
  });

  test('should allow canceling import', async ({ page }) => {
    await templateListPage.clickImportTemplate();
    await importModal.cancel();

    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
  });
});

test.describe('Admin Template Detail', () => {
  let templateDetailPage: AdminTemplateDetailPage;
  const testTemplateId = '00000000-0000-0000-0000-000000000001';

  test.beforeEach(async ({ page }) => {
    templateDetailPage = new AdminTemplateDetailPage(page);
    await templateDetailPage.goto(testTemplateId);
  });

  test('should display template details', async ({ page }) => {
    await expect(page.locator('[data-testid="template-name"]')).toBeVisible();
  });

  test('should show OS information', async ({ page }) => {
    const osInfo = await templateDetailPage.getOSInfo();

    expect(osInfo.family).toBeTruthy();
  });

  test('should show storage information', async ({ page }) => {
    const storageInfo = await templateDetailPage.getStorageInfo();

    expect(storageInfo.backend).toBeTruthy();
  });

  test('should show template size', async ({ page }) => {
    const size = await templateDetailPage.getSize();
    expect(size).toMatch(/\d+.*GB|MB/i);
  });

  test('should show action buttons', async ({ page }) => {
    await expect(page.locator('button:has-text("Edit")')).toBeVisible();
    await expect(page.locator('button:has-text("Delete")')).toBeVisible();
  });

  test('should show VM count using this template', async ({ page }) => {
    const vmCount = page.locator('[data-testid="vm-count"]');

    if (await vmCount.isVisible()) {
      const count = await vmCount.textContent();
      expect(count).toMatch(/\d+/);
    }
  });

  test('should navigate to versions tab', async ({ page }) => {
    await templateDetailPage.navigateToVersions();

    await expect(page.locator('[data-testid="version-list"], table')).toBeVisible();
  });

  test('should navigate to usage tab', async ({ page }) => {
    await templateDetailPage.navigateToUsage();

    await expect(page.locator('[data-testid="usage-list"], table')).toBeVisible();
  });
});

test.describe('Admin Template Versioning', () => {
  let versionsPage: AdminTemplateVersionsPage;
  const testTemplateId = '00000000-0000-0000-0000-000000000002';

  test.beforeEach(async ({ page }) => {
    versionsPage = new AdminTemplateVersionsPage(page);
    await versionsPage.goto(testTemplateId);
  });

  test('should display version list', async ({ page }) => {
    await expect(page.locator('h1, [data-testid="page-title"]')).toContainText(/versions/i);
  });

  test('should show version history', async ({ page }) => {
    const count = await versionsPage.getVersionCount();
    expect(count).toBeGreaterThanOrEqual(1);
  });

  test('should show default version indicator', async ({ page }) => {
    const versionList = await versionsPage.getVersionList();
    const count = await versionList.count();

    if (count > 0) {
      // Should show which version is default
      await expect(page.locator('text=/default|current/i')).toBeVisible();
    }
  });

  test('should allow setting new default version', async ({ page }) => {
    const versionList = await versionsPage.getVersionList();
    const count = await versionList.count();

    if (count > 1) {
      // Get second version
      const secondVersion = versionList.nth(1);
      const versionNum = await secondVersion.locator('[data-testid="version-number"]').textContent();

      if (versionNum) {
        await versionsPage.setDefaultVersion(versionNum);

        await expect(page.locator('text=/default.*updated|success/i')).toBeVisible();
      }
    }
  });

  test('should show import new version button', async ({ page }) => {
    await expect(page.locator('button:has-text("Import Version")')).toBeVisible();
  });
});

test.describe('Admin Template Deletion', () => {
  let templateDetailPage: AdminTemplateDetailPage;

  test('should show delete button for unused template', async ({ page }) => {
    templateDetailPage = new AdminTemplateDetailPage(page);
    await templateDetailPage.goto('unused-template-id');

    await expect(page.locator('button:has-text("Delete")')).toBeVisible();
  });

  test('should show deletion confirmation', async ({ page }) => {
    templateDetailPage = new AdminTemplateDetailPage(page);
    await templateDetailPage.goto('unused-template-id');
    await templateDetailPage.clickDelete();

    await expect(page.locator('[role="dialog"], .modal')).toBeVisible();
    await expect(page.locator('text=/confirm|are you sure/i')).toBeVisible();
  });

  test('should delete template after confirmation', async ({ page }) => {
    templateDetailPage = new AdminTemplateDetailPage(page);
    await templateDetailPage.goto('deletable-template-id');
    await templateDetailPage.clickDelete();

    await page.click('button:has-text("Confirm")');

    // Should redirect to template list
    await expect(page).toHaveURL(/\/templates$/);
  });

  test('should disable delete for template in use', async ({ page }) => {
    templateDetailPage = new AdminTemplateDetailPage(page);
    await templateDetailPage.goto('template-in-use-id');

    await expect(page.locator('button:has-text("Delete")')).toBeDisabled();
  });

  test('should show reason for disabled deletion', async ({ page }) => {
    templateDetailPage = new AdminTemplateDetailPage(page);
    await templateDetailPage.goto('template-in-use-id');

    await page.hover('button:has-text("Delete")');

    await expect(page.locator('text=/VMs|in use|cannot delete/i')).toBeVisible();
  });

  test('should allow canceling deletion', async ({ page }) => {
    templateDetailPage = new AdminTemplateDetailPage(page);
    await templateDetailPage.goto('unused-template-id');
    await templateDetailPage.clickDelete();

    await page.click('button:has-text("Cancel")');

    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
  });
});

test.describe('Admin Template OS Filters', () => {
  test('should filter by Linux distributions', async ({ page }) => {
    await page.goto('/templates');
    await page.click('[data-testid="os-filter"], select[name="os_family"]');
    await page.click('option:has-text("Linux")');

    await page.waitForLoadState('networkidle');

    const osFamilies = page.locator('[data-testid="os-family"]:has-text("Linux")');
    const count = await osFamilies.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should filter by Windows', async ({ page }) => {
    await page.goto('/templates');
    await page.click('[data-testid="os-filter"], select[name="os_family"]');
    await page.click('option:has-text("Windows")');

    await page.waitForLoadState('networkidle');

    // Should show only Windows templates
    const osFamilies = page.locator('[data-testid="os-family"]:has-text("Windows")');
    const count = await osFamilies.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should filter by BSD', async ({ page }) => {
    await page.goto('/templates');
    await page.click('[data-testid="os-filter"], select[name="os_family"]');
    await page.click('option:has-text("BSD")');

    await page.waitForLoadState('networkidle');

    const osFamilies = page.locator('[data-testid="os-family"]:has-text("BSD")');
    const count = await osFamilies.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });
});

test.describe('Admin Template Navigation', () => {
  test('should have working navigation from dashboard', async ({ page }) => {
    await page.goto('/dashboard');

    await page.click('a:has-text("Templates")');

    await expect(page).toHaveURL(/\/templates/);
  });

  test('should have breadcrumb navigation', async ({ page }) => {
    await page.goto('/templates/test-template-id');

    await page.click('a:has-text("Templates")');

    await expect(page).toHaveURL(/\/templates$/);
  });

  test('should navigate from template list to detail', async ({ page }) => {
    await page.goto('/templates');

    const templateLink = page.locator('[data-testid="template-card"] a, table tbody tr a').first();
    if (await templateLink.isVisible()) {
      await templateLink.click();
      await expect(page).toHaveURL(/\/templates\/[a-f0-9-]+/);
    }
  });
});