"use client";

import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@virtuestack/ui";
import { Button } from "@virtuestack/ui";
import { Input } from "@virtuestack/ui";
import { Label } from "@virtuestack/ui";
import { Switch } from "@virtuestack/ui";
import { Badge } from "@virtuestack/ui";
import { useToast } from "@virtuestack/ui";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@virtuestack/ui";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { settingsApi } from "@/lib/api-client";
import { useMutationToast } from "@/lib/utils/toast-helpers";
import {
  Smartphone,
  Lock,
  Loader2,
  Eye,
  EyeOff,
  Check,
  Download,
  RefreshCw,
  QrCode,
} from "lucide-react";

const passwordSchema = z.object({
  currentPassword: z.string().min(1, "Current password is required"),
  newPassword: z.string().min(12, "Password must be at least 12 characters"),
  confirmPassword: z.string().min(1, "Please confirm your password"),
}).refine((data) => data.newPassword === data.confirmPassword, {
  message: "Passwords do not match",
  path: ["confirmPassword"],
});

const totpSchema = z.object({
  code: z.string().length(6, "Code must be 6 digits").regex(/^\d+$/, "Code must contain only numbers"),
});

const disable2FASchema = z.object({
  password: z.string().min(12, "Password must be at least 12 characters"),
});

type PasswordFormData = z.infer<typeof passwordSchema>;
type TOTPFormData = z.infer<typeof totpSchema>;
type Disable2FAFormData = z.infer<typeof disable2FASchema>;

interface SecurityTabProps {
  twoFactorStatus: { enabled: boolean } | null | undefined;
  backupCodesData: { backup_codes: string[] } | null | undefined;
  isLoading: boolean;
}

