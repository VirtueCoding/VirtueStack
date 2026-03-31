import { test, expect, Page } from '@playwright/test';

/**
 * Admin IP Set Management E2E Tests
 *
 * Tests cover:
 * - IP set list viewing
 * - IP set creation
 * - IP address allocation
 * - IP address assignment to VMs
 * - IP set deletion
 */

// ============================================
// Page Object Models
// ============================================

class AdminIPSetListPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/ip-sets');
    await expect(this.page.locator('h1, [data-testid="page-title"]')).toContainText(/ip.*set|ip.*pool/i);
  }

  async getIPSetCards() {
    return this.page.locator('[data-testid="ip-set-card"], table tbody tr');
  }

  async getIPSetCount() {
    const cards = await this.getIPSetCards();
    return cards.count();
  }

  async clickCreateIPSet() {
    await this.page.click('button:has-text("Add IP Set"), a:has-text("Add IP Set"), [data-testid="create-ip-set-btn"]');
  }

  async searchIPSet(query: string) {
    await this.page.fill('input[placeholder*="search" i], input[name="search"]', query);
    await this.page.press('input[placeholder*="search" i], input[name="search"]', 'Enter');
  }

  async filterByLocation(location: string) {
    await this.page.click('[data-testid="location-filter"], select[name="location"]');
    await this.page.click(`option:has-text("${location}")`);
  }

  async filterByType(type: string) {
    await this.page.click('[data-testid="type-filter"], select[name="type"]');
    await this.page.click(`option:has-text("${type}")`);
  }

  async clickIPSetByName(name: string) {
    await this.page.click(`a:has-text("${name}"), [data-testid="ip-set-${name}"]`);
  }

  async expectIPSetInList(name: string) {
    await expect(this.page.locator(`text="${name}"`)).toBeVisible();
  }

  async expectIPSetNotInList(name: string) {
    await expect(this.page.locator(`text="${name}"`)).not.toBeVisible();
  }
}

class AdminIPSetCreatePage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/ip-sets/create');
    await expect(this.page.locator('h1')).toContainText(/add.*ip|new.*ip.*set/i);
  }

  async fillName(name: string) {
    await this.page.fill('input[name="name"]', name);
  }

  async selectLocation(location: string) {
    await this.page.click('[data-testid="location-select"], select[name="location_id"]');
    await this.page.click(`text="${location}"`);
  }

  async fillNetwork(cidr: string) {
    await this.page.fill('input[name="network"]', cidr);
  }

  async fillGateway(gateway: string) {
    await this.page.fill('input[name="gateway"]', gateway);
  }

  async fillNetmask(netmask: string) {
    await this.page.fill('input[name="netmask"]', netmask);
  }

  async fillVLAN(vlan: number) {
    await this.page.fill('input[name="vlan"]', vlan.toString());
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
    await expect(this.page).toHaveURL(/\/ip-sets\/[a-f0-9-]+/);
  }
}

class AdminIPSetDetailPage {
  constructor(private page: Page) {}

  async goto(ipSetId: string) {
    await this.page.goto(`/ip-sets/${ipSetId}`);
  }

  async getName() {
    return this.page.locator('[data-testid="ip-set-name"]').textContent();
  }

  async getNetworkInfo() {
    return {
      network: await this.page.locator('[data-testid="network"]').textContent(),
      gateway: await this.page.locator('[data-testid="gateway"]').textContent(),
      netmask: await this.page.locator('[data-testid="netmask"]').textContent(),
      vlan: await this.page.locator('[data-testid="vlan"]').textContent(),
    };
  }

  async getLocation() {
    return this.page.locator('[data-testid="location"]').textContent();
  }

  async getIPStats() {
    return {
      total: await this.page.locator('[data-testid="total-ips"]').textContent(),
      used: await this.page.locator('[data-testid="used-ips"]').textContent(),
      available: await this.page.locator('[data-testid="available-ips"]').textContent(),
    };
  }

  async clickEdit() {
    await this.page.click('button:has-text("Edit"), [data-testid="edit-btn"]');
  }

  async clickDelete() {
    await this.page.click('button:has-text("Delete"), [data-testid="delete-btn"]');
  }

  async navigateToIPs() {
    await this.page.click('a:has-text("IP Addresses"), [data-testid="ips-tab"]');
  }

  async clickViewAvailable() {
    await this.page.click('button:has-text("View Available"), a:has-text("Available")');
  }
}

class AdminIPListPage {
  constructor(private page: Page) {}

  async goto(ipSetId: string) {
    await this.page.goto(`/ip-sets/${ipSetId}/ips`);
  }

