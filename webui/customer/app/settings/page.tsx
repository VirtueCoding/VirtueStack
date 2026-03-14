"use client";

import { useState, useEffect } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Badge } from "@/components/ui/badge";
import { useToast } from "@/components/ui/use-toast";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { settingsApi, ApiKey, Webhook as WebhookType, ApiClientError } from "@/lib/api-client";
import {
  User,
  Shield,
  Key,
  Webhook,
  Copy,
  Check,
  RefreshCw,
  Play,
  QrCode,
  Mail,
  Lock,
  Smartphone,
  Calendar,
  Loader2,
  Trash2,
  Edit3,
  Plus,
  Eye,
  EyeOff,
  Download,
} from "lucide-react";
import { useAuth } from "@/lib/auth-context";

const profileSchema = z.object({
  name: z.string().min(2, "Name must be at least 2 characters"),
  email: z.string().email("Invalid email address"),
  phone: z.string().optional(),
});

const passwordSchema = z.object({
  currentPassword: z.string().min(1, "Current password is required"),
  newPassword: z.string().min(8, "Password must be at least 8 characters"),
  confirmPassword: z.string().min(1, "Please confirm your password"),
}).refine((data) => data.newPassword === data.confirmPassword, {
  message: "Passwords do not match",
  path: ["confirmPassword"],
});

const apiKeySchema = z.object({
  name: z.string().min(1, "Name is required"),
  permissions: z.array(z.string()).min(1, "At least one permission is required"),
});

const webhookSchema = z.object({
  url: z.string().url("Invalid URL"),
  events: z.array(z.string()).min(1, "At least one event is required"),
  secret: z.string().min(1, "Secret is required"),
});

const totpSchema = z.object({
  code: z.string().length(6, "Code must be 6 digits").regex(/^\d+$/, "Code must contain only numbers"),
});

type ProfileFormData = z.infer<typeof profileSchema>;
type PasswordFormData = z.infer<typeof passwordSchema>;
type ApiKeyFormData = z.infer<typeof apiKeySchema>;
type WebhookFormData = z.infer<typeof webhookSchema>;
type TOTPFormData = z.infer<typeof totpSchema>;

const AVAILABLE_EVENTS = [
  "vm.created",
  "vm.started",
  "vm.stopped",
  "vm.deleted",
  "backup.completed",
  "backup.failed",
  "snapshot.created",
  "bandwidth.threshold",
];

const AVAILABLE_PERMISSIONS = [
  "vms:read",
  "vms:write",
  "backups:read",
  "backups:write",
  "snapshots:read",
  "snapshots:write",
];

