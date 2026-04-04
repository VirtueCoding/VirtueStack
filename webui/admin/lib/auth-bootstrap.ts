export function advanceAuthVersion(currentVersion: number): number {
  return currentVersion + 1;
}

export function canApplyBootstrapResult(
  bootstrapVersion: number,
  currentVersion: number,
): boolean {
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