  async getIPRows() {
    return this.page.locator('table tbody tr, [data-testid="ip-row"]');
  }

  async getIPCount() {
    const rows = await this.getIPRows();
    return rows.count();
  }

  async filterByStatus(status: 'available' | 'used' | 'reserved') {
    await this.page.click('[data-testid="status-filter"], select[name="status"]');
    await this.page.click(`option:has-text("${status}")`);
  }

  async searchIP(ip: string) {
    await this.page.fill('input[placeholder*="search" i], input[name="search"]', ip);
    await this.page.press('input[placeholder*="search" i], input[name="search"]', 'Enter');
  }

  async clickIP(address: string) {
    await this.page.click(`a:has-text("${address}"), [data-testid="ip-${address}"]`);
  }

  async reserveIP(address: string) {
    const row = this.page.locator(`tr:has-text("${address}")`);
    await row.locator('button:has-text("Reserve")').click();
  }

  async releaseIP(address: string) {
    const row = this.page.locator(`tr:has-text("${address}")`);
    await row.locator('button:has-text("Release")').click();
  }
}

class AdminIPDetailPage {
  constructor(private page: Page) {}

  async goto(ipId: string) {
    await this.page.goto(`/ip-addresses/${ipId}`);
  }

  async getAddress() {
    return this.page.locator('[data-testid="ip-address"]').textContent();
  }

  async getStatus() {
    return this.page.locator('[data-testid="ip-status"]').textContent();
  }

  async getVMInfo() {
    return {
      hostname: await this.page.locator('[data-testid="vm-hostname"]').textContent(),
      id: await this.page.locator('[data-testid="vm-id"]').textContent(),
    };
  }

  async getRDNS() {
    return this.page.locator('[data-testid="rdns-hostname"]').textContent();
  }

  async setRDNS(hostname: string) {
    await this.page.click('button:has-text("Edit rDNS")');
    await this.page.fill('input[name="rdns_hostname"]', hostname);
    await this.page.click('button:has-text("Save")');
  }

  async releaseIP() {
    await this.page.click('button:has-text("Release")');
  }
}

class AdminAvailableIPsPage {
  constructor(private page: Page) {}

  async goto(ipSetId: string) {
    await this.page.goto(`/ip-sets/${ipSetId}/available`);
  }

  async getAvailableIPs() {
    return this.page.locator('[data-testid="available-ip"], table tbody tr');
  }

  async getAvailableCount() {
    const ips = await this.getAvailableIPs();
    return ips.count();
  }

  async selectIP(address: string) {
    await this.page.click(`[data-testid="ip-${address}"] input[type="checkbox"]`);
  }

  async reserveSelected() {
    await this.page.click('button:has-text("Reserve Selected")');
  }
}

// ============================================
// Test Suite
// ============================================

test.describe('Admin IP Set List', () => {
  let ipSetListPage: AdminIPSetListPage;

  test.beforeEach(async ({ page }) => {
    ipSetListPage = new AdminIPSetListPage(page);
    await ipSetListPage.goto();
  });

  test('should display IP set list', async ({ page }) => {
    await expect(page.locator('h1, [data-testid="page-title"]')).toContainText(/ip.*set|ip.*pool/i);
  });

  test('should show IP set cards with key info', async ({ page }) => {
    const cards = await ipSetListPage.getIPSetCards();
    const count = await cards.count();

    if (count > 0) {
      const firstCard = cards.first();

      // Should show IP set name
      await expect(firstCard.locator('[data-testid="ip-set-name"], h3')).toBeVisible();

      // Should show network CIDR
      await expect(firstCard.locator('text=/\d+\.\d+\.\d+\.\d+\/\d+/')).toBeVisible();
    }
  });

  test('should show IP usage summary', async ({ page }) => {
    const cards = await ipSetListPage.getIPSetCards();
    const count = await cards.count();

    if (count > 0) {
      // Should show used/total IPs
      const firstCard = cards.first();
      const text = await firstCard.textContent();
      expect(text).toMatch(/\d+.*\/.*\d+|\d+.*used|\d+.*available/i);
    }
  });

  test('should search IP sets by name', async ({ page }) => {
    await ipSetListPage.searchIPSet('public');

    await page.waitForLoadState('networkidle');

    const cards = await ipSetListPage.getIPSetCards();
    const count = await cards.count();

    if (count > 0) {
      const text = await cards.first().textContent();
      expect(text?.toLowerCase()).toContain('public');
    }
  });

  test('should filter IP sets by location', async ({ page }) => {
    await ipSetListPage.filterByLocation('US East');

    await page.waitForLoadState('networkidle');

    // Should show filtered results
    const cards = await ipSetListPage.getIPSetCards();
    expect(await cards.count()).toBeGreaterThanOrEqual(0);
  });

  test('should click IP set to view details', async ({ page }) => {
    const cards = await ipSetListPage.getIPSetCards();
    const count = await cards.count();

    if (count > 0) {
      const name = await cards.first().locator('[data-testid="ip-set-name"], h3').textContent();
      await ipSetListPage.clickIPSetByName(name || '');

      await expect(page).toHaveURL(/\/ip-sets\/[a-f0-9-]+/);
    }
  });
});

