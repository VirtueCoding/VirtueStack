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
      const { settingsApi } = await import("./api-client");
      const profile = await settingsApi.getProfile();
      updateState({
        user: {
          id: profile.id,
          email: profile.email,
          role: "customer",
        },
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
        const tokens = await customerAuthApi.login(credentials);

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
            role: "customer",
          },
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
        await customerAuthApi.verify2FA(request);
        updateState({
          user: {
            id: "",
            email: "",
            role: "customer",
          },
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