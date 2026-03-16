"use client";

import { useState, useEffect, useCallback } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Settings, Loader2, Edit3, Save, X } from "lucide-react";
import { adminSettingsApi, type SystemSetting } from "@/lib/api-client";
import { useToast } from "@/components/ui/use-toast";

export default function SettingsPage() {
  const { toast } = useToast();
  const [settings, setSettings] = useState<SystemSetting[]>([]);
  const [loading, setLoading] = useState(true);
  const [editingKey, setEditingKey] = useState<string | null>(null);
  const [editValue, setEditValue] = useState("");
  const [saving, setSaving] = useState(false);

  const loadSettings = useCallback(async () => {
    try {
      const data = await adminSettingsApi.getSettings();
      setSettings(data || []);
    } catch {
      toast({
        title: "Error",
        description: "Failed to load settings.",
        variant: "destructive",
      });
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    loadSettings();
  }, [loadSettings]);

  const handleEdit = (setting: SystemSetting) => {
    setEditingKey(setting.key);
    setEditValue(setting.value);
  };

  const handleCancel = () => {
    setEditingKey(null);
    setEditValue("");
  };

  const handleSave = async () => {
    if (!editingKey) return;
    setSaving(true);
    try {
      const updated = await adminSettingsApi.putSetting(editingKey, editValue);
      setSettings((prev) =>
        prev.map((s) => (s.key === editingKey ? { ...s, value: updated.value } : s))
      );
      toast({
        title: "Setting Updated",
        description: `"${editingKey}" has been updated.`,
      });
      setEditingKey(null);
      setEditValue("");
    } catch (error) {
      toast({
        title: "Update Failed",
        description: error instanceof Error ? error.message : "Failed to update setting",
        variant: "destructive",
      });
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="min-h-screen bg-background p-6 md:p-8">
      <div className="mx-auto max-w-5xl space-y-8">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Settings</h1>
          <p className="text-muted-foreground">Manage system configuration.</p>
        </div>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Settings className="h-5 w-5" />
              System Settings
            </CardTitle>
            <CardDescription>
              View and edit system configuration key-value pairs.
            </CardDescription>
          </CardHeader>
          <CardContent>
            {loading ? (
              <div className="flex justify-center py-8">
                <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
              </div>
            ) : settings.length === 0 ? (
              <div className="rounded-md bg-muted p-4 text-sm text-muted-foreground">
                No settings found.
              </div>
            ) : (
              <div className="rounded-md border">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-[200px]">Key</TableHead>
                      <TableHead>Description</TableHead>
                      <TableHead>Value</TableHead>
                      <TableHead className="text-right">Actions</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {settings.map((setting) => (
                      <TableRow key={setting.key}>
                        <TableCell className="font-mono text-sm font-medium">
                          {setting.key}
                        </TableCell>
                        <TableCell className="text-sm text-muted-foreground">
                          {setting.description || "—"}
                        </TableCell>
                        <TableCell>
                          {editingKey === setting.key ? (
                            <Input
                              value={editValue}
                              onChange={(e) => setEditValue(e.target.value)}
                              className="max-w-xs"
                              autoFocus
                              onKeyDown={(e) => {
                                if (e.key === "Enter") handleSave();
                                if (e.key === "Escape") handleCancel();
                              }}
                            />
                          ) : (
                            <span className="text-sm font-mono">{setting.value}</span>
                          )}
                        </TableCell>
                        <TableCell className="text-right">
                          {editingKey === setting.key ? (
                            <div className="flex justify-end gap-1">
                              <Button
                                variant="ghost"
                                size="icon"
                                onClick={handleCancel}
                                title="Cancel"
                              >
                                <X className="h-4 w-4" />
                              </Button>
                              <Button
                                variant="ghost"
                                size="icon"
                                onClick={handleSave}
                                disabled={saving}
                                title="Save"
                              >
                                {saving ? (
                                  <Loader2 className="h-4 w-4 animate-spin" />
                                ) : (
                                  <Save className="h-4 w-4" />
                                )}
                              </Button>
                            </div>
                          ) : (
                            <Button
                              variant="ghost"
                              size="icon"
                              onClick={() => handleEdit(setting)}
                              title="Edit"
                            >
                              <Edit3 className="h-4 w-4" />
                            </Button>
                          )}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
