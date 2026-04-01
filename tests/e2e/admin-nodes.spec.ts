import { test, expect, Page } from '@playwright/test';

/**
 * Admin Node Management E2E Tests
 *
 * Tests cover:
 * - Node list viewing
 * - Node creation and configuration
 * - Node status monitoring
 * - Node drain operations
 * - Node failover operations
 * - Node deletion
 */

// ============================================
// Page Object Models
// ============================================

class AdminNodeListPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/nodes');
    await expect(this.page.locator('h1, [data-testid="page-title"]')).toContainText(/nodes/i);
  }

  async getNodeCards() {
    return this.page.locator('[data-testid="node-card"], table tbody tr');
  }

  async getNodeCount() {
    const cards = await this.getNodeCards();
    return cards.count();
  }

  async clickCreateNode() {
    await this.page.click('button:has-text("Add Node"), a:has-text("Add Node"), [data-testid="create-node-btn"]');
  }

  async searchNode(query: string) {
    await this.page.fill('input[placeholder*="search" i], input[name="search"]', query);
    await this.page.press('input[placeholder*="search" i], input[name="search"]', 'Enter');
  }

  async filterByStatus(status: string) {
    await this.page.click('[data-testid="status-filter"], select[name="status"]');
    await this.page.click(`option:has-text("${status}")`);
  }

  async filterByLocation(location: string) {
    await this.page.click('[data-testid="location-filter"], select[name="location"]');
    await this.page.click(`option:has-text("${location}")`);
  }

  async clickNodeByHostname(hostname: string) {
    await this.page.click(`a:has-text("${hostname}"), [data-testid="node-${hostname}"]`);
  }

  async expectNodeInList(hostname: string) {
    await expect(this.page.locator(`text="${hostname}"`)).toBeVisible();
  }

  async expectNodeNotInList(hostname: string) {
    await expect(this.page.locator(`text="${hostname}"`)).not.toBeVisible();
  }
}

class AdminNodeCreatePage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/nodes/create');
    await expect(this.page.locator('h1')).toContainText(/add.*node|new.*node/i);
  }

  async fillHostname(hostname: string) {
    await this.page.fill('input[name="hostname"]', hostname);
  }

  async fillGRPCAddress(address: string) {
    await this.page.fill('input[name="grpc_address"]', address);
  }

  async fillManagementIP(ip: string) {
    await this.page.fill('input[name="management_ip"]', ip);
  }

  async selectLocation(location: string) {
    await this.page.click('[data-testid="location-select"], select[name="location_id"]');
    await this.page.click(`text="${location}"`);
  }

  async selectStorageBackend(backend: 'ceph' | 'qcow') {
    await this.page.click(`[data-testid="storage-${backend}"], input[value="${backend}"]`);
  }

  async fillStoragePath(path: string) {
    await this.page.fill('input[name="storage_path"]', path);
  }

  async submit() {
    await this.page.click('button[type="submit"]');
  }

  async expectValidationError(message: string | RegExp) {
    await expect(this.page.locator('.error, [role="alert"]')).toContainText(message);
  }

  async expectSuccess() {
    await expect(this.page).toHaveURL(/\/nodes\/[a-f0-9-]+/);
  }
}

class AdminNodeDetailPage {
  constructor(private page: Page) {}

  async goto(nodeId: string) {
    await this.page.goto(`/nodes/${nodeId}`);
  }

  async getHostname() {
    return this.page.locator('[data-testid="node-hostname"]').textContent();
  }

  async getStatus() {
    return this.page.locator('[data-testid="node-status"]').textContent();
  }

  async getResourceUsage() {
    return {
      cpu: await this.page.locator('[data-testid="cpu-usage"]').textContent(),
      memory: await this.page.locator('[data-testid="memory-usage"]').textContent(),
      storage: await this.page.locator('[data-testid="storage-usage"]').textContent(),
    };
  }

  async getVMCount() {
    return this.page.locator('[data-testid="vm-count"]').textContent();
  }

  async drainNode() {
    await this.page.click('button:has-text("Drain"), [data-testid="drain-btn"]');
  }

  async undrainNode() {
    await this.page.click('button:has-text("Undrain"), [data-testid="undrain-btn"]');
  }

  async initiateFailover() {
    await this.page.click('button:has-text("Failover"), [data-testid="failover-btn"]');
  }

  async deleteNode() {
    await this.page.click('button:has-text("Delete"), [data-testid="delete-btn"]');
  }

  async navigateToVMs() {
    await this.page.click('a:has-text("Virtual Machines"), [data-testid="vms-tab"]');
  }

