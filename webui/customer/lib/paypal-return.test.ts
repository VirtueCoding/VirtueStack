import assert from "node:assert/strict";
import { test } from "node:test";

type CaptureStatus = "processing" | "success" | "error";

interface PayPalReturnModule {
  getInitialPayPalCaptureStatus: (token: string | null) => CaptureStatus;
  getPayPalCaptureViewState: (options: {
    token: string | null;
    status: CaptureStatus;
  }) => {
    showProcessing: boolean;
    showSuccess: boolean;
    showError: boolean;
  };
}

async function loadPayPalReturnModule(): Promise<PayPalReturnModule | null> {
  try {
    return (await import(
      new URL("./paypal-return.ts", import.meta.url).href
    )) as PayPalReturnModule;
  } catch {
    return null;
  }
}

test("getInitialPayPalCaptureStatus returns error when the PayPal token is missing", async () => {
  const paypalReturnModule = await loadPayPalReturnModule();
  assert.ok(
    paypalReturnModule?.getInitialPayPalCaptureStatus,
    "getInitialPayPalCaptureStatus should exist",
  );

  assert.equal(
    paypalReturnModule.getInitialPayPalCaptureStatus(null),
    "error",
  );
});

test("getPayPalCaptureViewState hides processing when the PayPal token is missing", async () => {
  const paypalReturnModule = await loadPayPalReturnModule();
  assert.ok(
    paypalReturnModule?.getPayPalCaptureViewState,
    "getPayPalCaptureViewState should exist",
  );

  assert.deepEqual(
    paypalReturnModule.getPayPalCaptureViewState({
      token: null,
      status: "processing",
    }),
    {
      showProcessing: false,
      showSuccess: false,
      showError: true,
    },
  );
});