export function SecurityTab({ twoFactorStatus, backupCodesData, isLoading }: SecurityTabProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const { createMutationOnError } = useMutationToast();
  const [showPassword, setShowPassword] = useState(false);
  const [qrDialogOpen, setQrDialogOpen] = useState(false);
  const [backupCodesDialogOpen, setBackupCodesDialogOpen] = useState(false);
  const [disable2FADialogOpen, setDisable2FADialogOpen] = useState(false);
  const [showDisablePassword, setShowDisablePassword] = useState(false);
  const [qrCodeUrl, setQrCodeUrl] = useState<string>("");
  const [totpSecret, setTotpSecret] = useState<string>("");
  const [backupCodes, setBackupCodes] = useState<string[]>([]);

  const twoFactorEnabled = twoFactorStatus?.enabled || false;

  const passwordForm = useForm<PasswordFormData>({
    resolver: zodResolver(passwordSchema),
    defaultValues: {
      currentPassword: "",
      newPassword: "",
      confirmPassword: "",
    },
  });

  const totpForm = useForm<TOTPFormData>({
    resolver: zodResolver(totpSchema),
    defaultValues: {
      code: "",
    },
  });

  const disable2FAForm = useForm<Disable2FAFormData>({
    resolver: zodResolver(disable2FASchema),
    defaultValues: {
      password: "",
    },
  });

  const updatePasswordMutation = useMutation({
    mutationFn: settingsApi.updatePassword,
    onSuccess: () => {
      passwordForm.reset();
      toast({
        title: "Success",
        description: "Password updated successfully",
      });
    },
    onError: createMutationOnError("Failed to update password"),
  });

  const initiate2FAMutation = useMutation({
    mutationFn: settingsApi.initiate2FA,
    onSuccess: (data) => {
      setQrCodeUrl(data.qr_code_url);
      setTotpSecret(data.secret);
      setQrDialogOpen(true);
    },
    onError: createMutationOnError("Failed to initiate 2FA setup"),
  });

  const enable2FAMutation = useMutation({
    mutationFn: settingsApi.enable2FA,
    onSuccess: (data) => {
      setBackupCodes(data.backup_codes);
      setQrDialogOpen(false);
      queryClient.invalidateQueries({ queryKey: ["2fa-status"] });
      queryClient.invalidateQueries({ queryKey: ["backup-codes"] });
      setBackupCodesDialogOpen(true);
      totpForm.reset();
      toast({
        title: "Success",
        description: "2FA enabled successfully. Save your backup codes!",
      });
    },
    onError: createMutationOnError("Failed to enable 2FA. Check your code and try again."),
  });

  const disable2FAMutation = useMutation({
    mutationFn: settingsApi.disable2FA,
    onSuccess: () => {
      setDisable2FADialogOpen(false);
      disable2FAForm.reset();
      setShowDisablePassword(false);
      queryClient.invalidateQueries({ queryKey: ["2fa-status"] });
      toast({
        title: "Success",
        description: "2FA disabled successfully",
      });
    },
    onError: (error: unknown) => {
      const apiError = error as { code?: string; message?: string };
      if (apiError?.code === "INVALID_PASSWORD") {
        disable2FAForm.setError("password", {
          type: "manual",
          message: "Incorrect password. Please try again.",
        });
        return;
      }
      createMutationOnError("Failed to disable 2FA")(error);
    },
  });

  const regenerateBackupCodesMutation = useMutation({
    mutationFn: settingsApi.regenerateBackupCodes,
    onSuccess: (data) => {
      setBackupCodes(data.backup_codes);
      queryClient.invalidateQueries({ queryKey: ["backup-codes"] });
      setBackupCodesDialogOpen(true);
      toast({
        title: "Success",
        description: "Backup codes regenerated successfully",
      });
    },
    onError: createMutationOnError("Failed to regenerate backup codes"),
  });

  const handlePasswordSubmit = (data: PasswordFormData) => {
    updatePasswordMutation.mutate({
      current_password: data.currentPassword,
      new_password: data.newPassword,
    });
  };

  const handle2FAToggle = (enabled: boolean) => {
    if (enabled) {
      initiate2FAMutation.mutate();
    } else {
      disable2FAForm.reset();
      setShowDisablePassword(false);
      setDisable2FADialogOpen(true);
    }
  };

  const handleDisable2FASubmit = (data: Disable2FAFormData) => {
    disable2FAMutation.mutate({ password: data.password });
  };

  const handleVerifyTOTP = (data: TOTPFormData) => {
    enable2FAMutation.mutate({ totp_code: data.code });
  };

  const downloadBackupCodes = () => {
    const codes = backupCodes.length > 0 ? backupCodes : backupCodesData?.backup_codes || [];
    const content = codes.join("\n");
    const blob = new Blob([content], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "backup-codes.txt";
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
    toast({
      title: "Downloaded",
      description: "Backup codes downloaded successfully",
    });
  };

  if (isLoading) {
    return (
      <div className="flex justify-center p-8">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Lock className="h-5 w-5" />
            Change Password
          </CardTitle>
          <CardDescription>
            Update your password to keep your account secure
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={passwordForm.handleSubmit(handlePasswordSubmit)} className="space-y-4">
            <div className="grid gap-2">
              <Label htmlFor="current-password">Current Password</Label>
              <Input
                id="current-password"
                type={showPassword ? "text" : "password"}
                {...passwordForm.register("currentPassword")}
                className="max-w-md"
              />
              {passwordForm.formState.errors.currentPassword && (
                <p className="text-sm text-destructive">{passwordForm.formState.errors.currentPassword.message}</p>
              )}
            </div>
            <div className="grid gap-2">
              <Label htmlFor="new-password">New Password</Label>
              <Input
                id="new-password"
                type={showPassword ? "text" : "password"}
                {...passwordForm.register("newPassword")}
                className="max-w-md"
              />
              {passwordForm.formState.errors.newPassword && (
                <p className="text-sm text-destructive">{passwordForm.formState.errors.newPassword.message}</p>
              )}
            </div>
            <div className="grid gap-2">
              <Label htmlFor="confirm-password">Confirm New Password</Label>
              <div className="relative max-w-md">
                <Input
                  id="confirm-password"
                  type={showPassword ? "text" : "password"}
                  {...passwordForm.register("confirmPassword")}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="absolute right-0 top-0 h-full px-3"
                  onClick={() => setShowPassword(!showPassword)}
                >
                  {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </Button>
              </div>
              {passwordForm.formState.errors.confirmPassword && (
                <p className="text-sm text-destructive">{passwordForm.formState.errors.confirmPassword.message}</p>
              )}
            </div>
            <Button
              type="submit"
              variant="secondary"
              className="mt-2"
              disabled={updatePasswordMutation.isPending}
            >
              {updatePasswordMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Update Password
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Smartphone className="h-5 w-5" />
            Two-Factor Authentication
          </CardTitle>
          <CardDescription>
            Add an extra layer of security to your account
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between rounded-lg border p-4">
            <div className="space-y-0.5">
              <Label className="text-base">Enable 2FA</Label>
              <p className="text-sm text-muted-foreground">
                Require a verification code when signing in
              </p>
            </div>
            <Switch
              checked={twoFactorEnabled}
              onCheckedChange={handle2FAToggle}
              disabled={isLoading || initiate2FAMutation.isPending || disable2FAMutation.isPending}
            />
          </div>

          {!twoFactorEnabled && (
            <div className="rounded-lg border bg-muted p-4">
              <p className="text-sm text-muted-foreground text-center">
                Enable 2FA above to see the QR code setup
              </p>
            </div>
          )}

          {twoFactorEnabled && (
            <div className="rounded-lg border bg-muted p-4 space-y-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Check className="h-5 w-5 text-green-500" />
                  <span className="font-medium">2FA is enabled</span>
                </div>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setBackupCodesDialogOpen(true)}
                >
                  <Eye className="mr-2 h-4 w-4" />
                  View Backup Codes
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      {twoFactorEnabled && (
        <Card>
          <CardHeader>
            <CardTitle>Backup Codes</CardTitle>
            <CardDescription>
              Use these codes to recover your account if you lose access to your authenticator app
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="flex gap-2">
              <Button variant="outline" onClick={() => setBackupCodesDialogOpen(true)}>
                <Eye className="mr-2 h-4 w-4" />
                View Codes
              </Button>
              <Button variant="outline" onClick={() => regenerateBackupCodesMutation.mutate()}>
                <RefreshCw className="mr-2 h-4 w-4" />
                Regenerate
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {/* QR Code Dialog */}
      <Dialog open={qrDialogOpen} onOpenChange={setQrDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Setup Two-Factor Authentication</DialogTitle>
            <DialogDescription>
              Scan the QR code with your authenticator app (Google Authenticator, Authy, etc.)
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col items-center space-y-4 py-4">
            {qrCodeUrl && qrCodeUrl.startsWith("data:image/") ? (
              <img src={qrCodeUrl} alt="2FA QR Code" className="rounded-lg border" />
            ) : (
              <div className="h-48 w-48 bg-muted rounded-lg flex items-center justify-center">
                <QrCode className="h-24 w-24 text-muted-foreground" />
              </div>
            )}
            <div className="text-center">
              <p className="text-sm text-muted-foreground">Secret:</p>
              <code className="text-sm bg-muted px-2 py-1 rounded">{totpSecret}</code>
            </div>
            <form onSubmit={totpForm.handleSubmit(handleVerifyTOTP)} className="w-full space-y-4">
              <div className="space-y-2">
                <Label htmlFor="totp-code">Enter 6-digit code</Label>
                <Input
                  id="totp-code"
                  placeholder="000000"
                  maxLength={6}
                  {...totpForm.register("code")}
                />
                {totpForm.formState.errors.code && (
                  <p className="text-sm text-destructive">{totpForm.formState.errors.code.message}</p>
                )}
              </div>
              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => setQrDialogOpen(false)}>
                  Cancel
                </Button>
                <Button type="submit" disabled={enable2FAMutation.isPending}>
                  {enable2FAMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                  Verify & Enable
                </Button>
              </DialogFooter>
            </form>
          </div>
        </DialogContent>
      </Dialog>

      {/* Backup Codes Dialog */}
      <Dialog open={backupCodesDialogOpen} onOpenChange={setBackupCodesDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Backup Codes</DialogTitle>
            <DialogDescription>
              Save these codes in a secure place. Each code can only be used once.
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            <div className="grid grid-cols-2 gap-2 rounded-lg border bg-muted p-4">
              {(backupCodes.length > 0 ? backupCodes : backupCodesData?.backup_codes || []).map((code) => (
                <code key={code} className="text-center font-mono text-sm py-1">
                  {code}
                </code>
              ))}
            </div>
            <p className="text-sm text-muted-foreground mt-4 text-center">
              Download these codes and store them securely.
            </p>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setBackupCodesDialogOpen(false)}>
              Close
            </Button>
            <Button type="button" onClick={downloadBackupCodes}>
              <Download className="mr-2 h-4 w-4" />
              Download
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Disable 2FA Confirmation Dialog */}
      <Dialog
        open={disable2FADialogOpen}
        onOpenChange={(open) => {
          if (!open) {
            disable2FAForm.reset();
            setShowDisablePassword(false);
          }
          setDisable2FADialogOpen(open);
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Disable Two-Factor Authentication</DialogTitle>
            <DialogDescription>
              Enter your current password to confirm disabling 2FA. This will remove the extra layer of
              security from your account.
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={disable2FAForm.handleSubmit(handleDisable2FASubmit)} className="space-y-4 py-2">
            <div className="grid gap-2">
              <Label htmlFor="disable-2fa-password">Current Password</Label>
              <div className="relative">
                <Input
                  id="disable-2fa-password"
                  type={showDisablePassword ? "text" : "password"}
                  placeholder="Enter your password"
                  autoComplete="current-password"
                  {...disable2FAForm.register("password")}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="absolute right-0 top-0 h-full px-3"
                  onClick={() => setShowDisablePassword(!showDisablePassword)}
                >
                  {showDisablePassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </Button>
              </div>
              {disable2FAForm.formState.errors.password && (
                <p className="text-sm text-destructive">
                  {disable2FAForm.formState.errors.password.message}
                </p>
              )}
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setDisable2FADialogOpen(false)}
                disabled={disable2FAMutation.isPending}
              >
                Cancel
              </Button>
              <Button type="submit" variant="destructive" disabled={disable2FAMutation.isPending}>
                {disable2FAMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Disable 2FA
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  );
}