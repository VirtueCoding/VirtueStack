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
  applyRevalidationResultIfCurrent,
  canApplyBootstrapResult,
  getAuthSyncAction,
  getCancelled2FAState,
  getProfileBootstrapErrorState,
  shouldPublishSessionInvalidated,
  shouldRevalidateSession,
} from "./auth-bootstrap";

interface AuthState {
  user: AdminUser | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  requires2FA: boolean;
  hasBootstrapError: boolean;
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
const AUTH_SYNC_KEY = "virtuestack_admin_auth_sync";

export function AuthProvider({ children }: { children: ReactNode }) {
  const router = useRouter();
  const [state, setState] = useState<AuthState>({
    user: null,
    isAuthenticated: false,
    isLoading: true,
    requires2FA: false,
    hasBootstrapError: false,
  });
  const [tempToken, setTempToken] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const authVersionRef = useRef(0);
  const lastRevalidatedAtRef = useRef(0);
  const revalidationRequestIdRef = useRef(0);
  const revalidationInFlightRef = useRef(false);

  const clearError = useCallback(() => setError(null), []);

  const applyVerifiedSession = useCallback((user: AdminUser | null) => {
    setState({
      user,
      isAuthenticated: user !== null,
      isLoading: false,
      requires2FA: false,
      hasBootstrapError: false,
    });
    setTempToken(null);
    setError(null);
  }, []);

  const publishAuthSyncEvent = useCallback(
    (type: "logout" | "session-invalidated") => {
      if (typeof window === "undefined") {
        return;
      }

      try {
        window.localStorage.setItem(
          AUTH_SYNC_KEY,
          JSON.stringify({ type, at: Date.now() }),
        );
      } catch {
        // Cross-tab sync is best-effort; the current tab state is already updated.
      }
    },
    [],
  );

  const setAuthenticatedUser = useCallback(
    (user: AdminUser) => {
      authVersionRef.current = advanceAuthVersion(authVersionRef.current);
      lastRevalidatedAtRef.current = Date.now();
      applyVerifiedSession(user);
    },
    [applyVerifiedSession]
  );

  const clearAuthenticatedState = useCallback(() => {
    authVersionRef.current = advanceAuthVersion(authVersionRef.current);
    lastRevalidatedAtRef.current = Date.now();
    setState({
      user: null,
      isAuthenticated: false,
      isLoading: false,
      requires2FA: false,
      hasBootstrapError: false,
    });
    setTempToken(null);
    setError(null);
  }, []);

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

        lastRevalidatedAtRef.current = Date.now();
        if (!serverUser) {
          applyVerifiedSession(null);
          return;
        }

        applyVerifiedSession(serverUser);
      } catch {
        if (
          !isActive ||
          !canApplyBootstrapResult(bootstrapVersion, authVersionRef.current)
        ) {
          return;
        }

        lastRevalidatedAtRef.current = Date.now();
        // Network error or unexpected failure — clear stored state so we don't
        // grant access based on unvalidated cached data.
        setError("Unable to verify your session right now. Please try again.");
        setState(getProfileBootstrapErrorState());
      }
    };

    void initializeAuth();

    return () => {
      isActive = false;
    };
  }, [applyVerifiedSession]);

  const revalidateSession = useCallback(async ({ force = false }: { force?: boolean } = {}) => {
    if (typeof window === "undefined") {
      return;
    }

    const nowMs = Date.now();
    if (revalidationInFlightRef.current) {
      return;
    }
    if (
      !shouldRevalidateSession({
        isAuthenticated: state.isAuthenticated,
        isLoading: state.isLoading,
        requires2FA: state.requires2FA,
        hasBootstrapError: state.hasBootstrapError,
        lastRevalidatedAtMs: lastRevalidatedAtRef.current,
        nowMs,
        force,
      })
    ) {
      return;
    }

    lastRevalidatedAtRef.current = nowMs;
    revalidationInFlightRef.current = true;
    const revalidationVersion = authVersionRef.current;
    const revalidationRequestId = revalidationRequestIdRef.current + 1;
    revalidationRequestIdRef.current = revalidationRequestId;
    const wasAuthenticated = state.isAuthenticated;

    try {
      const serverUser = await adminAuthApi.me();
      if (!canApplyBootstrapResult(revalidationVersion, authVersionRef.current)) {
        return;
      }

      applyRevalidationResultIfCurrent(
        serverUser,
        revalidationRequestId,
        revalidationRequestIdRef.current,
        (verifiedUser) => {
          lastRevalidatedAtRef.current = Date.now();
          if (!verifiedUser) {
            clearAuthenticatedState();
            if (shouldPublishSessionInvalidated({ isAuthenticated: wasAuthenticated })) {
              publishAuthSyncEvent("session-invalidated");
            }
            router.replace("/login");
            return;
          }

          applyVerifiedSession(verifiedUser);
        },
      );
    } catch {
      if (!canApplyBootstrapResult(revalidationVersion, authVersionRef.current)) {
        return;
      }

      applyRevalidationResultIfCurrent(
        true,
        revalidationRequestId,
        revalidationRequestIdRef.current,
        () => {
          lastRevalidatedAtRef.current = Date.now();
          setError("Unable to verify your session right now. Please try again.");
          setState(getProfileBootstrapErrorState());
        },
      );
    } finally {
      if (revalidationRequestIdRef.current === revalidationRequestId) {
        revalidationInFlightRef.current = false;
      }
    }
  }, [
    applyVerifiedSession,
    clearAuthenticatedState,
    publishAuthSyncEvent,
    router,
    state.hasBootstrapError,
    state.isAuthenticated,
    state.isLoading,
    state.requires2FA,
  ]);

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }

    const handleStorage = (event: StorageEvent) => {
      if (event.key !== AUTH_SYNC_KEY) {
        return;
      }

      if (getAuthSyncAction(event.newValue) !== "clear-auth") {
        return;
      }

      clearAuthenticatedState();
      router.replace("/login");
    };

    const handleWindowFocus = () => {
      void revalidateSession({ force: true });
    };

    const handleVisibilityChange = () => {
      if (document.visibilityState !== "visible") {
        return;
      }
      void revalidateSession({ force: true });
    };

    window.addEventListener("storage", handleStorage);
    window.addEventListener("focus", handleWindowFocus);
    document.addEventListener("visibilitychange", handleVisibilityChange);

    return () => {
      window.removeEventListener("storage", handleStorage);
      window.removeEventListener("focus", handleWindowFocus);
      document.removeEventListener("visibilitychange", handleVisibilityChange);
    };
  }, [clearAuthenticatedState, revalidateSession, router]);

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
      publishAuthSyncEvent("logout");
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
  }, [clearAuthenticatedState, publishAuthSyncEvent, router]);

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
