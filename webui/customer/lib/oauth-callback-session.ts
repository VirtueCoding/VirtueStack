export const OAUTH_PROFILE_LOAD_ERROR =
  "Unable to load your profile after authentication. Please log in again.";

interface AuthenticatedUser {
  id: string;
  email: string;
  role: string;
}

interface FinalizeOAuthSessionOptions<TUser extends AuthenticatedUser> {
  loadUser: () => Promise<TUser>;
  setAuthenticatedUser: (user: TUser) => void;
  logout: () => Promise<void>;
}

export async function finalizeOAuthSession<TUser extends AuthenticatedUser>(
  options: FinalizeOAuthSessionOptions<TUser>,
): Promise<TUser> {
  try {
    const user = await options.loadUser();
    options.setAuthenticatedUser(user);
    return user;
  } catch {
    await options.logout();
    throw new Error(OAUTH_PROFILE_LOAD_ERROR);
  }
}
