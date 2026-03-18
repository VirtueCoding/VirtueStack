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
  adminAuthApi,
  ApiClientError,
  type AdminUser,
  type LoginRequest,
  type Verify2FARequest,
} from "./api-client";

interface AuthState {
  user: AdminUser | null;
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

const AUTH_STATE_KEY = "admin_auth_state";

interface StoredAuthState {
  user: AdminUser;
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
    (user: AdminUser | null, isAuthenticated: boolean) => {
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

    try {
      // Validate the stored session against the server on every page load.
      // This prevents XSS-injected sessionStorage data from granting access.
      //
      // We use POST /admin/auth/refresh as a lightweight session probe: it
      // validates the HttpOnly refresh-token cookie server-side and reissues
      // fresh access+refresh cookies.  A 401 means no valid session exists.
      //
      // TODO: Add a dedicated GET /admin/auth/me endpoint that returns the
      // current user's identity (id, email, role) so we can always use the
      // server as the authoritative source for the user object rather than
      // trusting sessionStorage-cached data post-validation.
      const serverUser = await adminAuthApi.me(stored?.user ?? null);
      if (!serverUser) {
        sessionStorage.removeItem(AUTH_STATE_KEY);
        setState({ user: null, isAuthenticated: false, isLoading: false, requires2FA: false });
        return;
      }
      setState({
        user: serverUser,
        isAuthenticated: true,
        isLoading: false,
        requires2FA: false,
      });
      sessionStorage.setItem(AUTH_STATE_KEY, JSON.stringify({ user: serverUser, isAuthenticated: true }));
    } catch {
      // Network error or unexpected failure — clear stored state so we don't
      // grant access based on unvalidated cached data.
      sessionStorage.removeItem(AUTH_STATE_KEY);
      setState({ user: null, isAuthenticated: false, isLoading: false, requires2FA: false });
    }
  }, []);

  useEffect(() => {
    initAuth();
  }, [initAuth]);

  const login = useCallback(
    async (credentials: LoginRequest) => {
      setError(null);
      setState((prev) => ({ ...prev, isLoading: true }));

      try {
        const tokens = await adminAuthApi.login(credentials);

        if (tokens.requires_2fa) {
          setState((prev) => ({ ...prev, isLoading: false, requires2FA: true }));
          setTempToken(tokens.temp_token || null);
          setPendingEmail(credentials.email);
          return;
        }

        const user: AdminUser = {
          id: credentials.email,
          email: credentials.email,
          role: "admin",
        };
        setState({
          user,
          isAuthenticated: true,
          isLoading: false,
          requires2FA: false,
        });
        setTempToken(null);
        setPendingEmail(null);
        persistState(user, true);
        router.push("/");
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

      try {
        await adminAuthApi.verify2FA(request);
        const user: AdminUser = {
          id: pendingEmail || "",
          email: pendingEmail || "",
          role: "admin",
        };
        setState({
          user,
          isAuthenticated: true,
          isLoading: false,
          requires2FA: false,
        });
        setTempToken(null);
        setPendingEmail(null);
        persistState(user, true);
        router.push("/");
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
      await adminAuthApi.logout();
    } catch (err) {
      // Logout errors are non-fatal — session may already be invalid.
      // Log for debugging but don't propagate to prevent UI from hanging.
      console.warn('Logout request failed (session may already be invalid):', err);
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
