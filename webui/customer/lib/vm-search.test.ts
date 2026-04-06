import assert from "node:assert/strict";
import { test } from "node:test";

interface VMSearchModule {
  getURLSyncedSearchTerm: (
    currentSearchTerm: string,
    searchParam: string | null,
  ) => string;
}

async function loadVMSearchModule(): Promise<VMSearchModule | null> {
  try {
    return (await import(
      new URL("./vm-search.ts", import.meta.url).href
    )) as VMSearchModule;
  } catch {
    return null;
  }
}

test("getURLSyncedSearchTerm adopts a new search query from the URL", async () => {
  const vmSearchModule = await loadVMSearchModule();
  assert.ok(vmSearchModule, "vm-search module should load");

  assert.equal(
    vmSearchModule.getURLSyncedSearchTerm("old value", "new search"),
    "new search",
  );
});

test("getURLSyncedSearchTerm clears stale local state when the URL search is removed", async () => {
  const vmSearchModule = await loadVMSearchModule();
  assert.ok(vmSearchModule, "vm-search module should load");

  assert.equal(
    vmSearchModule.getURLSyncedSearchTerm("stale local search", null),
    "",
  );
});
