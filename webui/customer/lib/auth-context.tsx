"use client";

import {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
  ReactNode,
} from "react";
import { useRouter } from "next/navigation";
import {
  customerAuthApi,
  ApiClientError,
  type LoginRequest,
  type Verify2FARequest,
} from "./api-client";
import {
  fetchCustomerProfile,
  fetchCustomerProfileAfter2FA,
  type CustomerUser,
} from "./auth-utils";

interface AuthState {
  user: CustomerUser | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  requires2FA: boolean;
}

interface AuthContextType extends AuthState {
  login: (credentials: LoginRequest) => Promise<void>;
  verify2FA: (request: Verify2FARequest) => Promise<void>;
  logout: () => Promise<void>;
  clearError: () => void;
  reset2FA: () => void;
  error: string | null;
  tempToken: string | null;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

const AUTH_STATE_KEY = "customer_auth_state";

interface StoredAuthState {
  user: CustomerUser;
  isAuthenticated: boolean;
}

function loadStoredState(): StoredAuthState | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = sessionStorage.getItem(AUTH_STATE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw);
    if (parsed && parsed.user && parsed.isAuthenticated) return parsed;
    return null;
  } catch {
    return null;
  }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const router = useRouter();
  const [state, setState] = useState<AuthState>({
    user: null,
    isAuthenticated: false,
    isLoading: true,
    requires2FA: false,
  });
  const [tempToken, setTempToken] = useState<string | null>(null);
  const [pendingEmail, setPendingEmail] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const persistState = useCallback(
    (user: CustomerUser | null, isAuthenticated: boolean) => {
      if (typeof window === "undefined") return;
      if (user && isAuthenticated) {
        sessionStorage.setItem(
          AUTH_STATE_KEY,
          JSON.stringify({ user, isAuthenticated })
        );
      } else {
        sessionStorage.removeItem(AUTH_STATE_KEY);
      }
    },
    []
  );

  const clearError = useCallback(() => setError(null), []);

  const reset2FA = useCallback(() => {
    setState((prev) => ({ ...prev, requires2FA: false }));
    setTempToken(null);
    setPendingEmail(null);
    setError(null);
  }, []);

  const initAuth = useCallback(async () => {
    if (typeof window === "undefined") return;

    const stored = loadStoredState();
    if (stored) {
      const user = await fetchCustomerProfile();
      if (user) {
        setState({
          user,
          isAuthenticated: true,
          isLoading: false,
          requires2FA: false,
        });
        persistState(user, true);
      } else {
        sessionStorage.removeItem(AUTH_STATE_KEY);
        setState((prev) => ({ ...prev, isLoading: false }));
      }
      return;
    }

    const user = await fetchCustomerProfile();
    if (user) {
      setState({
        user,
        isAuthenticated: true,
        isLoading: false,
        requires2FA: false,
      });
      persistState(user, true);
    } else {
      setState((prev) => ({ ...prev, isLoading: false }));
    }
  }, [persistState]);

  useEffect(() => {
    initAuth();
  }, [initAuth]);

  const login = useCallback(
    async (credentials: LoginRequest) => {
      setError(null);
      setState((prev) => ({ ...prev, isLoading: true }));

      try {
        const tokens = await customerAuthApi.login(credentials);

        if (tokens.requires_2fa) {
          setState((prev) => ({ ...prev, isLoading: false, requires2FA: true }));
          setTempToken(tokens.temp_token || null);
          setPendingEmail(credentials.email);
          return;
        }

        // Fetch the real profile to get the UUID rather than using email as ID.
        const user = await fetchCustomerProfileAfter2FA();
        setState({
          user,
          isAuthenticated: true,
          isLoading: false,
          requires2FA: false,
        });
        setTempToken(null);
        setPendingEmail(null);
        persistState(user, true);
        router.push("/vms");
      } catch (err) {
        let message = "Login failed. Please try again.";
        if (err instanceof ApiClientError) {
          if (err.code === "INVALID_CREDENTIALS") {
            message = "Invalid email or password. Please try again.";
          } else {
            message = err.message;
          }
        }
        setError(message);
        setState((prev) => ({ ...prev, isLoading: false }));
      }
    },
    [router, persistState]
  );

  const verify2FA = useCallback(
    async (request: Verify2FARequest) => {
      setError(null);
      setState((prev) => ({ ...prev, isLoading: true }));

      if (!pendingEmail) {
        // This should never happen — if pendingEmail is missing the session is
        // corrupt. Log out and redirect to login instead of constructing a fake user.
        setError("Session expired. Please log in again.");
        setState({
          user: null,
          isAuthenticated: false,
          isLoading: false,
          requires2FA: false,
        });
        setTempToken(null);
        sessionStorage.removeItem(AUTH_STATE_KEY);
        router.push("/login");
        return;
      }

      try {
        await customerAuthApi.verify2FA(request);

        // Fetch the real profile to get the UUID. Failure is fatal — do not
        // construct a fake user object with the email as the id.
        let user: CustomerUser;
        try {
          user = await fetchCustomerProfileAfter2FA();
        } catch {
          // Profile fetch failed after successful 2FA — log out and surface error.
          setError("Unable to load your profile after verification. Please log in again.");
          await customerAuthApi.logout();
          setState({
            user: null,
            isAuthenticated: false,
            isLoading: false,
            requires2FA: false,
          });
          setTempToken(null);
          setPendingEmail(null);
          sessionStorage.removeItem(AUTH_STATE_KEY);
          router.push("/login");
          return;
        }

        setState({
          user,
          isAuthenticated: true,
          isLoading: false,
          requires2FA: false,
        });
        setTempToken(null);
        setPendingEmail(null);
        persistState(user, true);
        router.push("/vms");
      } catch (err) {
        let message = "2FA verification failed. Please try again.";
        if (err instanceof ApiClientError) {
          if (err.code === "INVALID_2FA_CODE") {
            message = "Invalid 2FA code. Please try again.";
          } else {
            message = err.message;
          }
        }
        setError(message);
        setState((prev) => ({ ...prev, isLoading: false }));
      }
    },
    [router, persistState, pendingEmail]
  );

  const logout = useCallback(async () => {
    setState((prev) => ({ ...prev, isLoading: true }));

    try {
      await customerAuthApi.logout();
    } catch (err) {
      // Logout errors are non-fatal — session may already be invalid.
      // Log for debugging but always clear local state regardless.
      console.warn("Logout request failed (session may already be invalid):", err);
    } finally {
      setState({
        user: null,
        isAuthenticated: false,
        isLoading: false,
        requires2FA: false,
      });
      setTempToken(null);
      sessionStorage.removeItem(AUTH_STATE_KEY);
      router.push("/login");
    }
  }, [router]);

  const value: AuthContextType = {
    ...state,
    login,
    verify2FA,
    logout,
    error,
    clearError,
    reset2FA,
    tempToken,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextType {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return context;
}
