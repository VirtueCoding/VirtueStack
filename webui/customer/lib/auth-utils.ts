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
 * Fetches the customer profile with email fallback.
 * Used when we have a pending email (e.g., during 2FA flow) and want to
 * fall back to it if the profile fetch fails.
 *
 * @param fallbackEmail - Email to use if profile fetch fails
 * @returns CustomerUser object with real profile data or fallback values
 */
export async function fetchCustomerProfileWithEmailFallback(
  fallbackEmail: string
): Promise<CustomerUser> {
  try {
    const profile = await settingsApi.getProfile();
    return {
      id: profile.id,
      email: profile.email,
      role: "customer",
    };
  } catch {
    // Non-fatal: fall back to provided email as a temporary placeholder
    return {
      id: fallbackEmail,
      email: fallbackEmail,
      role: "customer",
    };
  }
}