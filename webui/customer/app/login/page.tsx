"use client";

import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Loader2, Shield } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useAuth } from "@/lib/auth-context";

const loginSchema = z.object({
  email: z.string().email("Invalid email address"),
  password: z.string().min(12, "Password must be at least 12 characters"),
});

const totpSchema = z.object({
  totp_code: z.string().length(6, "2FA code must be 6 digits").regex(/^\d+$/, "Code must contain only numbers"),
});

type LoginFormData = z.infer<typeof loginSchema>;
type TotpFormData = z.infer<typeof totpSchema>;

export default function LoginPage() {
  const { login, verify2FA, requires2FA, tempToken, isLoading, error, clearError, reset2FA } = useAuth();

  const loginForm = useForm<LoginFormData>({
    resolver: zodResolver(loginSchema),
  });

  const totpForm = useForm<TotpFormData>({
    resolver: zodResolver(totpSchema),
  });

  const onLoginSubmit = async (data: LoginFormData) => {
    clearError();
    await login(data);
  };

  const onTotpSubmit = async (data: TotpFormData) => {
    clearError();
    if (!tempToken) return;
    await verify2FA({
      temp_token: tempToken,
      totp_code: data.totp_code,
    });
  };

  return (
    <div className="flex min-h-screen items-center justify-center p-4 bg-gradient-to-br from-blue-50 to-indigo-100 dark:from-gray-900 dark:to-blue-950">
      <Card className="w-full max-w-md shadow-xl border-blue-100 dark:border-blue-900">
        <CardHeader className="space-y-1">
          <CardTitle className="text-2xl font-bold tracking-tight text-blue-900 dark:text-blue-100">
            {requires2FA ? "Two-Factor Authentication" : "Customer Login"}
          </CardTitle>
          <CardDescription>
            {requires2FA
              ? "Enter your 6-digit authentication code to continue"
              : "Enter your credentials to access your account"}
          </CardDescription>
        </CardHeader>

        {error && (
          <div className="mx-6 mb-4">
            <div className="rounded-md bg-destructive/15 p-3 text-sm text-destructive">
              {error}
            </div>
          </div>
        )}

        {requires2FA ? (
          <form onSubmit={totpForm.handleSubmit(onTotpSubmit)}>
            <CardContent className="space-y-4">
              <div className="flex items-center justify-center py-4">
                <div className="rounded-full bg-primary/10 p-4">
                  <Shield className="h-8 w-8 text-primary" />
                </div>
              </div>
              <div className="space-y-2">
                <Label htmlFor="totp_code">Authentication Code</Label>
                <Input
                  id="totp_code"
                  type="text"
                  maxLength={6}
                  placeholder="000000"
                  disabled={isLoading}
                  {...totpForm.register("totp_code")}
                  className="text-center text-lg tracking-widest"
                />
                {totpForm.formState.errors.totp_code && (
                  <p className="text-sm text-destructive">
                    {totpForm.formState.errors.totp_code.message}
                  </p>
                )}
              </div>
            </CardContent>
            <CardFooter className="flex flex-col space-y-4">
              <Button
                type="submit"
                className="w-full bg-blue-600 hover:bg-blue-700 dark:bg-blue-700 dark:hover:bg-blue-600"
                disabled={isLoading}
              >
                {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {isLoading ? "Verifying..." : "Verify"}
              </Button>
              <Button
                type="button"
                variant="ghost"
                className="text-sm text-muted-foreground"
                onClick={() => reset2FA()}
              >
                Back to login
              </Button>
            </CardFooter>
          </form>
        ) : (
          <form onSubmit={loginForm.handleSubmit(onLoginSubmit)}>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="email">Email</Label>
                <Input
                  id="email"
                  type="email"
                  placeholder="email@example.com"
                  disabled={isLoading}
                  {...loginForm.register("email")}
                />
                {loginForm.formState.errors.email && (
                  <p className="text-sm text-destructive">
                    {loginForm.formState.errors.email.message}
                  </p>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="password">Password</Label>
                <Input
                  id="password"
                  type="password"
                  placeholder="Enter your password"
                  disabled={isLoading}
                  {...loginForm.register("password")}
                />
                {loginForm.formState.errors.password && (
                  <p className="text-sm text-destructive">
                    {loginForm.formState.errors.password.message}
                  </p>
                )}
              </div>
            </CardContent>
            <CardFooter className="flex flex-col space-y-4">
              <Button
                type="submit"
                className="w-full bg-blue-600 hover:bg-blue-700 dark:bg-blue-700 dark:hover:bg-blue-600"
                disabled={isLoading}
              >
                {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {isLoading ? "Signing in..." : "Sign In"}
              </Button>
            </CardFooter>
          </form>
        )}
      </Card>
    </div>
  );
}