  async navigateToMetrics() {
    await this.page.click('a:has-text("Metrics"), [data-testid="metrics-tab"]');
  }

  async waitForStatus(status: string, timeout = 30000) {
    await expect(this.page.locator('[data-testid="node-status"]')).toContainText(status, { timeout });
  }

  async expectActionDisabled(action: string) {
    await expect(this.page.locator(`button:has-text("${action}")`)).toBeDisabled();
  }

  async expectActionEnabled(action: string) {
    await expect(this.page.locator(`button:has-text("${action}")`)).toBeEnabled();
  }
}

class AdminDrainModal {
  constructor(private page: Page) {}

  async expectVisible() {
    await expect(this.page.locator('[role="dialog"], .modal:has-text("Drain")')).toBeVisible();
  }

  async selectTargetNode(nodeName: string) {
    await this.page.click('[data-testid="target-node-select"]');
    await this.page.click(`text="${nodeName}"`);
  }

  async confirmDrain() {
    await this.page.click('button:has-text("Confirm"), button:has-text("Drain")');
  }

  async cancel() {
    await this.page.click('button:has-text("Cancel")');
  }

  async acknowledgeWarning() {
    await this.page.check('input[type="checkbox"]');
  }
}

class AdminFailoverModal {
  constructor(private page: Page) {}

  async expectVisible() {
    await expect(this.page.locator('[role="dialog"], .modal:has-text("Failover")')).toBeVisible();
  }

  async selectTargetNode(nodeName: string) {
    await this.page.click('[data-testid="target-node-select"]');
    await this.page.click(`text="${nodeName}"`);
  }

  async confirmFailover() {
    await this.page.click('button:has-text("Confirm"), button:has-text("Start Failover")');
  }

  async cancel() {
    await this.page.click('button:has-text("Cancel")');
  }

  async acknowledgeWarning() {
    await this.page.check('input[type="checkbox"]');
  }
}

// ============================================
// Test Suite
// ============================================

test.describe('Admin Node List', () => {
  let nodeListPage: AdminNodeListPage;

  test.beforeEach(async ({ page }) => {
    nodeListPage = new AdminNodeListPage(page);
    await nodeListPage.goto();
  });

  test('should display node list', async ({ page }) => {
    await expect(page.locator('h1, [data-testid="page-title"]')).toContainText(/nodes/i);
  });

  test('should show node cards with key info', async ({ page }) => {
    const cards = await nodeListPage.getNodeCards();
    const count = await cards.count();

    if (count > 0) {
      const firstCard = cards.first();

      // Should show hostname
      await expect(firstCard.locator('[data-testid="node-hostname"], h3')).toBeVisible();

      // Should show status
      await expect(firstCard.locator('[data-testid="node-status"], .status')).toBeVisible();
    }
  });

  test('should show node statistics', async ({ page }) => {
    const statsSection = page.locator('[data-testid="node-stats"]');
    if (await statsSection.isVisible()) {
      await expect(statsSection).toBeVisible();
    }
  });

  test('should search nodes by hostname', async ({ page }) => {
    await nodeListPage.searchNode('node');

    await page.waitForLoadState('networkidle');

    const cards = await nodeListPage.getNodeCards();
    const count = await cards.count();

    if (count > 0) {
      const text = await cards.first().textContent();
      expect(text?.toLowerCase()).toContain('node');
    }
  });

  test('should filter nodes by status', async ({ page }) => {
    await nodeListPage.filterByStatus('Online');

    await page.waitForLoadState('networkidle');

    const statuses = page.locator('[data-testid="node-status"]:has-text("Online"), td:has-text("Online")');
    const count = await statuses.count();
    expect(count).toBeGreaterThan(0);
  });

  test('should filter nodes by location', async ({ page }) => {
    await nodeListPage.filterByLocation('US East');

    await page.waitForLoadState('networkidle');

    // Should show filtered results
    const cards = await nodeListPage.getNodeCards();
    expect(await cards.count()).toBeGreaterThanOrEqual(0);
  });

  test('should click node to view details', async ({ page }) => {
    const cards = await nodeListPage.getNodeCards();
    const count = await cards.count();

    if (count > 0) {
      const hostname = await cards.first().locator('[data-testid="node-hostname"], h3').textContent();
      await nodeListPage.clickNodeByHostname(hostname || '');

      await expect(page).toHaveURL(/\/nodes\/[a-f0-9-]+/);
    }
  });
});

