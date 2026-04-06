# VirtueStack E2E Testing Guide

This guide explains how to run and maintain E2E tests for VirtueStack.

## Quick Start

```bash
# 1. Setup the test environment (generates secrets, certs, seed data)
./scripts/setup-e2e.sh

# 2. Start all services
./scripts/setup-e2e.sh --start

# 3. Run the tests
cd tests/e2e && pnpm test

# 4. Cleanup when done
./scripts/setup-e2e.sh --clean
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    Docker Stack (Controller Side)                │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────────┐ │
│  │PostgreSQL│  │   NATS   │  │Controller│  │ WebUIs + Nginx   │ │
│  │   :5432  │  │  :4222   │  │  :8080   │  │  :3000/:3001     │ │
│  └──────────┘  └──────────┘  └──────────┘  └──────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
                                     │ gRPC (mTLS)
                                     ▼
┌─────────────────────────────────────────────────────────────────┐
│              KVM Hypervisor Node (Node Agent Side)               │
│  ┌──────────────┐  ┌──────────┐  ┌──────────────────────────┐   │
│  │  Node Agent  │  │  libvirt │  │  VMs (QEMU/KVM)          │   │
│  │   :50051     │  │  daemon  │  │  + Ceph/QCOW storage     │   │
│  └──────────────┘  └──────────┘  └──────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## Test Categories

### 1. WebUI Tests (No KVM Required)
These tests only need the Docker stack running:

- `auth.spec.ts` - Authentication flows
- `admin-vm.spec.ts` - Admin VM management UI
- `admin-vm-pom.spec.ts` - Admin VM management UI (Page Object Model pattern)
- `admin-nodes.spec.ts` - Node management UI
- `admin-plans.spec.ts` - Plan management UI
- `admin-templates.spec.ts` - Template management UI
- `admin-ip-sets.spec.ts` - IP pool management UI
- `admin-customers.spec.ts` - Customer management UI
- `customer-vm.spec.ts` - Customer VM operations
- `customer-vm-pom.spec.ts` - Customer VM operations (Page Object Model pattern)
- `customer-backup.spec.ts` - Backup management
- `customer-snapshot.spec.ts` - Snapshot management
- `customer-settings.spec.ts` - Settings, 2FA, API keys

### 2. Full Stack Tests (Require KVM Node Agent)
These tests require a real Node Agent running on a KVM host:

- VM creation/deletion operations
- Live migration tests
- Snapshot creation on real storage
- Backup creation on real storage

## Test Credentials

The seed data creates these test users:

| User | Email | Password | 2FA Secret |
|------|-------|----------|------------|
| Admin | admin@test.virtuestack.local | AdminTest123! | - |
| Admin (2FA) | 2fa-admin@test.virtuestack.local | AdminTest123! | JBSWY3DPEHPK3PXP |
| Customer | customer@test.virtuestack.local | CustomerTest123! | - |
| Customer (2FA) | 2fa-customer@test.virtuestack.local | CustomerTest123! | KRSXG5DSN5XW4ZLP |

## Running Specific Tests

```bash
# Run only admin tests
pnpm run test:admin

# Run only customer tests
pnpm run test:customer

# Run only auth tests
pnpm run test:auth

# Run in headed mode (see browser)
pnpm run test:headed

# Debug tests
pnpm run test:debug

# Run on specific browser
pnpm run test:chromium
pnpm run test:firefox
pnpm run test:webkit
```

## Test Data IDs

The seed script creates predictable UUIDs for testing:

| Resource | ID Pattern |
|----------|------------|
| Plans | `11111111-1111-1111-1111-111111111001` - `004` |
| Locations | `22222222-2222-2222-2222-222222222001` - `002` |
| Nodes | `33333333-3333-3333-3333-333333333001` - `005` |
| IP Sets | `44444444-4444-4444-4444-444444444001` - `002` |
| Templates | `66666666-6666-6666-6666-666666666001` - `005` |
| Admins | `77777777-7777-7777-7777-777777777001` - `003` |
| Customers | `88888888-8888-8888-8888-888888888001` - `003` |
| VMs | `99999999-9999-9999-9999-999999999001` - `003` |
| Backups | `aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa01` - `03` |
| Snapshots | `bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb01` - `03` |
| API Keys | `cccccccc-cccc-cccc-cccc-cccccccccc01` - `02` |
| Webhooks | `dddddddd-dddd-dddd-dddd-dddddddddd01` |

## Environment Variables

Set these for the tests:

```bash
# URLs
ADMIN_URL=http://localhost:3000
CUSTOMER_URL=http://localhost:3001
BASE_URL=http://localhost:8080

# Test credentials (for tests that need TOTP)
TEST_ADMIN_TOTP_SECRET=JBSWY3DPEHPK3PXP
TEST_CUSTOMER_TOTP_SECRET=KRSXG5DSN5XW4ZLP
```

## CI/CD Integration

The GitHub Actions workflow `.github/workflows/e2e.yml` runs E2E tests on:

- Push to `main` affecting WebUI or API code
- Pull requests affecting WebUI or API code
- Manual trigger with browser selection

### Running in CI

```bash
# All tests on push to main
git push origin main

