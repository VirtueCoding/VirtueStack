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
  tokenStorage,
  ApiClientError,
  type AuthTokens,
  type LoginRequest,
  type Verify2FARequest,
} from "./api-client";

interface CustomerUser {
  id: string;
  email: string;
  role: string;
}

interface AuthState {
  user: CustomerUser | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  requires2FA: boolean;
  tempToken: string | null;
}

interface AuthContextType extends AuthState {
  login: (credentials: LoginRequest) => Promise<void>;
  verify2FA: (request: Verify2FARequest) => Promise<void>;
  logout: () => Promise<void>;
  clearError: () => void;
  error: string | null;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

const AUTH_STATE_KEY = "customer_auth_state";
const ACCESS_TOKEN_KEY = "customer_access_token";
const REFRESH_TOKEN_KEY = "customer_refresh_token";

function parseJwt(token: string): { sub: string; email?: string; role?: string } | null {
  try {
    const base64Url = token.split(".")[1];
    const base64 = base64Url.replace(/-/g, "+").replace(/_/g, "/");
    const jsonPayload = decodeURIComponent(
      atob(base64)
        .split("")
        .map((c) => "%" + ("00" + c.charCodeAt(0).toString(16)).slice(-2))
        .join("")
    );
    return JSON.parse(jsonPayload);
  } catch {
    return null;
  }
}

function buildUserFromToken(token: string): CustomerUser | null {
  const payload = parseJwt(token);
  if (!payload) return null;
  return {
    id: payload.sub,
    email: payload.email || "",
    role: payload.role || "customer",
  };
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const router = useRouter();
  const [state, setState] = useState<AuthState>({
    user: null,
    isAuthenticated: false,
    isLoading: true,
    requires2FA: false,
    tempToken: null,
  });
  const [error, setError] = useState<string | null>(null);

  const updateState = useCallback((updates: Partial<AuthState>) => {
    setState((prev) => {
      const next = { ...prev, ...updates };
      if (typeof window !== "undefined") {
        sessionStorage.setItem(
          AUTH_STATE_KEY,
          JSON.stringify({
            user: next.user,
            isAuthenticated: next.isAuthenticated,
            requires2FA: next.requires2FA,
            tempToken: next.tempToken,
          })
        );
      }
      return next;
    });
  }, []);

  const clearError = useCallback(() => setError(null), []);

  const initAuth = useCallback(async () => {
    if (typeof window === "undefined") return;

    const accessToken = tokenStorage.getAccessToken();
    const refreshToken = tokenStorage.getRefreshToken();

    if (!accessToken || !refreshToken) {
      setState((prev) => ({ ...prev, isLoading: false }));
      return;
    }

    if (tokenStorage.isTokenExpired()) {
      try {
        const tokens = await customerAuthApi.refreshToken(refreshToken);
        const user = buildUserFromToken(tokens.access_token);
        updateState({
          user,
          isAuthenticated: true,
          isLoading: false,
        });
      } catch {
        tokenStorage.clearTokens();
        setState({
          user: null,
          isAuthenticated: false,
          isLoading: false,
          requires2FA: false,
          tempToken: null,
        });
      }
    } else {
      const user = buildUserFromToken(accessToken);
      updateState({
        user,
        isAuthenticated: true,
        isLoading: false,
      });
    }
  }, [updateState]);

  useEffect(() => {
    initAuth();
  }, [initAuth]);

  useEffect(() => {
    const handleStorage = (e: StorageEvent) => {
      if (e.key === ACCESS_TOKEN_KEY || e.key === REFRESH_TOKEN_KEY) {
        if (!e.newValue) {
          setState({
            user: null,
            isAuthenticated: false,
            isLoading: false,
            requires2FA: false,
            tempToken: null,
          });
        }
      }
    };

    window.addEventListener("storage", handleStorage);
    return () => window.removeEventListener("storage", handleStorage);
  }, []);

  const login = useCallback(
    async (credentials: LoginRequest) => {
      setError(null);
      updateState({ isLoading: true });

      try {
        const tokens = await customerAuthApi.login(credentials);

        if (tokens.requires_2fa) {
          updateState({
            isLoading: false,
            requires2FA: true,
            tempToken: tokens.temp_token || null,
          });
          return;
        }

        tokenStorage.setTokens(tokens);
        const user = buildUserFromToken(tokens.access_token);
        updateState({
          user,
          isAuthenticated: true,
          isLoading: false,
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
        }
        setError(message);
        updateState({ isLoading: false });
      }
    },
    [router, updateState]
  );

  const verify2FA = useCallback(
    async (request: Verify2FARequest) => {
      setError(null);
      updateState({ isLoading: true });

      try {
        const tokens = await customerAuthApi.verify2FA(request);
        tokenStorage.setTokens(tokens);
        const user = buildUserFromToken(tokens.access_token);
        updateState({
          user,
          isAuthenticated: true,
          isLoading: false,
          requires2FA: false,
          tempToken: null,
        });
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
        updateState({ isLoading: false });
      }
    },
    [router, updateState]
  );

  const logout = useCallback(async () => {
    updateState({ isLoading: true });

    try {
      await customerAuthApi.logout();
    } finally {
      setState({
        user: null,
        isAuthenticated: false,
        isLoading: false,
        requires2FA: false,
        tempToken: null,
      });
      sessionStorage.removeItem(AUTH_STATE_KEY);
      router.push("/login");
    }
  }, [router, updateState]);

  const value: AuthContextType = {
    ...state,
    login,
    verify2FA,
    logout,
    error,
    clearError,
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