export default function SettingsPage() {
  const { user } = useAuth();
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [copiedId, setCopiedId] = useState<string | null>(null);
  
  const [apiKeyDialogOpen, setApiKeyDialogOpen] = useState(false);
  const [webhookDialogOpen, setWebhookDialogOpen] = useState(false);
  const [editingWebhook, setEditingWebhook] = useState<WebhookType | null>(null);
  const [deleteKeyDialogOpen, setDeleteKeyDialogOpen] = useState(false);
  const [rotateKeyDialogOpen, setRotateKeyDialogOpen] = useState(false);
  const [deleteWebhookDialogOpen, setDeleteWebhookDialogOpen] = useState(false);
  const [selectedKeyId, setSelectedKeyId] = useState<string | null>(null);
  const [selectedWebhookId, setSelectedWebhookId] = useState<string | null>(null);
  const [showPassword, setShowPassword] = useState(false);
  const [backupCodesDialogOpen, setBackupCodesDialogOpen] = useState(false);
  const [qrDialogOpen, setQrDialogOpen] = useState(false);
  const [qrCodeUrl, setQrCodeUrl] = useState<string>("");
  const [totpSecret, setTotpSecret] = useState<string>("");
  const [backupCodes, setBackupCodes] = useState<string[]>([]);

  const { data: profile, isLoading: profileLoading } = useQuery({
    queryKey: ["profile"],
    queryFn: () => settingsApi.getProfile(),
  });

  const { data: apiKeys, isLoading: apiKeysLoading } = useQuery({
    queryKey: ["api-keys"],
    queryFn: () => settingsApi.getApiKeys(),
  });

  const { data: webhooks, isLoading: webhooksLoading } = useQuery({
    queryKey: ["webhooks"],
    queryFn: () => settingsApi.getWebhooks(),
  });

  const { data: twoFactorStatus, isLoading: twoFactorLoading } = useQuery({
    queryKey: ["2fa-status"],
    queryFn: async () => {
      try {
        await settingsApi.getBackupCodes();
        return { enabled: true };
      } catch (error) {
        console.error("Failed to get 2FA status:", error);
        return { enabled: false };
      }
    },
  });

  const { data: backupCodesData, refetch: refetchBackupCodes } = useQuery({
    queryKey: ["backup-codes"],
    queryFn: () => settingsApi.getBackupCodes(),
    enabled: twoFactorStatus?.enabled || false,
  });

  const profileForm = useForm<ProfileFormData>({
    resolver: zodResolver(profileSchema),
    defaultValues: {
      name: "",
      email: "",
      phone: "",
    },
  });

  const passwordForm = useForm<PasswordFormData>({
    resolver: zodResolver(passwordSchema),
    defaultValues: {
      currentPassword: "",
      newPassword: "",
      confirmPassword: "",
    },
  });

  const apiKeyForm = useForm<ApiKeyFormData>({
    resolver: zodResolver(apiKeySchema),
    defaultValues: {
      name: "",
      permissions: [],
    },
  });

  const webhookForm = useForm<WebhookFormData>({
    resolver: zodResolver(webhookSchema),
    defaultValues: {
      url: "",
      events: [],
      secret: "",
    },
  });

  const totpForm = useForm<TOTPFormData>({
    resolver: zodResolver(totpSchema),
    defaultValues: {
      code: "",
    },
  });

  useEffect(() => {
    if (profile) {
      profileForm.reset({
        name: profile.name,
        email: profile.email,
        phone: profile.phone || "",
      });
    }
  }, [profile, profileForm]);

  useEffect(() => {
    if (editingWebhook) {
      webhookForm.reset({
        url: editingWebhook.url,
        events: editingWebhook.events,
        secret: "",
      });
    } else {
      webhookForm.reset({
        url: "",
        events: [],
        secret: "",
      });
    }
  }, [editingWebhook, webhookForm]);

  const updateProfileMutation = useMutation({
    mutationFn: settingsApi.updateProfile,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["profile"] });
      toast({
        title: "Success",
        description: "Profile updated successfully",
      });
    },
    onError: (error: ApiClientError) => {
      toast({
        title: "Error",
        description: error.message || "Failed to update profile",
        variant: "destructive",
      });
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
    onError: (error: ApiClientError) => {
      toast({
        title: "Error",
        description: error.message || "Failed to update password",
        variant: "destructive",
      });
    },
  });

  const initiate2FAMutation = useMutation({
    mutationFn: settingsApi.initiate2FA,
    onSuccess: (data) => {
      setQrCodeUrl(data.qr_code_url);
      setTotpSecret(data.secret);
      setQrDialogOpen(true);
    },
    onError: (error: ApiClientError) => {
      toast({
        title: "Error",
        description: error.message || "Failed to initiate 2FA setup",
        variant: "destructive",
      });
    },
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
    onError: (error: ApiClientError) => {
      toast({
        title: "Error",
        description: error.message || "Failed to enable 2FA. Check your code and try again.",
        variant: "destructive",
      });
    },
  });

  const disable2FAMutation = useMutation({
    mutationFn: settingsApi.disable2FA,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["2fa-status"] });
      toast({
        title: "Success",
        description: "2FA disabled successfully",
      });
    },
    onError: (error: ApiClientError) => {
      toast({
        title: "Error",
        description: error.message || "Failed to disable 2FA",
        variant: "destructive",
      });
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
    onError: (error: ApiClientError) => {
      toast({
        title: "Error",
        description: error.message || "Failed to regenerate backup codes",
        variant: "destructive",
      });
    },
  });

  const createApiKeyMutation = useMutation({
    mutationFn: settingsApi.createApiKey,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["api-keys"] });
      setApiKeyDialogOpen(false);
      apiKeyForm.reset();
      toast({
        title: "Success",
        description: "API key created successfully",
      });
    },
    onError: (error: ApiClientError) => {
      toast({
        title: "Error",
        description: error.message || "Failed to create API key",
        variant: "destructive",
      });
    },
  });

  const rotateApiKeyMutation = useMutation({
    mutationFn: settingsApi.rotateApiKey,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["api-keys"] });
      toast({
        title: "Success",
        description: "API key rotated successfully",
      });
    },
    onError: (error: ApiClientError) => {
      toast({
        title: "Error",
        description: error.message || "Failed to rotate API key",
        variant: "destructive",
      });
    },
  });

  const deleteApiKeyMutation = useMutation({
    mutationFn: settingsApi.deleteApiKey,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["api-keys"] });
      setDeleteKeyDialogOpen(false);
      setSelectedKeyId(null);
      toast({
        title: "Success",
        description: "API key deleted successfully",
      });
    },
    onError: (error: ApiClientError) => {
      toast({
        title: "Error",
        description: error.message || "Failed to delete API key",
        variant: "destructive",
      });
    },
  });

  const createWebhookMutation = useMutation({
    mutationFn: settingsApi.createWebhook,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["webhooks"] });
      setWebhookDialogOpen(false);
      webhookForm.reset();
      toast({
        title: "Success",
        description: "Webhook created successfully",
      });
    },
    onError: (error: ApiClientError) => {
      toast({
        title: "Error",
        description: error.message || "Failed to create webhook",
        variant: "destructive",
      });
    },
  });

  const updateWebhookMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: { url?: string; events?: string[]; secret?: string; is_active?: boolean } }) =>
      settingsApi.updateWebhook(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["webhooks"] });
      setWebhookDialogOpen(false);
      setEditingWebhook(null);
      webhookForm.reset();
      toast({
        title: "Success",
        description: "Webhook updated successfully",
      });
    },
    onError: (error: ApiClientError) => {
      toast({
        title: "Error",
        description: error.message || "Failed to update webhook",
        variant: "destructive",
      });
    },
  });

  const deleteWebhookMutation = useMutation({
    mutationFn: settingsApi.deleteWebhook,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["webhooks"] });
      setDeleteWebhookDialogOpen(false);
      setSelectedWebhookId(null);
      toast({
        title: "Success",
        description: "Webhook deleted successfully",
      });
    },
    onError: (error: ApiClientError) => {
      toast({
        title: "Error",
        description: error.message || "Failed to delete webhook",
        variant: "destructive",
      });
    },
  });

  const testWebhookMutation = useMutation({
    mutationFn: settingsApi.testWebhook,
    onSuccess: (data) => {
      if (data.success) {
        toast({
          title: "Success",
          description: `Webhook test successful (Status: ${data.status_code})`,
        });
      } else {
        toast({
          title: "Warning",
          description: `Webhook test failed: ${data.error || "Unknown error"}`,
          variant: "destructive",
        });
      }
      queryClient.invalidateQueries({ queryKey: ["webhooks"] });
    },
    onError: (error: ApiClientError) => {
      toast({
        title: "Error",
        description: error.message || "Failed to test webhook",
        variant: "destructive",
      });
    },
  });

  const handleCopy = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
    toast({
      title: "Copied",
      description: "Copied to clipboard",
    });
  };

  const handleProfileSubmit = (data: ProfileFormData) => {
    updateProfileMutation.mutate({
      name: data.name,
      email: data.email,
      phone: data.phone,
    });
  };

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
      disable2FAMutation.mutate();
    }
  };

  const handleVerifyTOTP = (data: TOTPFormData) => {
    enable2FAMutation.mutate({ totp_code: data.code });
  };

  const handleCreateApiKey = (data: ApiKeyFormData) => {
    createApiKeyMutation.mutate({
      name: data.name,
      permissions: data.permissions,
    });
  };

  const handleRotateKey = (keyId: string) => {
    setSelectedKeyId(keyId);
    setRotateKeyDialogOpen(true);
  };

  const confirmRotateKey = () => {
    if (selectedKeyId) {
      rotateApiKeyMutation.mutate(selectedKeyId);
      setRotateKeyDialogOpen(false);
    }
  };

  const handleDeleteKey = (keyId: string) => {
    setSelectedKeyId(keyId);
    setDeleteKeyDialogOpen(true);
  };

  const confirmDeleteKey = () => {
    if (selectedKeyId) {
      deleteApiKeyMutation.mutate(selectedKeyId);
    }
  };

  const handleCreateWebhook = (data: WebhookFormData) => {
    if (editingWebhook) {
      updateWebhookMutation.mutate({
        id: editingWebhook.id,
        data: {
          url: data.url,
          events: data.events,
        },
      });
    } else {
      createWebhookMutation.mutate({
        url: data.url,
        events: data.events,
        secret: data.secret,
      });
    }
  };

  const handleEditWebhook = (webhook: WebhookType) => {
    setEditingWebhook(webhook);
    setWebhookDialogOpen(true);
  };

  const handleDeleteWebhook = (webhookId: string) => {
    setSelectedWebhookId(webhookId);
    setDeleteWebhookDialogOpen(true);
  };

  const confirmDeleteWebhook = () => {
    if (selectedWebhookId) {
      deleteWebhookMutation.mutate(selectedWebhookId);
    }
  };

  const handleTestWebhook = (webhookId: string) => {
    testWebhookMutation.mutate(webhookId);
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

  const twoFactorEnabled = twoFactorStatus?.enabled || false;

  return (
    <div className="container mx-auto py-8 px-4 md:px-6 lg:px-8">
      <div className="mb-8">
        <h1 className="text-3xl font-bold tracking-tight">Account Settings</h1>
        <p className="text-muted-foreground mt-2">
          Manage your account settings, security, and integrations
        </p>
      </div>

      <Tabs defaultValue="profile" className="space-y-6">
        <TabsList className="grid w-full max-w-md grid-cols-2 lg:max-w-lg">
          <TabsTrigger value="profile" className="gap-2">
            <User className="h-4 w-4" />
            Profile
          </TabsTrigger>
          <TabsTrigger value="security" className="gap-2">
            <Shield className="h-4 w-4" />
            Security
          </TabsTrigger>
          <TabsTrigger value="api-keys" className="gap-2">
            <Key className="h-4 w-4" />
            API Keys
          </TabsTrigger>
          <TabsTrigger value="webhooks" className="gap-2">
            <Webhook className="h-4 w-4" />
            Webhooks
          </TabsTrigger>
        </TabsList>

        {/* Profile Tab */}
        <TabsContent value="profile" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <User className="h-5 w-5" />
                Profile Information
              </CardTitle>
              <CardDescription>
                Update your personal information and email address
              </CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={profileForm.handleSubmit(handleProfileSubmit)} className="space-y-4">
                <div className="grid gap-2">
                  <Label htmlFor="name">Full Name</Label>
                  <Input
                    id="name"
                    placeholder="Enter your full name"
                    {...profileForm.register("name")}
                    className="max-w-md"
                  />
                  {profileForm.formState.errors.name && (
                    <p className="text-sm text-destructive">{profileForm.formState.errors.name.message}</p>
                  )}
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="email">Email Address</Label>
                  <div className="relative max-w-md">
                    <Mail className="absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
                    <Input
                      id="email"
                      type="email"
                      placeholder="email@example.com"
                      {...profileForm.register("email")}
                      className="pl-10"
                    />
                  </div>
                  {profileForm.formState.errors.email && (
                    <p className="text-sm text-destructive">{profileForm.formState.errors.email.message}</p>
                  )}
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="phone">Phone Number</Label>
                  <div className="relative max-w-md">
                    <Smartphone className="absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
                    <Input
                      id="phone"
                      type="tel"
                      placeholder="+1 (555) 123-4567"
                      {...profileForm.register("phone")}
                      className="pl-10"
                    />
                  </div>
                  {profileForm.formState.errors.phone && (
                    <p className="text-sm text-destructive">{profileForm.formState.errors.phone.message}</p>
                  )}
                </div>
                <Button 
                  type="submit" 
                  className="mt-2"
                  disabled={updateProfileMutation.isPending}
                >
                  {updateProfileMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                  Save Changes
                </Button>
              </form>
            </CardContent>
          </Card>

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
        </TabsContent>

        {/* Security Tab */}
        <TabsContent value="security" className="space-y-6">
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
                  disabled={twoFactorLoading || initiate2FAMutation.isPending || disable2FAMutation.isPending}
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
        </TabsContent>

        {/* API Keys Tab */}
        <TabsContent value="api-keys" className="space-y-6">
          <Card>
            <CardHeader>
              <div className="flex items-center justify-between">
                <div>
                  <CardTitle className="flex items-center gap-2">
                    <Key className="h-5 w-5" />
                    API Keys
                  </CardTitle>
                  <CardDescription>
                    Manage your API keys for programmatic access
                  </CardDescription>
                </div>
                <Button size="sm" onClick={() => setApiKeyDialogOpen(true)}>
                  <Plus className="mr-2 h-4 w-4" />
                  Create New Key
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              {apiKeysLoading ? (
                <div className="flex justify-center p-8">
                  <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
                </div>
              ) : !apiKeys || apiKeys.length === 0 ? (
                <div className="text-center p-8 text-muted-foreground">
                  No API keys found. Create one to get started.
                </div>
              ) : (
                <div className="space-y-4">
                  {apiKeys.map((apiKey) => (
                    <div
                      key={apiKey.id}
                      className="flex flex-col gap-4 rounded-lg border p-4 md:flex-row md:items-center md:justify-between"
                    >
                      <div className="space-y-2">
                        <div className="flex items-center gap-2">
                          <span className="font-medium">{apiKey.name}</span>
                          {apiKey.is_active ? (
                            <Badge variant="default">Active</Badge>
                          ) : (
                            <Badge variant="secondary">Inactive</Badge>
                          )}
                        </div>
                        <div className="flex items-center gap-4 text-sm text-muted-foreground">
                          <div className="flex items-center gap-1">
                            <Calendar className="h-3 w-3" />
                            Created: {new Date(apiKey.created_at).toLocaleDateString()}
                          </div>
                          <div className="flex items-center gap-1">
                            <Calendar className="h-3 w-3" />
                            Last used: {apiKey.last_used_at ? new Date(apiKey.last_used_at).toLocaleDateString() : 'Never'}
                          </div>
                        </div>
                        {apiKey.key && (
                          <div className="flex items-center gap-2">
                            <code className="rounded bg-muted px-2 py-1 text-sm font-mono">
                              {apiKey.key}
                            </code>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-8 w-8"
                              onClick={() => handleCopy(apiKey.key!, apiKey.id)}
                            >
                              {copiedId === apiKey.id ? (
                                <Check className="h-4 w-4" />
                              ) : (
                                <Copy className="h-4 w-4" />
                              )}
                            </Button>
                          </div>
                        )}
                      </div>
                      <div className="flex gap-2">
                        <Button 
                          variant="outline" 
                          size="sm"
                          onClick={() => handleRotateKey(apiKey.id)}
                          disabled={rotateApiKeyMutation.isPending}
                        >
                          <RefreshCw className="mr-2 h-4 w-4" />
                          Regenerate
                        </Button>
                        <Button 
                          variant="outline" 
                          size="sm"
                          className="text-destructive hover:bg-destructive hover:text-destructive-foreground"
                          onClick={() => handleDeleteKey(apiKey.id)}
                        >
                          <Trash2 className="mr-2 h-4 w-4" />
                          Delete
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* Webhooks Tab */}
        <TabsContent value="webhooks" className="space-y-6">
          <Card>
            <CardHeader>
              <div className="flex items-center justify-between">
                <div>
                  <CardTitle className="flex items-center gap-2">
                    <Webhook className="h-5 w-5" />
                    Webhooks
                  </CardTitle>
                  <CardDescription>
                    Configure webhook endpoints to receive event notifications
                  </CardDescription>
                </div>
                <Button size="sm" onClick={() => { setEditingWebhook(null); setWebhookDialogOpen(true); }}>
                  <Plus className="mr-2 h-4 w-4" />
                  Add Webhook
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              {webhooksLoading ? (
                <div className="flex justify-center p-8">
                  <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
                </div>
              ) : !webhooks || webhooks.length === 0 ? (
                <div className="text-center p-8 text-muted-foreground">
                  No webhooks found. Create one to get started.
                </div>
              ) : (
                <div className="space-y-4">
                  {webhooks.map((webhook) => (
                    <div
                      key={webhook.id}
                      className="flex flex-col gap-4 rounded-lg border p-4 md:flex-row md:items-start md:justify-between"
                    >
                      <div className="space-y-2 flex-1">
                        <div className="flex items-center gap-2">
                          <span className="font-medium truncate">{webhook.url}</span>
                          <Badge
                            variant={webhook.is_active ? "default" : "secondary"}
                          >
                            {webhook.is_active ? "active" : "inactive"}
                          </Badge>
                        </div>
                        <div className="flex flex-wrap gap-1">
                          {webhook.events.map((event) => (
                            <Badge key={event} variant="outline" className="text-xs">
                              {event}
                            </Badge>
                          ))}
                        </div>
                        <div className="flex items-center gap-1 text-sm text-muted-foreground">
                          <Calendar className="h-3 w-3" />
                          Last triggered: {webhook.last_success_at
                            ? new Date(webhook.last_success_at).toLocaleString()
                            : "Never"}
                          {webhook.fail_count > 0 && (
                            <span className="text-destructive ml-2">
                              ({webhook.fail_count} failures)
                            </span>
                          )}
                        </div>
                      </div>
                      <div className="flex gap-2">
                        <Button 
                          variant="outline" 
                          size="sm"
                          onClick={() => handleTestWebhook(webhook.id)}
                          disabled={testWebhookMutation.isPending}
                        >
                          <Play className="mr-2 h-4 w-4" />
                          Test
                        </Button>
                        <Button 
                          variant="outline" 
                          size="sm"
                          onClick={() => handleEditWebhook(webhook)}
                        >
                          <Edit3 className="mr-2 h-4 w-4" />
                          Edit
                        </Button>
                        <Button 
                          variant="outline" 
                          size="sm"
                          className="text-destructive hover:bg-destructive hover:text-destructive-foreground"
                          onClick={() => handleDeleteWebhook(webhook.id)}
                        >
                          <Trash2 className="mr-2 h-4 w-4" />
                          Delete
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      {/* API Key Dialog */}
      <Dialog open={apiKeyDialogOpen} onOpenChange={setApiKeyDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Create API Key</DialogTitle>
            <DialogDescription>
              Create a new API key for programmatic access to your account.
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={apiKeyForm.handleSubmit(handleCreateApiKey)} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="key-name">Name</Label>
              <Input
                id="key-name"
                placeholder="e.g., Production API Key"
                {...apiKeyForm.register("name")}
              />
              {apiKeyForm.formState.errors.name && (
                <p className="text-sm text-destructive">{apiKeyForm.formState.errors.name.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label>Permissions</Label>
              <div className="space-y-2 border rounded-md p-3">
                {AVAILABLE_PERMISSIONS.map((permission) => (
                  <div key={permission} className="flex items-center space-x-2">
                    <input
                      type="checkbox"
                      id={permission}
                      value={permission}
                      {...apiKeyForm.register("permissions")}
                      className="rounded border-gray-300"
                    />
                    <Label htmlFor={permission} className="text-sm font-normal cursor-pointer">
                      {permission}
                    </Label>
                  </div>
                ))}
              </div>
              {apiKeyForm.formState.errors.permissions && (
                <p className="text-sm text-destructive">{apiKeyForm.formState.errors.permissions.message}</p>
              )}
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setApiKeyDialogOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={createApiKeyMutation.isPending}>
                {createApiKeyMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Create Key
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Webhook Dialog */}
      <Dialog open={webhookDialogOpen} onOpenChange={setWebhookDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{editingWebhook ? "Edit Webhook" : "Add Webhook"}</DialogTitle>
            <DialogDescription>
              {editingWebhook 
                ? "Update your webhook configuration." 
                : "Configure a new webhook endpoint to receive event notifications."}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={webhookForm.handleSubmit(handleCreateWebhook)} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="webhook-url">Endpoint URL</Label>
              <Input
                id="webhook-url"
                placeholder="https://example.com/webhook"
                {...webhookForm.register("url")}
              />
              {webhookForm.formState.errors.url && (
                <p className="text-sm text-destructive">{webhookForm.formState.errors.url.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="webhook-events">Events</Label>
              <Select
                onValueChange={(value) => {
                  const current = webhookForm.getValues("events") || [];
                  if (!current.includes(value)) {
                    webhookForm.setValue("events", [...current, value], { shouldValidate: true });
                  }
                }}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select events" />
                </SelectTrigger>
                <SelectContent>
                  {AVAILABLE_EVENTS.map((event) => (
                    <SelectItem key={event} value={event}>
                      {event}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <div className="flex flex-wrap gap-1 mt-2">
                {webhookForm.watch("events")?.map((event) => (
                  <Badge key={event} variant="secondary" className="cursor-pointer" onClick={() => {
                    const current = webhookForm.getValues("events") || [];
                    webhookForm.setValue("events", current.filter(e => e !== event), { shouldValidate: true });
                  }}>
                    {event} ×
                  </Badge>
                ))}
              </div>
              {webhookForm.formState.errors.events && (
                <p className="text-sm text-destructive">{webhookForm.formState.errors.events.message}</p>
              )}
            </div>
            {!editingWebhook && (
              <div className="space-y-2">
                <Label htmlFor="webhook-secret">Secret</Label>
                <Input
                  id="webhook-secret"
                  type="password"
                  placeholder="Webhook secret for signature verification"
                  {...webhookForm.register("secret")}
                />
                {webhookForm.formState.errors.secret && (
                  <p className="text-sm text-destructive">{webhookForm.formState.errors.secret.message}</p>
                )}
              </div>
            )}
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => { setWebhookDialogOpen(false); setEditingWebhook(null); }}>
                Cancel
              </Button>
              <Button type="submit" disabled={createWebhookMutation.isPending || updateWebhookMutation.isPending}>
                {(createWebhookMutation.isPending || updateWebhookMutation.isPending) && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {editingWebhook ? "Update" : "Add"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Delete API Key Dialog */}
      <Dialog open={deleteKeyDialogOpen} onOpenChange={setDeleteKeyDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Delete API Key</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete this API key? This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setDeleteKeyDialogOpen(false)}>
              Cancel
            </Button>
            <Button 
              type="button" 
              variant="destructive"
              onClick={confirmDeleteKey}
              disabled={deleteApiKeyMutation.isPending}
            >
              {deleteApiKeyMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={rotateKeyDialogOpen} onOpenChange={setRotateKeyDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Rotate API Key</DialogTitle>
            <DialogDescription>
              Are you sure you want to rotate this API key? The old key will stop working immediately.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setRotateKeyDialogOpen(false)}>
              Cancel
            </Button>
            <Button 
              type="button" 
              onClick={confirmRotateKey}
              disabled={rotateApiKeyMutation.isPending}
            >
              {rotateApiKeyMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Rotate
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Webhook Dialog */}
      <Dialog open={deleteWebhookDialogOpen} onOpenChange={setDeleteWebhookDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Delete Webhook</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete this webhook? This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setDeleteWebhookDialogOpen(false)}>
              Cancel
            </Button>
            <Button 
              type="button" 
              variant="destructive"
              onClick={confirmDeleteWebhook}
              disabled={deleteWebhookMutation.isPending}
            >
              {deleteWebhookMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

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
            {qrCodeUrl ? (
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
              {(backupCodes.length > 0 ? backupCodes : backupCodesData?.backup_codes || []).map((code, index) => (
                <code key={index} className="text-center font-mono text-sm py-1">
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
    </div>
  );
}
