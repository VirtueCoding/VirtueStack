import { settingsApi } from "./api-client";
import { shouldTreatProfileErrorAsUnauthenticated } from "./auth-error";

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
  } catch (error) {
    if (shouldTreatProfileErrorAsUnauthenticated(error)) {
      return null;
    }
    throw error;
  }
}
