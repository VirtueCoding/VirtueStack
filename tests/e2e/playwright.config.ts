import { defineConfig, devices } from '@playwright/test';

const adminURL = process.env.ADMIN_URL || 'http://localhost:3000';
const customerURL = process.env.CUSTOMER_URL || 'http://localhost:3001';
const controllerURL = process.env.BASE_URL || 'http://localhost:8080';
const apiBaseURL = process.env.NEXT_PUBLIC_API_URL || `${controllerURL}/api/v1`;
const shellSafeAPIBaseURL = apiBaseURL.replace(/'/g, `'\\''`);
const adminSpecPattern = /(^|\/)admin[^/]*\.spec\.ts$/;
const customerSpecPattern = /(^|\/)customer[^/]*\.spec\.ts$/;

/**
 * VirtueStack E2E Test Configuration
 * 
 * This configuration runs tests against:
 * - Admin Web UI (http://localhost:3000)
 * - Customer Web UI (http://localhost:3001)
 * 
 * Run tests with: npx playwright test
 * Run specific browser: npx playwright test --project=chromium
 * Run in UI mode: npx playwright test --ui
 */

export default defineConfig({
  // Test directory
  testDir: '.',
  
  // Run tests in parallel
  fullyParallel: true,
  
  // Fail build on CI if you accidentally left test.only in source code
  forbidOnly: !!process.env.CI,
  
  // Retry failed tests on CI
  retries: process.env.CI ? 2 : 0,
  
  // Parallel workers (limit on CI)
  workers: process.env.CI ? 1 : undefined,
  
  // Reporter configuration
  reporter: [
    ['html', { outputFolder: 'playwright-report' }],
    ['json', { outputFile: 'test-results/results.json' }],
    ['list'],
  ],
  
  // Global test settings
  use: {
    // Base URL for admin UI tests
    baseURL: adminURL,
    
    // Collect trace on failure
    trace: 'on-first-retry',
    
    // Screenshot on failure
    screenshot: 'only-on-failure',
    
    // Video on failure
    video: 'retain-on-failure',
    
    // Timeout for each action
    actionTimeout: 10000,
    
    // Navigation timeout
    navigationTimeout: 30000,
  },
  
  // Configure projects for different browsers and environments
  projects: [
    {
      name: 'setup-auth',
      testMatch: /auth\.setup\.ts/,
      use: {
        ...devices['Desktop Chrome'],
      },
    },

    // Admin UI Tests
    {
      name: 'admin-chromium',
      testMatch: adminSpecPattern,
      dependencies: ['setup-auth'],
      use: {
        ...devices['Desktop Chrome'],
        baseURL: adminURL,
        storageState: '.auth/admin-storage.json',
      },
    },
    {
      name: 'admin-firefox',
      testMatch: adminSpecPattern,
      dependencies: ['setup-auth'],
      use: {
        ...devices['Desktop Firefox'],
        baseURL: adminURL,
        storageState: '.auth/admin-storage.json',
      },
    },
    {
      name: 'admin-webkit',
      testMatch: adminSpecPattern,
      dependencies: ['setup-auth'],
      use: {
        ...devices['Desktop Safari'],
        baseURL: adminURL,
        storageState: '.auth/admin-storage.json',
      },
    },
    
    // Customer UI Tests
    {
      name: 'customer-chromium',
      testMatch: customerSpecPattern,
      dependencies: ['setup-auth'],
      use: {
        ...devices['Desktop Chrome'],
        baseURL: customerURL,
        storageState: '.auth/customer-storage.json',
      },
    },
    {
      name: 'customer-firefox',
      testMatch: customerSpecPattern,
      dependencies: ['setup-auth'],
      use: {
        ...devices['Desktop Firefox'],
        baseURL: customerURL,
        storageState: '.auth/customer-storage.json',
      },
    },
    {
      name: 'customer-webkit',
      testMatch: customerSpecPattern,
      dependencies: ['setup-auth'],
      use: {
        ...devices['Desktop Safari'],
        baseURL: customerURL,
        storageState: '.auth/customer-storage.json',
      },
    },
    
    // Auth Tests (both UIs)
    {
      name: 'auth-chromium',
      testMatch: /auth\.spec\.ts/,
      use: {
        ...devices['Desktop Chrome'],
      },
    },
    {
      name: 'auth-firefox',
      testMatch: /auth\.spec\.ts/,
      use: {
        ...devices['Desktop Firefox'],
      },
    },
    {
      name: 'auth-webkit',
      testMatch: /auth\.spec\.ts/,
      use: {
        ...devices['Desktop Safari'],
      },
    },
    
    // Mobile tests
    {
      name: 'mobile-chrome',
      testMatch: customerSpecPattern,
      dependencies: ['setup-auth'],
      use: {
        ...devices['Pixel 5'],
        baseURL: customerURL,
        storageState: '.auth/customer-storage.json',
      },
    },
    {
      name: 'mobile-safari',
      testMatch: customerSpecPattern,
      dependencies: ['setup-auth'],
      use: {
        ...devices['iPhone 12'],
        baseURL: customerURL,
        storageState: '.auth/customer-storage.json',
      },
    },
  ],
  
  // Web server for admin UI
  webServer: [
    {
      command: `NEXT_PUBLIC_API_URL='${shellSafeAPIBaseURL}' npm run dev --prefix ../../webui/admin`,
      url: adminURL,
      reuseExistingServer: !process.env.CI,
      timeout: 120000,
    },
    {
      command: `NEXT_PUBLIC_API_URL='${shellSafeAPIBaseURL}' npm run dev --prefix ../../webui/customer`,
      url: customerURL,
      reuseExistingServer: !process.env.CI,
      timeout: 120000,
    },
  ],
  
  // Output directory for test artifacts
  outputDir: 'test-results/',
  
  // Timeout for entire test
  timeout: 60000,
  
  // Expect timeout
  expect: {
    timeout: 10000,
  },
});
