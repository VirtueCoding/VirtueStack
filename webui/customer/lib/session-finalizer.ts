import type { CustomerUser } from "./auth-utils";

export const CUSTOMER_PROFILE_LOAD_ERROR =
  "Unable to load your profile after authentication. Please log in again.";
export const CUSTOMER_SESSION_STATE_UNKNOWN_ERROR =
  "We couldn't confirm that your session was signed out. Please refresh the page before trying again.";

export class CustomerProfileLoadError extends Error {
  constructor(cause?: unknown) {
    super(CUSTOMER_PROFILE_LOAD_ERROR, { cause });
    this.name = "CustomerProfileLoadError";
  }
}

export class CustomerSessionStateUnknownError extends Error {
  constructor(cause?: unknown) {
    super(CUSTOMER_SESSION_STATE_UNKNOWN_ERROR, { cause });
    this.name = "CustomerSessionStateUnknownError";
  }
}

interface FinalizeAuthenticatedSessionOptions<TUser extends CustomerUser> {
  user: TUser | null;
  sessionCleanupToken?: string;
  setAuthenticatedUser: (user: TUser) => boolean | void;
  invalidateSession: (sessionCleanupToken?: string) => Promise<void>;
}

export async function finalizeAuthenticatedSession<TUser extends CustomerUser>(
  options: FinalizeAuthenticatedSessionOptions<TUser>,
): Promise<{ user: TUser; didApplyAuthenticatedUser: true }> {
  if (!options.user) {
    try {
      await options.invalidateSession(options.sessionCleanupToken);
    } catch (invalidationError) {
      throw new CustomerSessionStateUnknownError({
        invalidationError,
        profileError: null,
      });
    }
    throw new CustomerProfileLoadError();
  }

  const didApplyAuthenticatedUser =
    options.setAuthenticatedUser(options.user) !== false;
  if (!didApplyAuthenticatedUser) {
    try {
      await options.invalidateSession(options.sessionCleanupToken);
    } catch (invalidationError) {
      throw new CustomerSessionStateUnknownError({
        invalidationError,
        profileError: null,
      });
    }
    throw new CustomerProfileLoadError();
  }
  return { user: options.user, didApplyAuthenticatedUser: true };
}
