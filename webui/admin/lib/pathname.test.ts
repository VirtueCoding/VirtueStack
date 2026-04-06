import assert from "node:assert/strict";
import { test } from "node:test";

interface PathnameModule {
  ADMIN_BASE_PATH: string;
  stripAdminBasePath: (pathname: string | null | undefined) => string;
  isAdminLoginPath: (pathname: string | null | undefined) => boolean;
  isAdminNavItemActive: (
    pathname: string | null | undefined,
    href: string,
  ) => boolean;
}

async function loadPathnameModule(): Promise<PathnameModule | null> {
  try {
    return (await import(
      new URL("./pathname.ts", import.meta.url).href
    )) as PathnameModule;
  } catch {
    return null;
  }
}

test("stripAdminBasePath removes the admin base path from mounted routes", async () => {
  const pathnameModule = await loadPathnameModule();
  assert.ok(pathnameModule, "pathname module should load");

  assert.equal(pathnameModule.ADMIN_BASE_PATH, "/admin");
  assert.equal(pathnameModule.stripAdminBasePath("/admin"), "/");
  assert.equal(pathnameModule.stripAdminBasePath("/admin/login"), "/login");
  assert.equal(pathnameModule.stripAdminBasePath("/admin/settings"), "/settings");
});

test("stripAdminBasePath preserves already-unprefixed admin routes", async () => {
  const pathnameModule = await loadPathnameModule();
  assert.ok(pathnameModule, "pathname module should load");

  assert.equal(pathnameModule.stripAdminBasePath("/login"), "/login");
  assert.equal(pathnameModule.stripAdminBasePath("/dashboard"), "/dashboard");
  assert.equal(pathnameModule.stripAdminBasePath(""), "/");
  assert.equal(pathnameModule.stripAdminBasePath(undefined), "/");
});

test("isAdminLoginPath recognizes login paths with or without the admin base path", async () => {
  const pathnameModule = await loadPathnameModule();
  assert.ok(pathnameModule, "pathname module should load");

  assert.equal(pathnameModule.isAdminLoginPath("/login"), true);
  assert.equal(pathnameModule.isAdminLoginPath("/admin/login"), true);
  assert.equal(pathnameModule.isAdminLoginPath("/admin/dashboard"), false);
});

test("isAdminNavItemActive compares nav hrefs after removing the admin base path", async () => {
  const pathnameModule = await loadPathnameModule();
  assert.ok(pathnameModule, "pathname module should load");

  assert.equal(pathnameModule.isAdminNavItemActive("/dashboard", "/dashboard"), true);
  assert.equal(pathnameModule.isAdminNavItemActive("/admin/dashboard", "/dashboard"), true);
  assert.equal(pathnameModule.isAdminNavItemActive("/admin/settings", "/dashboard"), false);
});
