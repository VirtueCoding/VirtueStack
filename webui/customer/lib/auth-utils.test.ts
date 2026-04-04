import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";

test("auth utils no longer expose a stale post-2fa profile helper", async () => {
  const authUtilsSource = await readFile(
    new URL("./auth-utils.ts", import.meta.url),
    "utf8",
  );

  assert.equal(
    authUtilsSource.includes("export async function fetchCustomerProfileAfter2FA"),
    false,
    "direct auth responses should be used instead of a second profile fetch",
  );
});
