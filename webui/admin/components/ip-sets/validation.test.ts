import assert from "node:assert/strict";
import { test } from "node:test";

interface ValidationModule {
  extractValidImportAddresses?: (lines: readonly string[]) => string[];
}

async function loadValidationModule(): Promise<ValidationModule | null> {
  try {
    return (await import(new URL("./validation.ts", import.meta.url).href)) as ValidationModule;
  } catch {
    return null;
  }
}

test("extractValidImportAddresses rejects malformed IPv4 and CIDR entries", async () => {
  const validationModule = await loadValidationModule();
  assert.ok(validationModule?.extractValidImportAddresses, "extractValidImportAddresses should exist");

  const addresses = validationModule.extractValidImportAddresses([
    "999.1.1.1",
    "10.0.0.1/33",
    "10.0.0.256/24",
    "1.2.3/24",
    "10.0.0.5",
    "10.0.0.0/24",
  ]);

  assert.deepEqual(addresses, ["10.0.0.5", "10.0.0.0/24"]);
});
