"use client";

import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Loader2, Shield, Server } from "lucide-react";
import { motion, AnimatePresence } from "motion/react";
import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
  Input,
  Label,
} from "@virtuestack/ui";
import { useAuth } from "@/lib/auth-context";

const loginSchema = z.object({
  email: z.string().email("Invalid email address"),
  password: z.string().min(12, "Password must be at least 12 characters"),
});

const totpSchema = z.object({
  totp_code: z
    .string()
    .length(6, "2FA code must be 6 digits")
    .regex(/^\d+$/, "Code must contain only numbers"),
});

type LoginFormData = z.infer<typeof loginSchema>;
type TotpFormData = z.infer<typeof totpSchema>;

export default function LoginPage() {
  const {
    login,
    verify2FA,
    requires2FA,
    tempToken,
    isLoading,
    error,
    clearError,
    reset2FA,
  } = useAuth();

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
    <div className="flex min-h-screen">
      {/* Brand Panel — hidden on mobile */}
      <div className="hidden lg:flex lg:w-1/2 items-center justify-center bg-gradient-to-br from-slate-900 via-slate-800 to-slate-900 p-12">
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.6, ease: [0.25, 0.1, 0.25, 1] }}
          className="max-w-md text-center"
        >
          <div className="mx-auto mb-8 flex h-16 w-16 items-center justify-center rounded-2xl bg-white/10 backdrop-blur-sm">
            <Server className="h-8 w-8 text-white" />
          </div>
          <h1 className="text-4xl font-bold tracking-tight text-white">
            VirtueStack
          </h1>
          <p className="mt-3 text-lg text-slate-300">
            Infrastructure Management
          </p>
          <div className="mt-8 grid grid-cols-3 gap-4">
            {["Nodes", "VMs", "Storage"].map((label) => (
              <div
                key={label}
                className="rounded-xl bg-white/5 px-4 py-3 text-center backdrop-blur-sm"
              >
                <p className="text-sm font-medium text-slate-300">{label}</p>
              </div>
            ))}
          </div>
        </motion.div>
      </div>

      {/* Login Form */}
      <div className="flex flex-1 items-center justify-center p-6 sm:p-12">
        <motion.div
          initial={{ opacity: 0, y: 12 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.4, ease: [0.25, 0.1, 0.25, 1] }}
          className="w-full max-w-md"
        >
          {/* Mobile logo */}
          <div className="mb-8 lg:hidden">
            <h1 className="text-2xl font-bold tracking-tight">VirtueStack</h1>
            <p className="text-sm text-muted-foreground">
              Administration Panel
            </p>
          </div>

          <Card className="border-0 shadow-xl sm:border">
            <CardHeader className="space-y-1">
              <CardTitle className="text-2xl font-bold tracking-tight">
                {requires2FA ? "Two-Factor Authentication" : "Admin Login"}
              </CardTitle>
              <CardDescription>
                {requires2FA
                  ? "Enter your 6-digit authentication code"
                  : "Sign in to your admin account"}
              </CardDescription>
            </CardHeader>

            <AnimatePresence mode="wait" initial={false}>
              {error && (
                <motion.div
                  key="error"
                  initial={{ opacity: 0, height: 0 }}
                  animate={{ opacity: 1, height: "auto" }}
                  exit={{ opacity: 0, height: 0 }}
                  className="mx-6 mb-4 overflow-hidden"
                >
                  <div className="rounded-lg bg-destructive/10 p-3 text-sm text-destructive">
                    {error}
                  </div>
                </motion.div>
              )}
            </AnimatePresence>

            <AnimatePresence mode="wait" initial={false}>
              {requires2FA ? (
                <motion.form
                  key="totp"
                  initial={{ opacity: 0, x: 20 }}
                  animate={{ opacity: 1, x: 0 }}
                  exit={{ opacity: 0, x: -20 }}
                  transition={{ duration: 0.2 }}
                  onSubmit={totpForm.handleSubmit(onTotpSubmit)}
                >
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
                    <Button type="submit" className="w-full" disabled={isLoading}>
                      {isLoading && (
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      )}
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
                </motion.form>
              ) : (
                <motion.form
                  key="login"
                  initial={{ opacity: 0, x: -20 }}
                  animate={{ opacity: 1, x: 0 }}
                  exit={{ opacity: 0, x: 20 }}
                  transition={{ duration: 0.2 }}
                  onSubmit={loginForm.handleSubmit(onLoginSubmit)}
                >
                  <CardContent className="space-y-4">
                    <div className="space-y-2">
                      <Label htmlFor="email">Email</Label>
                      <Input
                        id="email"
                        type="email"
                        placeholder="admin@example.com"
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
                  <CardFooter>
                    <Button type="submit" className="w-full" disabled={isLoading}>
                      {isLoading && (
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      )}
                      {isLoading ? "Signing in..." : "Sign In"}
                    </Button>
                  </CardFooter>
                </motion.form>
              )}
            </AnimatePresence>
          </Card>
        </motion.div>
      </div>
    </div>
  );
}
