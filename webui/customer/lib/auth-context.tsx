"use client";

import {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
  useRef,
  ReactNode,
} from "react";
import { useRouter } from "next/navigation";
import { toast } from "@virtuestack/ui";
import {
  customerAuthApi,
  ApiClientError,
  type LoginRequest,
  type Verify2FARequest,
} from "./api-client";
import {
  fetchCustomerProfile,
  type CustomerUser,
} from "./auth-utils";
import {
  advanceAuthVersion,
  applyAuthenticatedUserIfCurrent,
  canApplyBootstrapResult,
  getCancelled2FAState,
  getProfileBootstrapErrorState,
} from "./auth-bootstrap";
import {
  CustomerProfileLoadError,
  CustomerSessionStateUnknownError,
  finalizeAuthenticatedSession,
} from "./session-finalizer";

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
  setAuthenticatedUser: (user: CustomerUser) => void;
  getAuthVersion: () => number;
  guardedSetAuthenticatedUser: (
    expectedVersion: number,
    user: CustomerUser,
  ) => boolean;
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
  const authVersionRef = useRef(0);

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

  const setAuthenticatedUser = useCallback(
    (user: CustomerUser) => {
      authVersionRef.current = advanceAuthVersion(authVersionRef.current);
      setState({
        user,
        isAuthenticated: true,
        isLoading: false,
        requires2FA: false,
      });
      setTempToken(null);
      setPendingEmail(null);
      setError(null);
      persistState(user, true);
    },
    [persistState]
  );

  const getAuthVersion = useCallback(() => authVersionRef.current, []);

  const guardedSetAuthenticatedUser = useCallback(
    (expectedVersion: number, user: CustomerUser): boolean => {
      return applyAuthenticatedUserIfCurrent(
        user,
        expectedVersion,
        authVersionRef.current,
        setAuthenticatedUser,
      );
    },
    [setAuthenticatedUser]
  );

  const clearAuthenticatedState = useCallback(() => {
    authVersionRef.current = advanceAuthVersion(authVersionRef.current);
    setState({
      user: null,
      isAuthenticated: false,
      isLoading: false,
      requires2FA: false,
    });
    setTempToken(null);
    setPendingEmail(null);
    persistState(null, false);
  }, [persistState]);

  const reset2FA = useCallback(() => {
    authVersionRef.current = advanceAuthVersion(authVersionRef.current);
    setState((prev) => ({ ...prev, ...getCancelled2FAState() }));
    setTempToken(null);
    setPendingEmail(null);
    setError(null);
  }, []);

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }

    let isActive = true;
    const bootstrapVersion = authVersionRef.current;

    const initializeAuth = async () => {
      const stored = loadStoredState();
      let user: CustomerUser | null = null;

      try {
        user = await fetchCustomerProfile();
      } catch {
        if (
          !isActive ||
          !canApplyBootstrapResult(bootstrapVersion, authVersionRef.current)
        ) {
          return;
        }

        setError("Unable to verify your session right now. Please try again.");
        setState(getProfileBootstrapErrorState(stored));
        return;
      }

      if (
        !isActive ||
        !canApplyBootstrapResult(bootstrapVersion, authVersionRef.current)
      ) {
        return;
      }

      if (user) {
        setState({
          user,
          isAuthenticated: true,
          isLoading: false,
          requires2FA: false,
        });
        persistState(user, true);
        return;
      }

      if (stored) {
        sessionStorage.removeItem(AUTH_STATE_KEY);
      }
      setState((prev) => ({ ...prev, isLoading: false }));
    };

    void initializeAuth();

    return () => {
      isActive = false;
    };
  }, [persistState]);

  const login = useCallback(
    async (credentials: LoginRequest) => {
      setError(null);
      setState((prev) => ({ ...prev, isLoading: true }));

      try {
        const tokens = await customerAuthApi.login(credentials);

        if (tokens.requires_2fa) {
          authVersionRef.current = advanceAuthVersion(authVersionRef.current);
          setState((prev) => ({ ...prev, isLoading: false, requires2FA: true }));
          setTempToken(tokens.temp_token || null);
          setPendingEmail(credentials.email);
          return;
        }

        await finalizeAuthenticatedSession({
          user: tokens.user ?? null,
          sessionCleanupToken: tokens.session_cleanup_token,
          invalidateSession: customerAuthApi.invalidateSession,
          setAuthenticatedUser,
        });
        router.push("/vms");
      } catch (err) {
        let message = "Login failed. Please try again.";
        if (err instanceof ApiClientError) {
          if (err.code === "INVALID_CREDENTIALS") {
            message = "Invalid email or password. Please try again.";
          } else {
            message = err.message;
          }
        } else if (
          err instanceof CustomerProfileLoadError ||
          err instanceof CustomerSessionStateUnknownError
        ) {
          message = err.message;
          clearAuthenticatedState();
          setError(message);
          router.push("/login");
          return;
        } else if (err instanceof Error) {
          message = err.message;
        }
        setError(message);
        setState((prev) => ({ ...prev, isLoading: false }));
      }
    },
    [clearAuthenticatedState, router, setAuthenticatedUser]
  );

  const verify2FA = useCallback(
    async (request: Verify2FARequest) => {
      setError(null);
      setState((prev) => ({ ...prev, isLoading: true }));
      const verificationVersion = authVersionRef.current;

      if (!pendingEmail) {
        // This should never happen — if pendingEmail is missing the session is
        // corrupt. Log out and redirect to login instead of constructing a fake user.
        setError("Session expired. Please log in again.");
        clearAuthenticatedState();
        router.push("/login");
        return;
      }

      try {
        const tokens = await customerAuthApi.verify2FA(request);

        const { didApplyAuthenticatedUser } = await finalizeAuthenticatedSession({
          user: tokens.user ?? null,
          sessionCleanupToken: tokens.session_cleanup_token,
          invalidateSession: customerAuthApi.invalidateSession,
          setAuthenticatedUser: (user) =>
            guardedSetAuthenticatedUser(verificationVersion, user),
        });
        if (!didApplyAuthenticatedUser) {
          return;
        }
        router.push("/vms");
      } catch (err) {
        if (err instanceof CustomerSessionStateUnknownError) {
          clearAuthenticatedState();
          setError(err.message);
          router.push("/login");
          return;
        }
        if (!canApplyBootstrapResult(verificationVersion, authVersionRef.current)) {
          return;
        }
        let message = "2FA verification failed. Please try again.";
        if (err instanceof ApiClientError) {
          if (err.code === "INVALID_2FA_CODE") {
            message = "Invalid 2FA code. Please try again.";
          } else {
            message = err.message;
          }
        } else if (
          err instanceof CustomerProfileLoadError ||
          err instanceof CustomerSessionStateUnknownError
        ) {
          message = err.message;
          clearAuthenticatedState();
          setError(message);
          router.push("/login");
          return;
        } else if (err instanceof Error) {
          message = err.message;
        }
        setError(message);
        setState((prev) => ({ ...prev, isLoading: false }));
      }
    },
    [clearAuthenticatedState, guardedSetAuthenticatedUser, router, pendingEmail]
  );

  const logout = useCallback(async () => {
    setError(null);
    setState((prev) => ({ ...prev, isLoading: true }));

    try {
      await customerAuthApi.logout();
      clearAuthenticatedState();
      router.push("/login");
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to log out. Please try again.";
      setError(message);
      setState((prev) => ({ ...prev, isLoading: false }));
      toast({
        title: "Logout failed",
        description: message,
        variant: "destructive",
      });
    }
  }, [clearAuthenticatedState, router]);

  const value: AuthContextType = {
    ...state,
    login,
    verify2FA,
    logout,
    setAuthenticatedUser,
    getAuthVersion,
    guardedSetAuthenticatedUser,
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
