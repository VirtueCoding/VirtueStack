import assert from "node:assert/strict";
import { test } from "node:test";

interface RebootStateModule {
  getPostRebootFailureState: () => {
    shouldDisconnect: boolean;
    shouldReconnect: boolean;
    shouldKeepRebooting: boolean;
  };
  getPostRebootSuccessState: () => {
    shouldDisconnect: boolean;
    shouldReconnect: boolean;
    shouldKeepRebooting: boolean;
  };
}

async function loadRebootStateModule(): Promise<RebootStateModule | null> {
  try {
    return (await import(
      new URL("./reboot-state.ts", import.meta.url).href
    )) as RebootStateModule;
  } catch {
    return null;
  }
}

test("getPostRebootFailureState keeps the console session open", async () => {
  const rebootStateModule = await loadRebootStateModule();
  assert.ok(rebootStateModule, "reboot-state module should load");

  assert.deepEqual(rebootStateModule.getPostRebootFailureState(), {
    shouldDisconnect: false,
    shouldReconnect: false,
    shouldKeepRebooting: false,
  });
});

test("getPostRebootSuccessState disconnects and schedules reconnect", async () => {
  const rebootStateModule = await loadRebootStateModule();
  assert.ok(rebootStateModule, "reboot-state module should load");

  assert.deepEqual(rebootStateModule.getPostRebootSuccessState(), {
    shouldDisconnect: true,
    shouldReconnect: true,
    shouldKeepRebooting: true,
  });
});