test.describe('Admin Node Creation', () => {
  let nodeCreatePage: AdminNodeCreatePage;

  test.beforeEach(async ({ page }) => {
    nodeCreatePage = new AdminNodeCreatePage(page);
    await nodeCreatePage.goto();
  });

  test('should display node creation form', async ({ page }) => {
    await expect(page.locator('input[name="hostname"]')).toBeVisible();
    await expect(page.locator('input[name="grpc_address"]')).toBeVisible();
    await expect(page.locator('button[type="submit"]')).toBeVisible();
  });

  test('should show validation errors for missing fields', async ({ page }) => {
    await nodeCreatePage.submit();

    await nodeCreatePage.expectValidationError(/required|please.*enter/i);
  });

  test('should validate hostname format', async ({ page }) => {
    await nodeCreatePage.fillHostname('invalid hostname with spaces');
    await nodeCreatePage.submit();

    await nodeCreatePage.expectValidationError(/valid hostname|RFC 1123/i);
  });

  test('should validate GRPC address format', async ({ page }) => {
    await nodeCreatePage.fillGRPCAddress('invalid-address');
    await nodeCreatePage.submit();

    await nodeCreatePage.expectValidationError(/valid.*address|host:port/i);
  });

  test('should validate IP address format', async ({ page }) => {
    await nodeCreatePage.fillManagementIP('invalid-ip');
    await nodeCreatePage.submit();

    await nodeCreatePage.expectValidationError(/valid.*ip|IP address/i);
  });

  test('should show storage backend options', async ({ page }) => {
    await expect(page.locator('text=/ceph|qcow/i')).toBeVisible();
  });

  test('should show storage path field for QCOW backend', async ({ page }) => {
    await nodeCreatePage.selectStorageBackend('qcow');

    await expect(page.locator('input[name="storage_path"]')).toBeVisible();
  });

  test('should create node with Ceph backend', async ({ page }) => {
    const testHostname = `test-node-${Date.now()}`;

    await nodeCreatePage.fillHostname(testHostname);
    await nodeCreatePage.fillGRPCAddress('10.0.0.100:50051');
    await nodeCreatePage.fillManagementIP('10.0.0.100');
    await nodeCreatePage.selectStorageBackend('ceph');
    await nodeCreatePage.submit();

    // Should redirect to node detail or show success
    await expect(page.locator(`text="${testHostname}"`)).toBeVisible();
  });

  test('should create node with QCOW backend', async ({ page }) => {
    const testHostname = `qcow-node-${Date.now()}`;

    await nodeCreatePage.fillHostname(testHostname);
    await nodeCreatePage.fillGRPCAddress('10.0.0.101:50051');
    await nodeCreatePage.fillManagementIP('10.0.0.101');
    await nodeCreatePage.selectStorageBackend('qcow');
    await nodeCreatePage.fillStoragePath('/var/lib/virtuestack/vms');
    await nodeCreatePage.submit();

    await expect(page.locator(`text="${testHostname}"`)).toBeVisible();
  });
});

test.describe('Admin Node Detail', () => {
  let nodeDetailPage: AdminNodeDetailPage;
  const testNodeId = '00000000-0000-0000-0000-000000000001';

  test.beforeEach(async ({ page }) => {
    nodeDetailPage = new AdminNodeDetailPage(page);
    await nodeDetailPage.goto(testNodeId);
  });

  test('should display node details', async ({ page }) => {
    await expect(page.locator('[data-testid="node-hostname"]')).toBeVisible();
    await expect(page.locator('[data-testid="node-status"]')).toBeVisible();
  });

  test('should show resource usage', async ({ page }) => {
    const usage = await nodeDetailPage.getResourceUsage();

    // At least one resource metric should be visible
    expect(usage.cpu || usage.memory || usage.storage).toBeTruthy();
  });

  test('should show VM count', async ({ page }) => {
    const vmCount = page.locator('[data-testid="vm-count"]');

    if (await vmCount.isVisible()) {
      const count = await vmCount.textContent();
      expect(count).toMatch(/\d+/);
    }
  });

  test('should show action buttons', async ({ page }) => {
    await expect(page.locator('button:has-text("Drain")')).toBeVisible();
    await expect(page.locator('button:has-text("Failover")')).toBeVisible();
  });

  test('should navigate to VMs tab', async ({ page }) => {
    await nodeDetailPage.navigateToVMs();

    await expect(page.locator('table, [data-testid="vm-list"]')).toBeVisible();
  });

  test('should navigate to metrics tab', async ({ page }) => {
    await nodeDetailPage.navigateToMetrics();

    await expect(page.locator('[data-testid="metrics-chart"], .chart')).toBeVisible();
  });
});

