import assert from "node:assert/strict";
import { test } from "node:test";

interface MutationRefreshModule {
  MutationRefreshError: typeof Error;
  completeMutationWithRefresh: (
    mutate: () => Promise<void>,
    refresh: () => Promise<void>,
    onSuccess: () => void,
    refreshErrorMessage: string,
  ) => Promise<void>;
}

async function loadMutationRefreshModule(): Promise<MutationRefreshModule | null> {
  try {
    return (await import(
      new URL("./mutation-refresh.ts", import.meta.url).href
    )) as MutationRefreshModule;
  } catch {
    return null;
  }
}

async function flushMicrotasks(times = 1): Promise<void> {
  for (let index = 0; index < times; index += 1) {
    await Promise.resolve();
  }
}

test("completeMutationWithRefresh calls success only after refresh finishes", async () => {
  const mutationRefreshModule = await loadMutationRefreshModule();
  assert.ok(
    mutationRefreshModule?.completeMutationWithRefresh,
    "completeMutationWithRefresh should exist",
  );

  const calls: string[] = [];
  let resolveMutation!: () => void;
  let resolveRefresh!: () => void;
  let successCalled = false;

  const completionPromise = mutationRefreshModule.completeMutationWithRefresh(
    async () => new Promise<void>((resolve) => {
      calls.push("mutate");
      resolveMutation = resolve;
    }),
    async () => new Promise<void>((resolve) => {
      calls.push("refresh");
      resolveRefresh = resolve;
    }),
    () => {
      calls.push("success");
      successCalled = true;
    },
    "list refresh failed",
  );

  await flushMicrotasks();
  assert.deepEqual(calls, ["mutate"]);
  assert.equal(successCalled, false);

  resolveMutation();
  await flushMicrotasks(2);
  assert.deepEqual(calls, ["mutate", "refresh"]);
  assert.equal(successCalled, false);

  resolveRefresh();
  await completionPromise;
  assert.deepEqual(calls, ["mutate", "refresh", "success"]);
  assert.equal(successCalled, true);
});

test("completeMutationWithRefresh wraps refresh failures and skips success", async () => {
  const mutationRefreshModule = await loadMutationRefreshModule();
  assert.ok(
    mutationRefreshModule?.completeMutationWithRefresh,
    "completeMutationWithRefresh should exist",
  );

  let successCalled = false;
  let rejectedError: unknown;

  try {
    await mutationRefreshModule.completeMutationWithRefresh(
      async () => {},
      async () => {
        throw new Error("refresh failed");
      },
      () => {
        successCalled = true;
      },
      "list refresh failed",
    );
  } catch (error) {
    rejectedError = error;
  }

  assert.ok(rejectedError, "refresh failure should reject");
  assert.ok(
    rejectedError instanceof mutationRefreshModule.MutationRefreshError,
    "refresh failure should be wrapped as MutationRefreshError",
  );
  assert.match(String((rejectedError as Error).cause ?? ""), /refresh failed/);
  assert.equal((rejectedError as Error).message, "list refresh failed");
  assert.equal(successCalled, false);
});
