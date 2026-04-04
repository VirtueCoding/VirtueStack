export type CaptureStatus = "processing" | "success" | "error";

export function getInitialPayPalCaptureStatus(token: string | null): CaptureStatus {
  return token ? "processing" : "error";
}

export function getPayPalCaptureViewState(options: {
  token: string | null;
  status: CaptureStatus;
}): {
  showProcessing: boolean;
  showSuccess: boolean;
  showError: boolean;
} {
  const { token, status } = options;

  return {
    showProcessing: Boolean(token) && status === "processing",
    showSuccess: status === "success",
    showError: !token || status === "error",
  };
}
