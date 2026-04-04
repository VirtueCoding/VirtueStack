import assert from "node:assert/strict";
import { test } from "node:test";

interface ISOLimitModule {
  getISOUploadDescription: (maxISOSizeBytes?: number) => string;
  getISOUploadHint: (maxISOSizeBytes?: number) => string;
  validateISOUploadFile: (
    file: { name: string; size: number },
    maxISOSizeBytes?: number,
  ) => string | null;
}

async function loadISOLimitModule(): Promise<ISOLimitModule | null> {
  try {
    return (await import(
      new URL("./iso-limit.ts", import.meta.url).href
    )) as ISOLimitModule;
  } catch {
    return null;
  }
}

const gib = 1024 * 1024 * 1024;

test("getISOUploadDescription uses backend-provided ISO size limit", async () => {
  const isoLimitModule = await loadISOLimitModule();
  assert.ok(isoLimitModule?.getISOUploadDescription, "getISOUploadDescription should exist");

  assert.equal(
    isoLimitModule.getISOUploadDescription(5 * gib),
    "Upload an ISO image to attach to this VM. Maximum file size is 5 GB.",
  );
});

test("getISOUploadHint uses backend-provided ISO size limit", async () => {
  const isoLimitModule = await loadISOLimitModule();
  assert.ok(isoLimitModule?.getISOUploadHint, "getISOUploadHint should exist");

  assert.equal(
    isoLimitModule.getISOUploadHint(6 * gib),
    "Only .iso files are accepted (max 6 GB)",
  );
});

test("validateISOUploadFile rejects files above backend-provided ISO limit", async () => {
  const isoLimitModule = await loadISOLimitModule();
  assert.ok(isoLimitModule?.validateISOUploadFile, "validateISOUploadFile should exist");

  assert.equal(
    isoLimitModule.validateISOUploadFile({ name: "installer.iso", size: 6 * gib }, 5 * gib),
    "File size exceeds the 5 GB limit",
  );
});

test("validateISOUploadFile skips size validation when backend limit is unavailable", async () => {
  const isoLimitModule = await loadISOLimitModule();
  assert.ok(isoLimitModule?.validateISOUploadFile, "validateISOUploadFile should exist");

  assert.equal(
    isoLimitModule.validateISOUploadFile({ name: "installer.iso", size: 6 * gib }, undefined),
    null,
  );
});
