import assert from "node:assert/strict";
import { test } from "node:test";

interface SettingsAuthModule {
  shouldEnableSettingsQueries: (state: {
    isAuthenticated: boolean;
    isLoading: boolean;
    hasBootstrapError: boolean;
  }) => boolean;
}

async function loadSettingsAuthModule(): Promise<SettingsAuthModule | null> {
  try {
    return (await import(
      new URL("./settings-auth.ts", import.meta.url).href
    )) as SettingsAuthModule;
  } catch {
    return null;
  }
}

test("shouldEnableSettingsQueries waits for a verified authenticated customer session", async () => {
  const settingsAuthModule = await loadSettingsAuthModule();
  assert.ok(settingsAuthModule, "settings-auth module should load");

  assert.equal(
    settingsAuthModule.shouldEnableSettingsQueries({
      isAuthenticated: false,
      isLoading: true,
      hasBootstrapError: false,
    }),
    false,
  );
  assert.equal(
    settingsAuthModule.shouldEnableSettingsQueries({
      isAuthenticated: false,
      isLoading: false,
      hasBootstrapError: false,
    }),
    false,
  );
  assert.equal(
    settingsAuthModule.shouldEnableSettingsQueries({
      isAuthenticated: true,
      isLoading: false,
      hasBootstrapError: true,
    }),
    false,
  );
  assert.equal(
    settingsAuthModule.shouldEnableSettingsQueries({
      isAuthenticated: true,
      isLoading: false,
      hasBootstrapError: false,
    }),
    true,
  );
});
