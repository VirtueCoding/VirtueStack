import type { AdminUser } from "./api-client";

export const ADMIN_PROFILE_LOAD_ERROR =
  "Unable to load your profile after authentication. Please log in again.";
export const ADMIN_SESSION_STATE_UNKNOWN_ERROR =
  "We couldn't confirm that your session was signed out. Please refresh the page before trying again.";

export class AdminProfileLoadError extends Error {
  constructor(cause?: unknown) {
    super(ADMIN_PROFILE_LOAD_ERROR, { cause });
    this.name = "AdminProfileLoadError";
  }
}

export class AdminSessionStateUnknownError extends Error {
  constructor(cause?: unknown) {
    super(ADMIN_SESSION_STATE_UNKNOWN_ERROR, { cause });
    this.name = "AdminSessionStateUnknownError";
  }
}

interface FinalizeAuthenticatedSessionOptions<TUser extends AdminUser> {
  user: TUser | null;
  sessionCleanupToken?: string;
  setAuthenticatedUser: (user: TUser) => boolean | void;
  invalidateSession: (sessionCleanupToken?: string) => Promise<void>;
}

export async function finalizeAuthenticatedSession<TUser extends AdminUser>(
  options: FinalizeAuthenticatedSessionOptions<TUser>,
): Promise<{ user: TUser; didApplyAuthenticatedUser: true }> {
  if (!options.user) {
    try {
      await options.invalidateSession(options.sessionCleanupToken);
    } catch (invalidationError) {
      throw new AdminSessionStateUnknownError({
        invalidationError,
        profileError: null,
      });
    }
    throw new AdminProfileLoadError();
  }

  const didApplyAuthenticatedUser =
    options.setAuthenticatedUser(options.user) !== false;
  if (!didApplyAuthenticatedUser) {
    try {
      await options.invalidateSession(options.sessionCleanupToken);
    } catch (invalidationError) {
      throw new AdminSessionStateUnknownError({
        invalidationError,
        profileError: null,
      });
    }
    throw new AdminProfileLoadError();
  }
  return { user: options.user, didApplyAuthenticatedUser: true };
}