# Manual trigger for one browser family across all UI/auth projects
gh workflow run e2e.yml -f project_group="chromium"
```

## Mock Node Agent

For testing without real KVM, we use Wiremock to mock gRPC responses:

```bash
# Start mock node agent
docker run -d \
  --name mock-node-agent \
  -p 50051:50051 \
  -v $(pwd)/tests/e2e/mocks:/home/wiremock \
  wiremock/wiremock:3 \
  --port 50051 --verbose
```

## Writing New Tests

### Project Structure

```
tests/e2e/
├── pages/                    # Page Object Models
│   ├── BasePage.ts          # Base class with common methods
│   ├── AdminLoginPage.ts    # Admin login page
│   ├── AdminDashboardPage.ts
│   ├── AdminVMListPage.ts
│   ├── AdminVMDetailPage.ts
│   ├── CustomerLoginPage.ts
│   ├── CustomerDashboardPage.ts
│   ├── CustomerVMListPage.ts
│   ├── CustomerVMDetailPage.ts
│   ├── CustomerConsolePage.ts
│   └── index.ts             # Exports all pages
├── utils/                    # Utilities
│   ├── auth.ts              # TOTP generation, credentials
│   ├── api.ts               # API client, test IDs
│   └── index.ts             # Exports all utilities
├── fixtures/                 # Custom Playwright fixtures
│   └── index.ts             # adminTest, customerTest
├── auth.setup.ts            # Authentication state setup
└── *.spec.ts                # Test files
```

### Page Object Model Pattern

Page Objects encapsulate page-specific logic for better maintainability:

```typescript
// pages/MyNewPage.ts
import { Page, expect, Locator } from '@playwright/test';
import { BasePage } from './BasePage';

export class MyNewPage extends BasePage {
  readonly title: Locator;
  readonly submitButton: Locator;

  constructor(page: Page) {
    super(page);
    this.title = page.locator('h1');
    this.submitButton = page.locator('button[type="submit"]');
  }

  async goto() {
    await this.navigate('/my-page');
    await expect(this.title).toContainText(/my page/i);
  }

  async submitForm(value: string) {
    await this.fillInput('field', value);
    await this.submitButton.click();
  }
}
```

#### Using Custom Fixtures

Use the custom fixtures for cleaner test code:

```typescript
// my-feature.spec.ts
import { test, expect } from './fixtures';

test.describe('My Feature', () => {
  test.use({ storageState: '.auth/admin-storage.json' });

  test('should work', async ({ adminDashboardPage }) => {
    await adminDashboardPage.goto();
    // Page object is already instantiated
  });
});
```

#### Creating New Fixtures

Add new page objects to fixtures/index.ts:

```typescript
export const adminTest = base.extend<AdminFixtures>({
  myNewPage: async ({ page }, use) => {
    await use(new MyNewPage(page));
  },
});
```

### Using Test IDs

Add `data-testid` attributes to components for stable selectors:

```tsx
// Component
<button data-testid="submit-btn" type="submit">Submit</button>

// Test
await page.click('[data-testid="submit-btn"]');

// Or use the helper from BasePage
await myNewPage.getByTestId('submit-btn').click();
```

### Test Data IDs

The seed script creates predictable UUIDs for testing:

```typescript
import { TEST_IDS } from './utils/api';

// Use in tests
await adminVMDetailPage.goto(TEST_IDS.vms.testVM1);
```

| Resource | ID Pattern |
|----------|------------|
| Plans | `11111111-1111-1111-1111-111111111001` - `004` |
| Nodes | `33333333-3333-3333-3333-333333333001` - `005` |
| Templates | `66666666-6666-6666-6666-666666666001` - `005` |
| Customers | `88888888-8888-8888-8888-888888888001` - `003` |
| VMs | `99999999-9999-9999-9999-999999999001` - `003` |

### API Client Helpers

Use the API clients for setup/teardown or direct API testing:

```typescript
import { AdminAPIClient, CustomerAPIClient } from './utils/api';

test('should work', async ({ request }) => {
  const adminApi = new AdminAPIClient(request);
  const { data, status } = await adminApi.get('/api/v1/admin/vms');
  expect(status).toBe(200);
});
```

### Waiting for API Responses

```typescript
// Wait for specific API response
await page.waitForResponse(resp =>
  resp.url().includes('/api/v1/customer/vms') &&
  resp.status() === 200
);
```

## Troubleshooting

### Tests Timing Out

1. Increase timeout: `test('...', async ({ page }) => {}).timeout(60000)`
2. Add explicit waits: `await page.waitForSelector('[data-testid="loaded"]')`
3. Check network conditions

### Flaky Tests

1. Use `test.describe.configure({ retries: 2 })`
2. Avoid `waitForTimeout` - use explicit waits
3. Check for race conditions

### Auth State Issues

```bash
# Regenerate auth state files
cd tests/e2e
pnpm exec playwright test --project=setup-admin --project=setup-customer
```

## Debugging

```bash
# Debug mode with Playwright Inspector
pnpm run test:debug

# Generate test code by recording actions
pnpm run codegen -- http://localhost:3000

# View trace after failure
pnpm exec playwright show-trace test-results/trace.zip
```
