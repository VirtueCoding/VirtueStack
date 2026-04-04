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
  };
}
