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
  adminAuthApi,
  ApiClientError,
  type AdminUser,
  type LoginRequest,
  type Verify2FARequest,
} from "./api-client";
import {
  AdminProfileLoadError,
  AdminSessionStateUnknownError,
  finalizeAuthenticatedSession,
} from "./session-finalizer";
import {
  advanceAuthVersion,
  applyAuthenticatedUserIfCurrent,
  canApplyBootstrapResult,
  getCancelled2FAState,
} from "./auth-bootstrap";

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

export function AuthProvider({ children }: { children: ReactNode }) {
  const router = useRouter();
  const [state, setState] = useState<AuthState>({
    user: null,
    isAuthenticated: false,
    isLoading: true,
    requires2FA: false,
  });
  const [tempToken, setTempToken] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const authVersionRef = useRef(0);

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

  const setAuthenticatedUser = useCallback(
    (user: AdminUser) => {
      authVersionRef.current = advanceAuthVersion(authVersionRef.current);
      setState({
        user,
        isAuthenticated: true,
        isLoading: false,
        requires2FA: false,
      });
      setTempToken(null);
      setError(null);
      persistState(user, true);
    },
    [persistState]
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
    persistState(null, false);
  }, [persistState]);

  const reset2FA = useCallback(() => {
    authVersionRef.current = advanceAuthVersion(authVersionRef.current);
    setState((prev) => ({ ...prev, ...getCancelled2FAState() }));
    setTempToken(null);
    setError(null);
  }, []);

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }

    let isActive = true;
    const bootstrapVersion = authVersionRef.current;

    const initializeAuth = async () => {
      try {
        // Fetch the current user's identity from the server using the lightweight
        // GET /admin/auth/me endpoint. This validates the session and returns
        // authoritative user data (id, email, role) without heavy queries.
        // A 401/403 response means no valid session exists.
        const serverUser = await adminAuthApi.me();
        if (
          !isActive ||
          !canApplyBootstrapResult(bootstrapVersion, authVersionRef.current)
        ) {
          return;
        }

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
        sessionStorage.setItem(
          AUTH_STATE_KEY,
          JSON.stringify({ user: serverUser, isAuthenticated: true }),
        );
      } catch {
        if (
          !isActive ||
          !canApplyBootstrapResult(bootstrapVersion, authVersionRef.current)
        ) {
          return;
        }

        // Network error or unexpected failure — clear stored state so we don't
        // grant access based on unvalidated cached data.
        sessionStorage.removeItem(AUTH_STATE_KEY);
        setState({ user: null, isAuthenticated: false, isLoading: false, requires2FA: false });
      }
    };

    void initializeAuth();

    return () => {
      isActive = false;
    };
  }, []);

  const login = useCallback(
    async (credentials: LoginRequest) => {
      setError(null);
      setState((prev) => ({ ...prev, isLoading: true }));

      try {
        const tokens = await adminAuthApi.login(credentials);

        if (tokens.requires_2fa) {
          authVersionRef.current = advanceAuthVersion(authVersionRef.current);
          setState((prev) => ({ ...prev, isLoading: false, requires2FA: true }));
          setTempToken(tokens.temp_token || null);
          return;
        }

        await finalizeAuthenticatedSession({
          user: tokens.user ?? null,
          sessionCleanupToken: tokens.session_cleanup_token,
          invalidateSession: adminAuthApi.invalidateSession,
          setAuthenticatedUser,
        });
        router.push("/");
      } catch (err) {
        let message = "Login failed. Please try again.";
        if (err instanceof ApiClientError) {
          if (err.code === "INVALID_CREDENTIALS") {
            message = "Invalid email or password. Please try again.";
          } else {
            message = err.message;
          }
        } else if (err instanceof AdminProfileLoadError) {
          message = err.message;
          clearAuthenticatedState();
          setError(message);
          return;
        } else if (err instanceof AdminSessionStateUnknownError) {
          message = err.message;
          clearAuthenticatedState();
          setError(message);
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

      try {
        const tokens = await adminAuthApi.verify2FA(request);

        const { didApplyAuthenticatedUser } = await finalizeAuthenticatedSession({
          user: tokens.user ?? null,
          sessionCleanupToken: tokens.session_cleanup_token,
          invalidateSession: adminAuthApi.invalidateSession,
          setAuthenticatedUser: (user) => {
            return applyAuthenticatedUserIfCurrent(
              user,
              verificationVersion,
              authVersionRef.current,
              setAuthenticatedUser,
            );
          },
        });
        if (!didApplyAuthenticatedUser) {
          return;
        }
        router.push("/");
      } catch (err) {
        if (err instanceof AdminSessionStateUnknownError) {
          clearAuthenticatedState();
          setError(err.message);
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
        } else if (err instanceof AdminProfileLoadError) {
          message = err.message;
          clearAuthenticatedState();
          setError(message);
          return;
        } else if (err instanceof AdminSessionStateUnknownError) {
          message = err.message;
          clearAuthenticatedState();
          setError(message);
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

  const logout = useCallback(async () => {
    setError(null);
    setState((prev) => ({ ...prev, isLoading: true }));

    try {
      await adminAuthApi.logout();
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
