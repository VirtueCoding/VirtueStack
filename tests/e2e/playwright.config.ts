import { defineConfig, devices } from '@playwright/test';

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
    baseURL: process.env.ADMIN_URL || 'http://localhost:3000',
    
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
    // Admin UI Tests
    {
      name: 'admin-chromium',
      testMatch: /admin.*\.spec\.ts/,
      use: {
        ...devices['Desktop Chrome'],
        baseURL: process.env.ADMIN_URL || 'http://localhost:3000',
        storageState: '.auth/admin-storage.json',
      },
    },
    {
      name: 'admin-firefox',
      testMatch: /admin.*\.spec\.ts/,
      use: {
        ...devices['Desktop Firefox'],
        baseURL: process.env.ADMIN_URL || 'http://localhost:3000',
        storageState: '.auth/admin-storage.json',
      },
    },
    {
      name: 'admin-webkit',
      testMatch: /admin.*\.spec\.ts/,
      use: {
        ...devices['Desktop Safari'],
        baseURL: process.env.ADMIN_URL || 'http://localhost:3000',
        storageState: '.auth/admin-storage.json',
      },
    },
    
    // Customer UI Tests
    {
      name: 'customer-chromium',
      testMatch: /customer.*\.spec\.ts/,
      use: {
        ...devices['Desktop Chrome'],
        baseURL: process.env.CUSTOMER_URL || 'http://localhost:3001',
        storageState: '.auth/customer-storage.json',
      },
    },
    {
      name: 'customer-firefox',
      testMatch: /customer.*\.spec\.ts/,
      use: {
        ...devices['Desktop Firefox'],
        baseURL: process.env.CUSTOMER_URL || 'http://localhost:3001',
        storageState: '.auth/customer-storage.json',
      },
    },
    {
      name: 'customer-webkit',
      testMatch: /customer.*\.spec\.ts/,
      use: {
        ...devices['Desktop Safari'],
        baseURL: process.env.CUSTOMER_URL || 'http://localhost:3001',
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
      testMatch: /customer.*\.spec\.ts/,
      use: {
        ...devices['Pixel 5'],
        baseURL: process.env.CUSTOMER_URL || 'http://localhost:3001',
      },
    },
    {
      name: 'mobile-safari',
      testMatch: /customer.*\.spec\.ts/,
      use: {
        ...devices['iPhone 12'],
        baseURL: process.env.CUSTOMER_URL || 'http://localhost:3001',
      },
    },
  ],
  
  // Web server for admin UI
  webServer: [
    {
      command: 'npm run dev --prefix ../../webui/admin',
      url: 'http://localhost:3000',
      reuseExistingServer: !process.env.CI,
      timeout: 120000,
    },
    {
      command: 'npm run dev --prefix ../../webui/customer',
      url: 'http://localhost:3001',
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