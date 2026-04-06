import assert from "node:assert/strict";
import { test } from "node:test";

interface BillingRedirectModule {
  getSafeTopUpRedirectURL: (paymentURL: string) => string | null;
}

async function loadBillingRedirectModule(): Promise<BillingRedirectModule | null> {
  try {
    return (await import(
      new URL("./billing-redirect.ts", import.meta.url).href
    )) as BillingRedirectModule;
  } catch {
    return null;
  }
}

test("getSafeTopUpRedirectURL accepts https payment redirects", async () => {
  const billingRedirectModule = await loadBillingRedirectModule();
  assert.ok(billingRedirectModule, "billing-redirect module should load");

  assert.equal(
    billingRedirectModule.getSafeTopUpRedirectURL(
      "https://checkout.stripe.com/c/pay/cs_test_123",
    ),
    "https://checkout.stripe.com/c/pay/cs_test_123",
  );
  assert.equal(
    billingRedirectModule.getSafeTopUpRedirectURL(
      "https://pay.example.test/crypto/session-123",
    ),
    "https://pay.example.test/crypto/session-123",
  );
});

test("getSafeTopUpRedirectURL rejects malformed or unsafe redirects", async () => {
  const billingRedirectModule = await loadBillingRedirectModule();
  assert.ok(billingRedirectModule, "billing-redirect module should load");

  assert.equal(
    billingRedirectModule.getSafeTopUpRedirectURL("javascript:alert(1)"),
    null,
  );
  assert.equal(
    billingRedirectModule.getSafeTopUpRedirectURL("http://pay.example.test/crypto"),
    null,
  );
  assert.equal(
    billingRedirectModule.getSafeTopUpRedirectURL("/billing/paypal-return"),
    null,
  );
});
