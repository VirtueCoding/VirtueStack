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
  type LoginRequest,
  type Verify2FARequest,
} from "./api-client";

interface AdminUser {
  id: string;
  email: string;
  role: string;
}

interface AuthState {
  user: AdminUser | null;
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

const AUTH_STATE_KEY = "admin_auth_state";

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

    try {
      const { adminNodesApi } = await import("./api-client");
      await adminNodesApi.getNodes();
      const stored = sessionStorage.getItem(AUTH_STATE_KEY);
      if (stored) {
        const parsed = JSON.parse(stored);
        if (parsed.user && parsed.isAuthenticated) {
          updateState({
            user: parsed.user,
            isAuthenticated: true,
            isLoading: false,
          });
          return;
        }
      }
      updateState({
        user: { id: "", email: "", role: "admin" },
        isAuthenticated: true,
        isLoading: false,
      });
    } catch {
      setState((prev) => ({ ...prev, isLoading: false }));
    }
  }, [updateState]);

  useEffect(() => {
    initAuth();
  }, [initAuth]);

  const login = useCallback(
    async (credentials: LoginRequest) => {
      setError(null);
      updateState({ isLoading: true });

      try {
        const tokens = await adminAuthApi.login(credentials);

        if (tokens.requires_2fa) {
          updateState({
            isLoading: false,
            requires2FA: true,
            tempToken: tokens.temp_token || null,
          });
          return;
        }

        updateState({
          user: {
            id: "",
            email: credentials.email,
            role: "admin",
          },
          isAuthenticated: true,
          isLoading: false,
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
        await adminAuthApi.verify2FA(request);
        updateState({
          user: {
            id: "",
            email: "",
            role: "admin",
          },
          isAuthenticated: true,
          isLoading: false,
          requires2FA: false,
          tempToken: null,
        });
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
        updateState({ isLoading: false });
      }
    },
    [router, updateState]
  );

  const logout = useCallback(async () => {
    updateState({ isLoading: true });

    try {
      await adminAuthApi.logout();
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