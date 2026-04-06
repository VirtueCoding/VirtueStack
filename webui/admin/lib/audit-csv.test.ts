import assert from "node:assert/strict";
import { test } from "node:test";

interface AuditCSVModule {
  buildAuditLogCSV: (rows: ReadonlyArray<ReadonlyArray<string>>) => string;
}

async function loadAuditCSVModule(): Promise<AuditCSVModule | null> {
  try {
    return (await import(new URL("./audit-csv.ts", import.meta.url).href)) as AuditCSVModule;
  } catch {
    return null;
  }
}

test("buildAuditLogCSV neutralizes formula-like cells before quoting output", async () => {
  const auditCSVModule = await loadAuditCSVModule();
  assert.ok(auditCSVModule?.buildAuditLogCSV, "buildAuditLogCSV should exist");

  const csv = auditCSVModule.buildAuditLogCSV([
    ["Action", "Actor ID", "Resource ID", "IP Address"],
    ["=cmd|' /C calc'!A0", "+admin", "-resource", "@attacker"],
  ]);

  const expectedRow = [
    `"'=cmd|' /C calc'!A0"`,
    `"'+admin"`,
    `"'-resource"`,
    `"'@attacker"`,
  ].join(",");

  assert.equal(
    csv,
    [
      '"Action","Actor ID","Resource ID","IP Address"',
      expectedRow,
    ].join("\n"),
  );
});
