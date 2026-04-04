import type { CustomerUser } from "./auth-utils";

interface FinalizeOAuthSessionOptions<TUser extends CustomerUser> {
  user: TUser | null;
  sessionCleanupToken?: string;
  setAuthenticatedUser: (user: TUser) => boolean | void;
  invalidateSession: (sessionCleanupToken?: string) => Promise<void>;
}

export const OAUTH_PROFILE_LOAD_ERROR =
  "Unable to load your profile after authentication. Please log in again.";
export const OAUTH_SESSION_STATE_UNKNOWN_ERROR =
  "We couldn't confirm that your session was signed out. Please refresh the page before trying again.";

export async function finalizeOAuthSession<TUser extends CustomerUser>(
  options: FinalizeOAuthSessionOptions<TUser>,
): Promise<{ user: TUser; didApplyAuthenticatedUser: true }> {
  if (!options.user) {
    try {
      await options.invalidateSession(options.sessionCleanupToken);
    } catch (invalidationError) {
      throw new Error(OAUTH_SESSION_STATE_UNKNOWN_ERROR, {
        cause: {
          invalidationError,
          profileError: null,
        },
      });
    }
    throw new Error(OAUTH_PROFILE_LOAD_ERROR);
  }

  const didApplyAuthenticatedUser = options.setAuthenticatedUser(options.user) !== false;
  if (!didApplyAuthenticatedUser) {
    try {
      await options.invalidateSession(options.sessionCleanupToken);
    } catch (invalidationError) {
      throw new Error(OAUTH_SESSION_STATE_UNKNOWN_ERROR, {
        cause: {
          invalidationError,
          profileError: null,
        },
      });
    }
    throw new Error(OAUTH_PROFILE_LOAD_ERROR);
  }

  return { user: options.user, didApplyAuthenticatedUser: true };
}
