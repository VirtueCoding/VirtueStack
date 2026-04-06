import type { CustomerUser } from "./auth-utils";

interface StoredAuthState {
  user: CustomerUser;
  isAuthenticated: boolean;
}

export function advanceAuthVersion(currentVersion: number): number {
  return currentVersion + 1;
}

export function canApplyBootstrapResult(bootstrapVersion: number, currentVersion: number): boolean {
  return bootstrapVersion === currentVersion;
}

export function getCancelled2FAState() {
  return {
    requires2FA: false,
    isLoading: false,
  };
}

export function applyAuthenticatedUserIfCurrent<TUser>(
  user: TUser,
  expectedVersion: number,
  currentVersion: number,
  applyAuthenticatedUser: (user: TUser) => void,
): boolean {
  if (!canApplyBootstrapResult(expectedVersion, currentVersion)) {
    return false;
  }

  applyAuthenticatedUser(user);
  return true;
}

export function getProfileBootstrapErrorState(_stored: StoredAuthState | null) {
  return {
    user: null,
    isAuthenticated: false,
    isLoading: false,
    requires2FA: false,
    hasBootstrapError: true,
  };
}

export function shouldRedirectToLogin(state: {
  isAuthenticated: boolean;
  isLoading: boolean;
  hasBootstrapError: boolean;
}) {
  return getProtectedRouteView(state).kind === "redirect";
}

export function getProtectedRouteView(state: {
  isAuthenticated: boolean;
  isLoading: boolean;
  hasBootstrapError: boolean;
}) {
  if (state.isLoading) {
    return { kind: "loading" } as const;
  }

  if (state.hasBootstrapError) {
    return {
      kind: "retryable-error",
      fallbackPath: "/login",
      allowRetry: true,
    } as const;
  }

  if (state.isAuthenticated) {
    return { kind: "content" } as const;
  }

  return { kind: "redirect", path: "/login" } as const;
}

export function getHomeRedirectPath(state: {
  isAuthenticated: boolean;
  isLoading: boolean;
  hasBootstrapError: boolean;
}): "/vms" | "/login" | null {
  if (state.isLoading || state.hasBootstrapError) {
    return null;
  }

  if (state.isAuthenticated) {
    return "/vms";
  }

  return "/login";
}

export function getLoginRedirectMethod() {
  return "replace";
}

export function shouldRevalidateSession(state: {
  isAuthenticated: boolean;
  isLoading: boolean;
  requires2FA: boolean;
  hasBootstrapError: boolean;
  lastRevalidatedAtMs: number;
  nowMs: number;
  minIntervalMs?: number;
  force?: boolean;
}) {
  if (state.isLoading || state.requires2FA) {
    return false;
  }

  if (!state.isAuthenticated && !state.hasBootstrapError) {
    return false;
  }

  if (state.force) {
    return true;
  }

  const minIntervalMs = state.minIntervalMs ?? 15_000;
  return state.nowMs - state.lastRevalidatedAtMs >= minIntervalMs;
}

export function getAuthSyncAction(rawEvent: string | null): "clear-auth" | null {
  if (!rawEvent) {
    return null;
  }

  try {
    const parsed = JSON.parse(rawEvent);
    if (parsed?.type === "logout" || parsed?.type === "session-invalidated") {
      return "clear-auth";
    }
  } catch {
    return null;
  }

  return null;
}

export function applyRevalidationResultIfCurrent<TResult>(
  result: TResult,
  expectedRequestId: number,
  latestRequestId: number,
  applyResult: (result: TResult) => void,
): boolean {
  if (expectedRequestId !== latestRequestId) {
    return false;
  }

  applyResult(result);
  return true;
}

export function shouldPublishSessionInvalidated(state: {
  isAuthenticated: boolean;
}) {
  return state.isAuthenticated;
}
