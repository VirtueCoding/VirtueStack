/**
 * Authentication Utilities
 *
 * Provides TOTP generation, login helpers, and session management.
 */

import { createHmac } from 'crypto';
import type { Page, TestInfo } from '@playwright/test';

function envOrDefault(name: string, fallback: string): string {
  return process.env[name] || fallback;
}

/**
 * Base32 decode for TOTP secrets
 */
function base32Decode(input: string): Buffer {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
  const cleaned = input.replace(/=+$/, '');
  const bits: string[] = [];

  for (const char of cleaned.toUpperCase()) {
    const val = alphabet.indexOf(char);
    if (val === -1) continue;
    bits.push(val.toString(2).padStart(5, '0'));
  }

  const octets = bits.join('');
  const bytes = Buffer.alloc(Math.floor(octets.length / 8));

  for (let i = 0; i < bytes.length; i++) {
    bytes[i] = parseInt(octets.slice(i * 8, i * 8 + 8), 2);
  }

  return bytes;
}

/**
 * Generate TOTP code
 */
export function generateTOTP(secret: string, period = 30, digits = 6): string {
  const epoch = Math.floor(Date.now() / 1000 / period);
  const counter = Buffer.alloc(8);
  counter.writeUInt32BE(0, 0);
  counter.writeUInt32BE(epoch, 4);

  const key = base32Decode(secret.replace(/ /g, ''));
  const hmac = createHmac('sha1', key);
  hmac.update(counter);

  const bytes = hmac.digest();
  const offset = bytes[bytes.length - 1] & 0x0f;

  const binary =
    ((bytes[offset] & 0x7f) << 24) |
    ((bytes[offset + 1] & 0xff) << 16) |
    ((bytes[offset + 2] & 0xff) << 8) |
    (bytes[offset + 3] & 0xff);

  return (binary % Math.pow(10, digits)).toString().padStart(digits, '0');
}

export async function generateFreshTOTP(
  secret: string,
  period = 30,
  digits = 6,
  minRemainingMs = 5000
): Promise<string> {
  const periodMs = period * 1000;
  const remainingMs = periodMs - (Date.now() % periodMs);

  if (remainingMs < minRemainingMs) {
    await new Promise((resolve) => setTimeout(resolve, remainingMs + 250));
  }

  return generateTOTP(secret, period, digits);
}

/**
 * Test credentials
 */
export const CREDENTIALS = {
  admin: {
    email: envOrDefault('TEST_ADMIN_EMAIL', 'admin@test.virtuestack.local'),
    password: envOrDefault('TEST_ADMIN_PASSWORD', 'AdminTest123!'),
    totpSecret: envOrDefault('TEST_ADMIN_TOTP_SECRET', 'JBSWY3DPEHPK3PXP'),
  },
  adminWith2FA: {
    email: envOrDefault('TEST_ADMIN_2FA_EMAIL', '2fa-admin@test.virtuestack.local'),
    password: envOrDefault('TEST_ADMIN_2FA_PASSWORD', 'AdminTest123!'),
    totpSecret: envOrDefault('TEST_ADMIN_TOTP_SECRET', 'JBSWY3DPEHPK3PXP'),
  },
  customer: {
    email: envOrDefault('TEST_CUSTOMER_EMAIL', 'customer@test.virtuestack.local'),
    password: envOrDefault('TEST_CUSTOMER_PASSWORD', 'CustomerTest123!'),
    totpSecret: null,
  },
  customerWith2FA: {
    email: envOrDefault('TEST_CUSTOMER_2FA_EMAIL', '2fa-customer@test.virtuestack.local'),
    password: envOrDefault('TEST_CUSTOMER_2FA_PASSWORD', 'Customer2FA123!'),
    totpSecret: envOrDefault('TEST_CUSTOMER_TOTP_SECRET', 'KRSXG5DSN5XW4ZLP'),
  },
} as const;

const CONTROLLER_URL = envOrDefault('BASE_URL', 'http://localhost:8080');
const FORWARDED_IP_NAMESPACE = envOrDefault(
  'E2E_FORWARDED_IP_NAMESPACE',
  `${process.pid}-${Date.now()}`
);

function hashSeed(seed: string): number {
  let hash = 0;
  for (const char of seed) {
    hash = (hash * 31 + char.charCodeAt(0)) % 250;
  }
  return hash;
}

export function forwardedIPForSeed(seed: string): string {
  return `198.51.100.${hashSeed(`${FORWARDED_IP_NAMESPACE}:${seed}`) + 1}`;
}

export function forwardedIPForTest(testInfo: TestInfo, scope: string): string {
  return forwardedIPForSeed(
    `${scope}:${testInfo.project.name}:${testInfo.titlePath.join(' > ')}`
  );
}

export async function routeAPIRequestsFromIP(page: Page, ip: string): Promise<void> {
  await page.route(`${CONTROLLER_URL}/**`, async (route) => {
    await route.continue({
      headers: {
        ...route.request().headers(),
        'x-forwarded-for': ip,
      },
    });
  });
}

/**
 * Check if running in CI
 */
export const isCI = !!process.env.CI;

/**
 * Skip condition helpers
 */
export const skipConditions = {
  requiresAdminTOTP: !envOrDefault('TEST_ADMIN_TOTP_SECRET', ''),
  requiresCustomerTOTP: !envOrDefault('TEST_CUSTOMER_TOTP_SECRET', ''),
  requiresNodeAgent: !process.env.NODE_AGENT_URL,
  requiresRealKVM: process.env.SKIP_KVM_TESTS === 'true',
};
