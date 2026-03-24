import { settingsApi } from "@/lib/api-client";

export interface CustomerUser {
  id: string;
  email: string;
  role: string;
}

/**
 * Fetches the customer profile and transforms it into a CustomerUser object.
 * Used in auth-context to reduce duplicate profile-fetching logic.
 *
 * @returns CustomerUser object if successful, null if the fetch fails
 */
export async function fetchCustomerProfile(): Promise<CustomerUser | null> {
  try {
    const profile = await settingsApi.getProfile();
    return {
      id: profile.id,
      email: profile.email,
      role: "customer",
    };
  } catch {
    return null;
  }
}

/**
 * Fetches the customer profile after 2FA verification.
 * Profile fetch failure is treated as fatal — the caller should log the user
 * out and surface a clear error rather than constructing a fake user object.
 *
 * @throws If the profile fetch fails
 * @returns CustomerUser object with real profile data
 */
export async function fetchCustomerProfileAfter2FA(): Promise<CustomerUser> {
  const profile = await settingsApi.getProfile();
  return {
    id: profile.id,
    email: profile.email,
    role: "customer",
  };
}