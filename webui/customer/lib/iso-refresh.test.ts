import assert from "node:assert/strict";
import { test } from "node:test";

interface ISORefreshModule {
  ISOMutationRefreshError: typeof Error;
  loadMutationRefreshData: <T>(
    load: () => Promise<T>,
    apply: (data: T) => void,
  ) => Promise<void>;
  refreshISOMutationState: (
    refreshISOs: () => Promise<void>,
    refreshVM: () => Promise<void>,
  ) => Promise<void>;
  completeISOMutationWithRefresh: (
    mutate: () => Promise<void>,
    refreshISOs: () => Promise<void>,
    refreshVM: () => Promise<void>,
    onSuccess: () => void,
  ) => Promise<void>;
}

async function flushMicrotasks(times = 1): Promise<void> {
  for (let index = 0; index < times; index += 1) {
    await Promise.resolve();
  }
}

test("loadMutationRefreshData applies loaded data", async () => {
  const isoRefreshModule = await loadISORefreshModule();
  assert.ok(isoRefreshModule?.loadMutationRefreshData, "loadMutationRefreshData should exist");

  let appliedValue = "";

  await isoRefreshModule.loadMutationRefreshData(
    async () => "fresh-data",
    (value) => {
      appliedValue = value;
    },
  );

  assert.equal(appliedValue, "fresh-data");
});

test("loadMutationRefreshData rethrows load failures without applying stale data", async () => {
  const isoRefreshModule = await loadISORefreshModule();
  assert.ok(isoRefreshModule?.loadMutationRefreshData, "loadMutationRefreshData should exist");

  let applied = false;

  await assert.rejects(
    isoRefreshModule.loadMutationRefreshData(
      async () => {
        throw new Error("load failed");
      },
      () => {
        applied = true;
      },
    ),
    /load failed/,
  );

  assert.equal(applied, false);
});

async function loadISORefreshModule(): Promise<ISORefreshModule | null> {
  try {
    return (await import(
      new URL("./iso-refresh.ts", import.meta.url).href
    )) as ISORefreshModule;
  } catch {
    return null;
  }
}

test("refreshISOMutationState refreshes ISO list and VM details before resolving", async () => {
  const isoRefreshModule = await loadISORefreshModule();
  assert.ok(isoRefreshModule?.refreshISOMutationState, "refreshISOMutationState should exist");

  const calls: string[] = [];
  let resolveISOs!: () => void;
  let resolveVM!: () => void;

  const refreshPromise = isoRefreshModule.refreshISOMutationState(
    async () => new Promise<void>((resolve) => {
      calls.push("isos");
      resolveISOs = resolve;
    }),
    async () => new Promise<void>((resolve) => {
      calls.push("vm");
      resolveVM = resolve;
    }),
  );

  let resolved = false;
  void refreshPromise.then(() => {
    resolved = true;
  });

  await Promise.resolve();
  assert.deepEqual(calls, ["isos", "vm"]);
  assert.equal(resolved, false);

  resolveISOs();
  await Promise.resolve();
  assert.equal(resolved, false);

  resolveVM();
  await refreshPromise;
  assert.equal(resolved, true);
});

test("completeISOMutationWithRefresh calls success only after mutation and both refreshes complete", async () => {
  const isoRefreshModule = await loadISORefreshModule();
  assert.ok(
    isoRefreshModule?.completeISOMutationWithRefresh,
    "completeISOMutationWithRefresh should exist",
  );

  const calls: string[] = [];
  let resolveMutation!: () => void;
  let resolveISOs!: () => void;
  let resolveVM!: () => void;
  let successCalled = false;

  const completionPromise = isoRefreshModule.completeISOMutationWithRefresh(
    async () => new Promise<void>((resolve) => {
      calls.push("mutate");
      resolveMutation = resolve;
    }),
    async () => new Promise<void>((resolve) => {
      calls.push("isos");
      resolveISOs = resolve;
    }),
    async () => new Promise<void>((resolve) => {
      calls.push("vm");
      resolveVM = resolve;
    }),
    () => {
      calls.push("success");
      successCalled = true;
    },
  );

  await flushMicrotasks();
  assert.deepEqual(calls, ["mutate"]);
  assert.equal(successCalled, false);

  resolveMutation();
  await flushMicrotasks(2);
  assert.deepEqual(calls, ["mutate", "isos", "vm"]);
  assert.equal(successCalled, false);

  resolveISOs();
  await flushMicrotasks();
  assert.equal(successCalled, false);

  resolveVM();
  await completionPromise;
  assert.deepEqual(calls, ["mutate", "isos", "vm", "success"]);
  assert.equal(successCalled, true);
});

test("completeISOMutationWithRefresh does not call success when refresh fails", async () => {
  const isoRefreshModule = await loadISORefreshModule();
  assert.ok(
    isoRefreshModule?.completeISOMutationWithRefresh,
    "completeISOMutationWithRefresh should exist",
  );

  let successCalled = false;
  let rejectedError: unknown;

  try {
    await isoRefreshModule.completeISOMutationWithRefresh(
      async () => {},
      async () => {
        throw new Error("refresh failed");
      },
      async () => {},
      () => {
        successCalled = true;
      },
    );
  } catch (error) {
    rejectedError = error;
  }

  assert.ok(rejectedError, "refresh failure should reject");
  assert.ok(
    rejectedError instanceof isoRefreshModule.ISOMutationRefreshError,
    "refresh failure should be wrapped as ISOMutationRefreshError",
  );
  assert.match(String((rejectedError as Error).cause ?? ""), /refresh failed/);
  assert.equal(successCalled, false);
});
