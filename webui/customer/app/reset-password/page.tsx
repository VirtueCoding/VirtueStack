"use client";

import { useState, Suspense } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Loader2, KeyRound, ArrowLeft, CheckCircle2 } from "lucide-react";

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
import { customerAuthApi, ApiClientError } from "@/lib/api-client";

const resetPasswordSchema = z.object({
  new_password: z.string().min(12, "Password must be at least 12 characters"),
  confirm_password: z.string().min(12, "Password must be at least 12 characters"),
}).refine((data) => data.new_password === data.confirm_password, {
  message: "Passwords do not match",
  path: ["confirm_password"],
});

type ResetPasswordFormData = z.infer<typeof resetPasswordSchema>;

function ResetPasswordForm() {
  const searchParams = useSearchParams();
  const token = searchParams.get("token");

  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isSuccess, setIsSuccess] = useState(false);

  const form = useForm<ResetPasswordFormData>({
    resolver: zodResolver(resetPasswordSchema),
  });

  const onSubmit = async (data: ResetPasswordFormData) => {
    if (!token) {
      setError("Invalid reset link. Please request a new password reset.");
      return;
    }

    setError(null);
    setIsLoading(true);
    try {
      await customerAuthApi.resetPassword(token, data.new_password);
      setIsSuccess(true);
    } catch (err) {
      if (err instanceof ApiClientError) {
        if (err.code === "INVALID_RESET_TOKEN") {
          setError("This reset link is invalid or has expired. Please request a new one.");
        } else if (err.code === "RATE_LIMIT_EXCEEDED") {
          setError("Too many attempts. Please try again later.");
        } else {
          setError(err.message);
        }
      } else {
        setError("An unexpected error occurred. Please try again.");
      }
    } finally {
      setIsLoading(false);
    }
  };

  if (!token) {
    return (
      <Card className="w-full max-w-md shadow-xl border-blue-100 dark:border-blue-900">
        <CardHeader className="space-y-1">
          <CardTitle className="text-2xl font-bold tracking-tight text-blue-900 dark:text-blue-100">
            Invalid Reset Link
          </CardTitle>
          <CardDescription>
            This password reset link is invalid or missing.
          </CardDescription>
        </CardHeader>
        <CardFooter className="flex flex-col space-y-4">
          <Link href="/forgot-password" className="w-full">
            <Button className="w-full bg-blue-600 hover:bg-blue-700 dark:bg-blue-700 dark:hover:bg-blue-600">
              Request New Reset Link
            </Button>
          </Link>
          <Link
            href="/login"
            className="text-sm text-muted-foreground hover:text-primary transition-colors"
          >
            <span className="flex items-center justify-center">
              <ArrowLeft className="mr-1 h-3 w-3" />
              Back to Login
            </span>
          </Link>
        </CardFooter>
      </Card>
    );
  }

  if (isSuccess) {
    return (
      <Card className="w-full max-w-md shadow-xl border-blue-100 dark:border-blue-900">
        <CardHeader className="space-y-1">
          <CardTitle className="text-2xl font-bold tracking-tight text-blue-900 dark:text-blue-100">
            Password Reset Successful
          </CardTitle>
          <CardDescription>
            Your password has been updated
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-center py-4">
            <div className="rounded-full bg-green-100 dark:bg-green-900/30 p-4">
              <CheckCircle2 className="h-8 w-8 text-green-600 dark:text-green-400" />
            </div>
          </div>
          <p className="text-center text-sm text-muted-foreground">
            Your password has been reset successfully. You can now log in with your new password.
          </p>
        </CardContent>
        <CardFooter>
          <Link href="/login" className="w-full">
            <Button className="w-full bg-blue-600 hover:bg-blue-700 dark:bg-blue-700 dark:hover:bg-blue-600">
              Go to Login
            </Button>
          </Link>
        </CardFooter>
      </Card>
    );
  }

  return (
    <Card className="w-full max-w-md shadow-xl border-blue-100 dark:border-blue-900">
      <CardHeader className="space-y-1">
        <CardTitle className="text-2xl font-bold tracking-tight text-blue-900 dark:text-blue-100">
          Reset Password
        </CardTitle>
        <CardDescription>
          Enter your new password below
        </CardDescription>
      </CardHeader>

      {error && (
        <div className="mx-6 mb-4">
          <div className="rounded-md bg-destructive/15 p-3 text-sm text-destructive">
            {error}
          </div>
        </div>
      )}

      <form onSubmit={form.handleSubmit(onSubmit)}>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-center py-2">
            <div className="rounded-full bg-primary/10 p-4">
              <KeyRound className="h-8 w-8 text-primary" />
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="new_password">New Password</Label>
            <Input
              id="new_password"
              type="password"
              placeholder="Enter new password (min. 12 characters)"
              disabled={isLoading}
              {...form.register("new_password")}
            />
            {form.formState.errors.new_password && (
              <p className="text-sm text-destructive">
                {form.formState.errors.new_password.message}
              </p>
            )}
          </div>
          <div className="space-y-2">
            <Label htmlFor="confirm_password">Confirm Password</Label>
            <Input
              id="confirm_password"
              type="password"
              placeholder="Confirm new password"
              disabled={isLoading}
              {...form.register("confirm_password")}
            />
            {form.formState.errors.confirm_password && (
              <p className="text-sm text-destructive">
                {form.formState.errors.confirm_password.message}
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
            {isLoading ? "Resetting..." : "Reset Password"}
          </Button>
          <Link
            href="/login"
            className="text-sm text-muted-foreground hover:text-primary transition-colors"
          >
            <span className="flex items-center justify-center">
              <ArrowLeft className="mr-1 h-3 w-3" />
              Back to Login
            </span>
          </Link>
        </CardFooter>
      </form>
    </Card>
  );
}

export default function ResetPasswordPage() {
  return (
    <div className="flex min-h-screen items-center justify-center p-4 bg-gradient-to-br from-blue-50 to-indigo-100 dark:from-gray-900 dark:to-blue-950">
      <Suspense fallback={
        <Card className="w-full max-w-md shadow-xl border-blue-100 dark:border-blue-900">
          <CardContent className="flex items-center justify-center py-12">
            <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
          </CardContent>
        </Card>
      }>
        <ResetPasswordForm />
      </Suspense>
    </div>
  );
}
