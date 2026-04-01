import type { CustomerUser } from "./auth-utils";

export const OAUTH_PROFILE_LOAD_ERROR =
  "Unable to load your profile after authentication. Please log in again.";

interface FinalizeOAuthSessionOptions<TUser extends CustomerUser> {
  loadUser: () => Promise<TUser>;
  setAuthenticatedUser: (user: TUser) => void;
  logout: () => Promise<void>;
}

export async function finalizeOAuthSession<TUser extends CustomerUser>(
  options: FinalizeOAuthSessionOptions<TUser>,
): Promise<TUser> {
  let user: TUser;
  try {
    user = await options.loadUser();
  } catch (error) {
    await options.logout();
    throw new Error(OAUTH_PROFILE_LOAD_ERROR, { cause: error });
  }

  options.setAuthenticatedUser(user);
  return user;
}
