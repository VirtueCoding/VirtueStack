import assert from "node:assert/strict";
import { test } from "node:test";

interface VMActionRefreshModule {
  VMActionRefreshError: typeof Error;
  completeVMActionWithRefresh: (
    mutate: () => Promise<void>,
    refresh?: () => Promise<void> | void,
  ) => Promise<void>;
}

async function loadVMActionRefreshModule(): Promise<VMActionRefreshModule | null> {
  try {
    return (await import(
      new URL("./vm-action-refresh.ts", import.meta.url).href
    )) as VMActionRefreshModule;
  } catch {
    return null;
  }
}

async function flushMicrotasks(times = 1): Promise<void> {
  for (let index = 0; index < times; index += 1) {
    await Promise.resolve();
  }
}

test("completeVMActionWithRefresh waits for follow-up refresh before resolving", async () => {
  const vmActionRefreshModule = await loadVMActionRefreshModule();
  assert.ok(
    vmActionRefreshModule?.completeVMActionWithRefresh,
    "completeVMActionWithRefresh should exist",
  );

  const calls: string[] = [];
  let resolveMutation!: () => void;
  let resolveRefresh!: () => void;

  const completionPromise = vmActionRefreshModule.completeVMActionWithRefresh(
    async () => new Promise<void>((resolve) => {
      calls.push("mutate");
      resolveMutation = resolve;
    }),
    async () => new Promise<void>((resolve) => {
      calls.push("refresh");
      resolveRefresh = resolve;
    }),
  );

  let resolved = false;
  void completionPromise.then(() => {
    resolved = true;
  });

  await flushMicrotasks();
  assert.deepEqual(calls, ["mutate"]);
  assert.equal(resolved, false);

  resolveMutation();
  await flushMicrotasks(2);
  assert.deepEqual(calls, ["mutate", "refresh"]);
  assert.equal(resolved, false);

  resolveRefresh();
  await completionPromise;
  assert.equal(resolved, true);
});

test("completeVMActionWithRefresh wraps refresh failures", async () => {
  const vmActionRefreshModule = await loadVMActionRefreshModule();
  assert.ok(
    vmActionRefreshModule?.completeVMActionWithRefresh,
    "completeVMActionWithRefresh should exist",
  );

  let rejectedError: unknown;

  try {
    await vmActionRefreshModule.completeVMActionWithRefresh(
      async () => {},
      async () => {
        throw new Error("refresh failed");
      },
    );
  } catch (error) {
    rejectedError = error;
  }

  assert.ok(rejectedError, "refresh failure should reject");
  assert.ok(
    rejectedError instanceof vmActionRefreshModule.VMActionRefreshError,
    "refresh failure should be wrapped as VMActionRefreshError",
  );
  assert.match(String((rejectedError as Error).cause ?? ""), /refresh failed/);
});