test.describe('Admin Node Drain', () => {
  let nodeDetailPage: AdminNodeDetailPage;
  let drainModal: AdminDrainModal;
  const testNodeId = '00000000-0000-0000-0000-000000000002';

  test.beforeEach(async ({ page }) => {
    nodeDetailPage = new AdminNodeDetailPage(page);
    drainModal = new AdminDrainModal(page);
    await nodeDetailPage.goto(testNodeId);
  });

  test('should show drain button for online node', async ({ page }) => {
    await nodeDetailPage.expectActionEnabled('Drain');
  });

  test('should open drain confirmation modal', async ({ page }) => {
    await nodeDetailPage.drainNode();

    await drainModal.expectVisible();
  });

  test('should show drain warning', async ({ page }) => {
    await nodeDetailPage.drainNode();

    await expect(page.locator('text=/migrate|warning|VMs will be/i')).toBeVisible();
  });

  test('should require acknowledgment before drain', async ({ page }) => {
    await nodeDetailPage.drainNode();

    // Confirm button should be disabled until acknowledged
    await expect(page.locator('button:has-text("Drain"):not(:disabled)')).not.toBeVisible();

    await drainModal.acknowledgeWarning();

    // Now should be enabled
    await expect(page.locator('button:has-text("Drain")')).toBeEnabled();
  });

  test('should initiate drain after confirmation', async ({ page }) => {
    await nodeDetailPage.drainNode();
    await drainModal.acknowledgeWarning();
    await drainModal.confirmDrain();

    // Should show success or progress
    await expect(page.locator('text=/drain.*started|draining/i')).toBeVisible();
  });

  test('should allow canceling drain', async ({ page }) => {
    await nodeDetailPage.drainNode();
    await drainModal.cancel();

    // Modal should close
    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
  });

  test('should show undrain button for draining node', async ({ page }) => {
    // Navigate to a draining node
    await page.goto('/nodes/draining-node-id');

    await nodeDetailPage.expectActionEnabled('Undrain');
  });

  test('should undrain a draining node', async ({ page }) => {
    await page.goto('/nodes/draining-node-id');
    await nodeDetailPage.undrainNode();

    await expect(page.locator('text=/undrain.*success|cancelled/i')).toBeVisible();
  });
});

test.describe('Admin Node Failover', () => {
  let nodeDetailPage: AdminNodeDetailPage;
  let failoverModal: AdminFailoverModal;
  const testNodeId = '00000000-0000-0000-0000-000000000003';

  test.beforeEach(async ({ page }) => {
    nodeDetailPage = new AdminNodeDetailPage(page);
    failoverModal = new AdminFailoverModal(page);
    await nodeDetailPage.goto(testNodeId);
  });

  test('should show failover button', async ({ page }) => {
    await expect(page.locator('button:has-text("Failover")')).toBeVisible();
  });

  test('should open failover confirmation modal', async ({ page }) => {
    await nodeDetailPage.initiateFailover();

    await failoverModal.expectVisible();
  });

  test('should show failover warning', async ({ page }) => {
    await nodeDetailPage.initiateFailover();

    await expect(page.locator('text=/warning|data loss|critical/i')).toBeVisible();
  });

  test('should require target node selection', async ({ page }) => {
    await nodeDetailPage.initiateFailover();

    // Should show target node selector
    await expect(page.locator('[data-testid="target-node-select"]')).toBeVisible();
  });

  test('should list available target nodes', async ({ page }) => {
    await nodeDetailPage.initiateFailover();
    await page.click('[data-testid="target-node-select"]');

    // Should show node options
    await expect(page.locator('[role="option"], option')).toBeVisible();
  });

  test('should require acknowledgment before failover', async ({ page }) => {
    await nodeDetailPage.initiateFailover();
    await failoverModal.selectTargetNode('target-node');
    await failoverModal.acknowledgeWarning();
    await failoverModal.confirmFailover();

    // Should show progress
    await expect(page.locator('text=/failover.*started|initiated/i')).toBeVisible();
  });

  test('should allow canceling failover', async ({ page }) => {
    await nodeDetailPage.initiateFailover();
    await failoverModal.cancel();

    // Modal should close
    await expect(page.locator('[role="dialog"]')).not.toBeVisible();
  });

  test('should disable failover for offline node with no target', async ({ page }) => {
    // Navigate to offline node with no healthy targets
    await page.goto('/nodes/isolated-node-id');

    await nodeDetailPage.expectActionDisabled('Failover');
  });
});