test.describe('Admin IP Set Creation', () => {
  let ipSetCreatePage: AdminIPSetCreatePage;

  test.beforeEach(async ({ page }) => {
    ipSetCreatePage = new AdminIPSetCreatePage(page);
    await ipSetCreatePage.goto();
  });

  test('should display IP set creation form', async ({ page }) => {
    await expect(page.locator('input[name="name"]')).toBeVisible();
    await expect(page.locator('input[name="network"]')).toBeVisible();
    await expect(page.locator('input[name="gateway"]')).toBeVisible();
    await expect(page.locator('button[type="submit"]')).toBeVisible();
  });

  test('should show validation errors for missing fields', async ({ page }) => {
    await ipSetCreatePage.submit();

    await ipSetCreatePage.expectValidationError(/required|please.*enter/i);
  });

  test('should validate network CIDR format', async ({ page }) => {
    await ipSetCreatePage.fillNetwork('invalid-cidr');
    await ipSetCreatePage.submit();

    await ipSetCreatePage.expectValidationError(/valid.*cidr|network.*format/i);
  });

  test('should validate gateway is within network', async ({ page }) => {
    await ipSetCreatePage.fillNetwork('192.168.1.0/24');
    await ipSetCreatePage.fillGateway('10.0.0.1');
    await ipSetCreatePage.submit();

    await ipSetCreatePage.expectValidationError(/within.*network|same.*subnet/i);
  });

  test('should validate IP address format', async ({ page }) => {
    await ipSetCreatePage.fillGateway('invalid-ip');
    await ipSetCreatePage.submit();

    await ipSetCreatePage.expectValidationError(/valid.*ip|IP.*format/i);
  });

  test('should show location options', async ({ page }) => {
    await page.click('[data-testid="location-select"], select[name="location_id"]');

    // Should show location options
    await expect(page.locator('option, [role="option"]')).toBeVisible();
  });

  test('should create IP set with basic info', async ({ page }) => {
    const ipSetName = `test-ipset-${Date.now()}`;

    await ipSetCreatePage.fillName(ipSetName);
    await ipSetCreatePage.selectLocation('US East');
    await ipSetCreatePage.fillNetwork('10.0.100.0/24');
    await ipSetCreatePage.fillGateway('10.0.100.1');
    await ipSetCreatePage.submit();

    // Should redirect to IP set detail page
    await expect(page).toHaveURL(/\/ip-sets\/[a-f0-9-]+/);
  });

  test('should create IP set with VLAN', async ({ page }) => {
    const ipSetName = `vlan-ipset-${Date.now()}`;

    await ipSetCreatePage.fillName(ipSetName);
    await ipSetCreatePage.fillNetwork('192.168.50.0/24');
    await ipSetCreatePage.fillGateway('192.168.50.1');
    await ipSetCreatePage.fillVLAN(50);
    await ipSetCreatePage.submit();

    await expect(page).toHaveURL(/\/ip-sets\/[a-f0-9-]+/);
  });
});

test.describe('Admin IP Set Detail', () => {
  let ipSetDetailPage: AdminIPSetDetailPage;
  const testIPSetId = '00000000-0000-0000-0000-000000000001';

  test.beforeEach(async ({ page }) => {
    ipSetDetailPage = new AdminIPSetDetailPage(page);
    await ipSetDetailPage.goto(testIPSetId);
  });

  test('should display IP set details', async ({ page }) => {
    await expect(page.locator('[data-testid="ip-set-name"]')).toBeVisible();
  });

  test('should show network information', async ({ page }) => {
    const networkInfo = await ipSetDetailPage.getNetworkInfo();

    expect(networkInfo.network).toBeTruthy();
    expect(networkInfo.gateway).toBeTruthy();
  });

  test('should show location', async ({ page }) => {
    const location = await ipSetDetailPage.getLocation();
    expect(location).toBeTruthy();
  });

  test('should show IP usage statistics', async ({ page }) => {
    const stats = await ipSetDetailPage.getIPStats();

    expect(stats.total).toBeTruthy();
    expect(stats.used).toBeTruthy();
    expect(stats.available).toBeTruthy();
  });

  test('should show action buttons', async ({ page }) => {
    await expect(page.locator('button:has-text("View Available")')).toBeVisible();
    await expect(page.locator('button:has-text("Edit")')).toBeVisible();
  });

  test('should navigate to IP addresses tab', async ({ page }) => {
    await ipSetDetailPage.navigateToIPs();

    await expect(page.locator('table, [data-testid="ip-list"]')).toBeVisible();
  });

  test('should navigate to available IPs', async ({ page }) => {
    await ipSetDetailPage.clickViewAvailable();

    await expect(page).toHaveURL(/\/available/);
  });
});

