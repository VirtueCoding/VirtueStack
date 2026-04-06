import assert from "node:assert/strict";
import { test } from "node:test";

interface NotificationsModule {
  parseUnreadCountEventData: (raw: string) => number | null;
}

async function loadNotificationsModule(): Promise<NotificationsModule | null> {
  try {
    return (await import(
      new URL("./index.ts", import.meta.url).href
    )) as NotificationsModule;
  } catch {
    return null;
  }
}

test("parseUnreadCountEventData returns the unread count for valid payloads", async () => {
  const notificationsModule = await loadNotificationsModule();
  assert.ok(notificationsModule, "notifications module should load");

  assert.equal(notificationsModule.parseUnreadCountEventData('{"count":3}'), 3);
});

test("parseUnreadCountEventData rejects malformed or invalid payloads", async () => {
  const notificationsModule = await loadNotificationsModule();
  assert.ok(notificationsModule, "notifications module should load");

  assert.equal(notificationsModule.parseUnreadCountEventData("{"), null);
  assert.equal(notificationsModule.parseUnreadCountEventData('{"count":"3"}'), null);
  assert.equal(notificationsModule.parseUnreadCountEventData("{}"), null);
});