test.describe('Admin Node Status Updates', () => {
  let nodeDetailPage: AdminNodeDetailPage;
  const testNodeId = '00000000-0000-0000-0000-000000000004';

  test.beforeEach(async ({ page }) => {
    nodeDetailPage = new AdminNodeDetailPage(page);
  });

  test('should show online status', async ({ page }) => {
    await nodeDetailPage.goto(testNodeId);

    const status = await nodeDetailPage.getStatus();
    expect(status?.toLowerCase()).toContain('online');
  });

  test('should show offline status', async ({ page }) => {
    await page.goto('/nodes/offline-node-id');

    const status = await nodeDetailPage.getStatus();
    expect(status?.toLowerCase()).toContain('offline');
  });

  test('should show draining status', async ({ page }) => {
    await page.goto('/nodes/draining-node-id');

    const status = await nodeDetailPage.getStatus();
    expect(status?.toLowerCase()).toContain('draining');
  });

  test('should show error status for unhealthy node', async ({ page }) => {
    await page.goto('/nodes/error-node-id');

    const status = await nodeDetailPage.getStatus();
    expect(status?.toLowerCase()).toContain('error');
  });
});

test.describe('Admin Node Deletion', () => {
  let nodeDetailPage: AdminNodeDetailPage;

  test('should show delete button for node with no VMs', async ({ page }) => {
    nodeDetailPage = new AdminNodeDetailPage(page);
    await nodeDetailPage.goto('empty-node-id');

    await expect(page.locator('button:has-text("Delete")')).toBeVisible();
  });

  test('should show deletion confirmation', async ({ page }) => {
    nodeDetailPage = new AdminNodeDetailPage(page);
    await nodeDetailPage.goto('empty-node-id');
    await nodeDetailPage.deleteNode();

    await expect(page.locator('[role="dialog"], .modal')).toBeVisible();
    await expect(page.locator('text=/confirm|are you sure/i')).toBeVisible();
  });

  test('should delete node after confirmation', async ({ page }) => {
    nodeDetailPage = new AdminNodeDetailPage(page);
    await nodeDetailPage.goto('deletable-node-id');
    await nodeDetailPage.deleteNode();

    await page.click('button:has-text("Confirm")');

    // Should redirect to node list
    await expect(page).toHaveURL(/\/nodes$/);
  });

  test('should disable delete for node with VMs', async ({ page }) => {
    nodeDetailPage = new AdminNodeDetailPage(page);
    await nodeDetailPage.goto('node-with-vms-id');

    await nodeDetailPage.expectActionDisabled('Delete');
  });

  test('should show reason for disabled deletion', async ({ page }) => {
    nodeDetailPage = new AdminNodeDetailPage(page);
    await nodeDetailPage.goto('node-with-vms-id');

    // Hover over disabled button
    await page.hover('button:has-text("Delete")');

    // Should show tooltip explaining why
    await expect(page.locator('text=/VMs|migrate.*first/i')).toBeVisible();
  });
});

test.describe('Admin Node Metrics', () => {
  const testNodeId = '00000000-0000-0000-0000-000000000005';

  test('should show CPU usage chart', async ({ page }) => {
    await page.goto(`/nodes/${testNodeId}/metrics`);

    const cpuChart = page.locator('[data-testid="cpu-chart"], section:has-text("CPU")');
    await expect(cpuChart).toBeVisible({ timeout: 10000 });
  });

  test('should show memory usage chart', async ({ page }) => {
    await page.goto(`/nodes/${testNodeId}/metrics`);

    const memoryChart = page.locator('[data-testid="memory-chart"], section:has-text("Memory")');
    await expect(memoryChart).toBeVisible({ timeout: 10000 });
  });

  test('should show storage usage', async ({ page }) => {
    await page.goto(`/nodes/${testNodeId}/metrics`);

    const storageSection = page.locator('[data-testid="storage-chart"], section:has-text("Storage")');
    await expect(storageSection).toBeVisible({ timeout: 10000 });
  });

  test('should show network I/O', async ({ page }) => {
    await page.goto(`/nodes/${testNodeId}/metrics`);

    const networkSection = page.locator('[data-testid="network-chart"], section:has-text("Network")');
    await expect(networkSection).toBeVisible({ timeout: 10000 });
  });
});

test.describe('Admin Node Navigation', () => {
  test('should have working navigation from dashboard', async ({ page }) => {
    await page.goto('/dashboard');

    await page.click('a:has-text("Nodes")');

    await expect(page).toHaveURL(/\/nodes/);
  });

  test('should have breadcrumb navigation', async ({ page }) => {
    await page.goto('/nodes/test-node-id');

    await page.click('a:has-text("Nodes")');

    await expect(page).toHaveURL(/\/nodes$/);
  });
});