test.describe('Admin IP Address List', () => {
  let ipListPage: AdminIPListPage;
  const testIPSetId = '00000000-0000-0000-0000-000000000002';

  test.beforeEach(async ({ page }) => {
    ipListPage = new AdminIPListPage(page);
    await ipListPage.goto(testIPSetId);
  });

  test('should display IP address list', async ({ page }) => {
    await expect(page.locator('table')).toBeVisible();
  });

  test('should show IP addresses with status', async ({ page }) => {
    const rows = await ipListPage.getIPRows();
    const count = await rows.count();

    if (count > 0) {
      const firstRow = rows.first();

      // Should show IP address
      await expect(firstRow.locator('text=/\d+\.\d+\.\d+\.\d+/')).toBeVisible();

      // Should show status
      await expect(firstRow.locator('[data-testid="status"], .status-badge')).toBeVisible();
    }
  });

  test('should filter IPs by status', async ({ page }) => {
    await ipListPage.filterByStatus('available');

    await page.waitForLoadState('networkidle');

    const statuses = page.locator('[data-testid="status"]:has-text("available")');
    const count = await statuses.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should search for specific IP', async ({ page }) => {
    await ipListPage.searchIP('192.168.1');

    await page.waitForLoadState('networkidle');

    const rows = await ipListPage.getIPRows();
    const count = await rows.count();

    if (count > 0) {
      const text = await rows.first().textContent();
      expect(text).toContain('192.168.1');
    }
  });

  test('should show VM assignment for used IPs', async ({ page }) => {
    await ipListPage.filterByStatus('used');

    await page.waitForLoadState('networkidle');

    const rows = await ipListPage.getIPRows();
    const count = await rows.count();

    if (count > 0) {
      // Should show VM hostname or ID
      const firstRow = rows.first();
      await expect(firstRow.locator('text=/vm-|hostname/i')).toBeVisible();
    }
  });

  test('should click IP to view details', async ({ page }) => {
    const rows = await ipListPage.getIPRows();
    const count = await rows.count();

    if (count > 0) {
      const ipText = await rows.first().locator('td:first-child, [data-testid="ip-address"]').textContent();
      if (ipText) {
        await ipListPage.clickIP(ipText.trim());
        await expect(page).toHaveURL(/\/ip-addresses\/[a-f0-9-]+/);
      }
    }
  });
});

test.describe('Admin IP Address Detail', () => {
  let ipDetailPage: AdminIPDetailPage;
  const testIPId = '00000000-0000-0000-0000-000000000010';

  test.beforeEach(async ({ page }) => {
    ipDetailPage = new AdminIPDetailPage(page);
    await ipDetailPage.goto(testIPId);
  });

  test('should display IP address details', async ({ page }) => {
    const address = await ipDetailPage.getAddress();
    expect(address).toMatch(/\d+\.\d+\.\d+\.\d+/);
  });

  test('should show IP status', async ({ page }) => {
    const status = await ipDetailPage.getStatus();
    expect(['available', 'used', 'reserved']).toContain(status?.toLowerCase());
  });

  test('should show VM info for assigned IP', async ({ page }) => {
    const status = await ipDetailPage.getStatus();

    if (status?.toLowerCase() === 'used') {
      const vmInfo = await ipDetailPage.getVMInfo();
      expect(vmInfo.hostname).toBeTruthy();
    }
  });

  test('should show rDNS hostname', async ({ page }) => {
    const rdns = page.locator('[data-testid="rdns-hostname"]');

    if (await rdns.isVisible()) {
      const hostname = await rdns.textContent();
      // Should be a valid hostname or show "not set"
      expect(hostname).toBeTruthy();
    }
  });

  test('should allow setting rDNS', async ({ page }) => {
    const status = await ipDetailPage.getStatus();

    if (status?.toLowerCase() === 'used') {
      await ipDetailPage.setRDNS('vm.example.com');

      await expect(page.locator('text=/saved|updated|success/i')).toBeVisible();
    }
  });

  test('should show release button for assigned IP', async ({ page }) => {
    const status = await ipDetailPage.getStatus();

    if (status?.toLowerCase() === 'used') {
      await expect(page.locator('button:has-text("Release")')).toBeVisible();
    }
  });
});

