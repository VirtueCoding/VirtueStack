import assert from "node:assert/strict";
import { test } from "node:test";

interface DialogActionModule {
  completeDialogAction: (
    action: () => Promise<void>,
    onSuccess: () => void,
  ) => Promise<boolean>;
}

async function loadDialogActionModule(): Promise<DialogActionModule | null> {
  try {
    return (await import(
      new URL("./dialog-action.ts", import.meta.url).href
    )) as DialogActionModule;
  } catch {
    return null;
  }
}

test("completeDialogAction returns true and runs success logic after a successful action", async () => {
  const dialogActionModule = await loadDialogActionModule();
  assert.ok(dialogActionModule?.completeDialogAction, "completeDialogAction should exist");

  const calls: string[] = [];
  const succeeded = await dialogActionModule.completeDialogAction(
    async () => {
      calls.push("action");
    },
    () => {
      calls.push("success");
    },
  );

  assert.equal(succeeded, true);
  assert.deepEqual(calls, ["action", "success"]);
});

test("completeDialogAction swallows rejected actions and skips success logic", async () => {
  const dialogActionModule = await loadDialogActionModule();
  assert.ok(dialogActionModule?.completeDialogAction, "completeDialogAction should exist");

  let resolved = false;
  let successCalled = false;
  await dialogActionModule.completeDialogAction(
    async () => {
      throw new Error("action failed");
    },
    () => {
      successCalled = true;
    },
  ).then((succeeded) => {
    resolved = succeeded;
  });

  assert.equal(resolved, false);
  assert.equal(successCalled, false);
});
