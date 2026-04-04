import assert from "node:assert/strict";
import { test } from "node:test";

interface OAuthStartStateModule {
  getOAuthStartFailureMessage: (provider: string) => string;
}

async function loadOAuthStartStateModule(): Promise<OAuthStartStateModule | null> {
  try {
    return (await import(
      new URL("./oauth-start-state.ts", import.meta.url).href
    )) as OAuthStartStateModule;
  } catch {
    return null;
  }
}

test("getOAuthStartFailureMessage surfaces provider-specific login failure feedback", async () => {
  const oauthStartStateModule = await loadOAuthStartStateModule();
  assert.ok(oauthStartStateModule, "oauth-start-state module should load");

  assert.equal(
    oauthStartStateModule.getOAuthStartFailureMessage("google"),
    "Unable to start Google sign-in right now. Please try again.",
  );
  assert.equal(
    oauthStartStateModule.getOAuthStartFailureMessage("github"),
    "Unable to start GitHub sign-in right now. Please try again.",
  );
});