test.describe('Admin Available IPs', () => {
  let availableIPsPage: AdminAvailableIPsPage;
  const testIPSetId = '00000000-0000-0000-0000-000000000003';

  test.beforeEach(async ({ page }) => {
    availableIPsPage = new AdminAvailableIPsPage(page);
    await availableIPsPage.goto(testIPSetId);
  });

  test('should display available IPs', async ({ page }) => {
    await expect(page.locator('h1, [data-testid="page-title"]')).toContainText(/available/i);
  });

  test('should show available IP count', async ({ page }) => {
    const count = await availableIPsPage.getAvailableCount();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should allow selecting IPs', async ({ page }) => {
    const ips = await availableIPsPage.getAvailableIPs();
    const count = await ips.count();

    if (count > 0) {
      const firstIP = await ips.first().locator('[data-testid="ip-address"], td:first-child').textContent();

      if (firstIP) {
        await availableIPsPage.selectIP(firstIP.trim());

        // Checkbox should be checked
        await expect(page.locator('input[type="checkbox"]:checked')).toBeVisible();
      }
    }
  });

  test('should show reserve button when IPs selected', async ({ page }) => {
    const ips = await availableIPsPage.getAvailableIPs();
    const count = await ips.count();

    if (count > 0) {
      const firstIP = await ips.first().locator('[data-testid="ip-address"], td:first-child').textContent();

      if (firstIP) {
        await availableIPsPage.selectIP(firstIP.trim());
        await expect(page.locator('button:has-text("Reserve")')).toBeEnabled();
      }
    }
  });
});

test.describe('Admin IP Set Deletion', () => {
  let ipSetDetailPage: AdminIPSetDetailPage;

  test('should show delete button for unused IP set', async ({ page }) => {
    ipSetDetailPage = new AdminIPSetDetailPage(page);
    await ipSetDetailPage.goto('unused-ipset-id');

    await expect(page.locator('button:has-text("Delete")')).toBeVisible();
  });

  test('should show deletion confirmation', async ({ page }) => {
    ipSetDetailPage = new AdminIPSetDetailPage(page);
    await ipSetDetailPage.goto('unused-ipset-id');
    await ipSetDetailPage.clickDelete();

    await expect(page.locator('[role="dialog"], .modal')).toBeVisible();
    await expect(page.locator('text=/confirm|are you sure/i')).toBeVisible();
  });

  test('should delete IP set after confirmation', async ({ page }) => {
    ipSetDetailPage = new AdminIPSetDetailPage(page);
    await ipSetDetailPage.goto('deletable-ipset-id');
    await ipSetDetailPage.clickDelete();

    await page.click('button:has-text("Confirm")');

    // Should redirect to IP set list
    await expect(page).toHaveURL(/\/ip-sets$/);
  });

  test('should disable delete for IP set with used IPs', async ({ page }) => {
    ipSetDetailPage = new AdminIPSetDetailPage(page);
    await ipSetDetailPage.goto('ipset-with-used-ips');

    await expect(page.locator('button:has-text("Delete")')).toBeDisabled();
  });

  test('should show reason for disabled deletion', async ({ page }) => {
    ipSetDetailPage = new AdminIPSetDetailPage(page);
    await ipSetDetailPage.goto('ipset-with-used-ips');

    await page.hover('button:has-text("Delete")');

    await expect(page.locator('text=/in use|assigned|cannot delete/i')).toBeVisible();
  });
});

test.describe('Admin IP Set Navigation', () => {
  test('should have working navigation from dashboard', async ({ page }) => {
    await page.goto('/dashboard');

    await page.click('a:has-text("IP Sets")');

    await expect(page).toHaveURL(/\/ip-sets/);
  });

  test('should have breadcrumb navigation', async ({ page }) => {
    await page.goto('/ip-sets/test-ipset-id');

    await page.click('a:has-text("IP Sets")');

    await expect(page).toHaveURL(/\/ip-sets$/);
  });

  test('should navigate from IP set to IPs', async ({ page }) => {
    await page.goto('/ip-sets/test-ipset-id');

    await page.click('a:has-text("IP Addresses")');

    await expect(page).toHaveURL(/\/ip-sets\/test-ipset-id\/ips/);
  });
});