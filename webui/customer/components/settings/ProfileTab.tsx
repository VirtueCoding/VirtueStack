"use client";

import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@virtuestack/ui";
import { Button } from "@virtuestack/ui";
import { Input } from "@virtuestack/ui";
import { Label } from "@virtuestack/ui";
import { useToast } from "@virtuestack/ui";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { settingsApi } from "@/lib/api-client";
import { useMutationToast } from "@/lib/utils/toast-helpers";
import { User, Mail, Smartphone, Loader2 } from "lucide-react";

const profileSchema = z.object({
  name: z.string().min(2, "Name must be at least 2 characters"),
  email: z.string().email("Invalid email address"),
  phone: z.string().optional(),
});

type ProfileFormData = z.infer<typeof profileSchema>;

interface ProfileTabProps {
  profile: {
    name: string;
    email: string;
    phone?: string;
  } | null | undefined;
  isLoading: boolean;
}

export function ProfileTab({ profile, isLoading }: ProfileTabProps) {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const { createMutationOnError } = useMutationToast();

  const profileForm = useForm<ProfileFormData>({
    resolver: zodResolver(profileSchema),
    defaultValues: {
      name: "",
      email: "",
      phone: "",
    },
    values: profile ? {
      name: profile.name,
      email: profile.email,
      phone: profile.phone || "",
    } : undefined,
  });

  const updateProfileMutation = useMutation({
    mutationFn: settingsApi.updateProfile,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["profile"] });
      toast({
        title: "Success",
        description: "Profile updated successfully",
      });
    },
    onError: createMutationOnError("Failed to update profile"),
  });

  const handleProfileSubmit = (data: ProfileFormData) => {
    updateProfileMutation.mutate({
      name: data.name,
      email: data.email,
      phone: data.phone,
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
    </>
  );
}