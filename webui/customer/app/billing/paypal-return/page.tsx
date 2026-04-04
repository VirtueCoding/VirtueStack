"use client";

import { useSearchParams, useRouter } from "next/navigation";
import { useEffect, useState, Suspense } from "react";
import { billingApi } from "@/lib/api-client";
import { RequireAuth } from "@/lib/require-auth";
import { Sidebar } from "@/components/sidebar";
import { MobileNav } from "@/components/mobile-nav";
import { ThemeToggle } from "@/components/theme-toggle";
import {
  getInitialPayPalCaptureStatus,
  getPayPalCaptureViewState,
  type CaptureStatus,
} from "@/lib/paypal-return";

function PayPalCaptureContent() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const token = searchParams.get("token");
  const [status, setStatus] = useState<CaptureStatus>(
    getInitialPayPalCaptureStatus(token),
  );
  const viewState = getPayPalCaptureViewState({ token, status });

  useEffect(() => {
    if (!token) {
      return;
    }

    let isActive = true;
    let redirectTimer: ReturnType<typeof setTimeout> | undefined;

    billingApi
      .capturePayPalOrder(token)
      .then(() => {
        if (!isActive) {
          return;
        }
        setStatus("success");
        redirectTimer = setTimeout(() => router.push("/billing"), 2000);
      })
      .catch(() => {
        if (isActive) {
          setStatus("error");
        }
      });

    return () => {
      isActive = false;
      if (redirectTimer !== undefined) {
        clearTimeout(redirectTimer);
      }
    };
  }, [token, router]);

  return (
    <div className="flex items-center justify-center min-h-[400px]">
      {viewState.showProcessing && (
        <div className="text-center space-y-2">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent mx-auto" />
          <p className="text-muted-foreground">Processing your PayPal payment...</p>
        </div>
      )}
      {viewState.showSuccess && (
        <p className="text-green-600 font-medium">
          Payment successful! Redirecting to billing...
        </p>
      )}
      {viewState.showError && (
        <div className="text-center space-y-2">
          <p className="text-red-600 font-medium">
            Payment could not be completed.
          </p>
          <button
            onClick={() => router.push("/billing")}
            className="text-sm text-primary underline"
          >
            Return to billing
          </button>
        </div>
      )}
    </div>
  );
}

export default function PayPalReturnPage() {
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);

  return (
    <RequireAuth>
      <div className="flex h-screen">
        <Sidebar collapsed={sidebarCollapsed} onToggle={() => setSidebarCollapsed(!sidebarCollapsed)} />
        <div className="flex-1 flex flex-col overflow-hidden">
          <header className="flex h-14 items-center justify-between border-b px-4 lg:px-6">
            <MobileNav />
            <h1 className="text-lg font-semibold">PayPal Payment</h1>
            <ThemeToggle />
          </header>
          <main className="flex-1 overflow-y-auto p-4 lg:p-6">
            <Suspense fallback={<div className="flex items-center justify-center min-h-[400px]"><p>Loading...</p></div>}>
              <PayPalCaptureContent />
            </Suspense>
          </main>
        </div>
      </div>
    </RequireAuth>
  );
}
