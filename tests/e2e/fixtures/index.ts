/**
 * Test Fixtures
 *
 * Custom Playwright fixtures for VirtueStack E2E tests.
 */

import { test as base, Page, APIRequestContext } from '@playwright/test';
import {
  AdminLoginPage,
  AdminDashboardPage,
  AdminVMListPage,
  AdminVMDetailPage,
  CustomerLoginPage,
  CustomerDashboardPage,
  CustomerVMListPage,
  CustomerVMDetailPage,
  CustomerConsolePage,
} from '../pages';
import { AdminAPIClient, CustomerAPIClient } from '../utils/api';

// Define custom fixture types
type AdminFixtures = {
  adminLoginPage: AdminLoginPage;
  adminDashboardPage: AdminDashboardPage;
  adminVMListPage: AdminVMListPage;
  adminVMDetailPage: AdminVMDetailPage;
  adminApiClient: AdminAPIClient;
};

type CustomerFixtures = {
  customerLoginPage: CustomerLoginPage;
  customerDashboardPage: CustomerDashboardPage;
  customerVMListPage: CustomerVMListPage;
  customerVMDetailPage: CustomerVMDetailPage;
  customerConsolePage: CustomerConsolePage;
  customerApiClient: CustomerAPIClient;
};

// Admin test fixtures
export const adminTest = base.extend<AdminFixtures>({
  adminLoginPage: async ({ page }, use) => {
    await use(new AdminLoginPage(page));
  },
  adminDashboardPage: async ({ page }, use) => {
    await use(new AdminDashboardPage(page));
  },
  adminVMListPage: async ({ page }, use) => {
    await use(new AdminVMListPage(page));
  },
  adminVMDetailPage: async ({ page }, use) => {
    await use(new AdminVMDetailPage(page));
  },
  adminApiClient: async ({ request }, use) => {
    await use(new AdminAPIClient(request));
  },
});

// Customer test fixtures
export const customerTest = base.extend<CustomerFixtures>({
  customerLoginPage: async ({ page }, use) => {
    await use(new CustomerLoginPage(page));
  },
  customerDashboardPage: async ({ page }, use) => {
    await use(new CustomerDashboardPage(page));
  },
  customerVMListPage: async ({ page }, use) => {
    await use(new CustomerVMListPage(page));
  },
  customerVMDetailPage: async ({ page }, use) => {
    await use(new CustomerVMDetailPage(page));
  },
  customerConsolePage: async ({ page }, use) => {
    await use(new CustomerConsolePage(page));
  },
  customerApiClient: async ({ request }, use) => {
    await use(new CustomerAPIClient(request));
  },
});

// Re-export expect for convenience
export { expect } from '@playwright/test';