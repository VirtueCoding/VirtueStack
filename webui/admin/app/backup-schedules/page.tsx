"use client";

import { useState } from "react";
import { Calendar, Clock } from "lucide-react";
import { AdminScheduleList } from "@/components/backups/AdminScheduleList";
import { CreateScheduleModal } from "@/components/backups/CreateScheduleModal";
import { AdminBackupSchedule } from "@/lib/api-client";

export default function BackupSchedulesPage() {
  const [createModalOpen, setCreateModalOpen] = useState(false);
  const [editSchedule, setEditSchedule] = useState<AdminBackupSchedule | null>(null);

  const handleEditSchedule = (schedule: AdminBackupSchedule) => {
    setEditSchedule(schedule);
    setCreateModalOpen(true);
  };

  const handleCreateSchedule = () => {
    setEditSchedule(null);
    setCreateModalOpen(true);
  };

  const handleModalClose = (open: boolean) => {
    setCreateModalOpen(open);
    if (!open) {
      setEditSchedule(null);
    }
  };

  return (
    <div className="min-h-screen bg-background p-6 md:p-8">
      <div className="mx-auto max-w-7xl space-y-8">
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">
              Backup Schedules
            </h1>
            <p className="text-muted-foreground">
              Configure automated backup campaigns for multiple VMs
            </p>
          </div>
        </div>

        {/* Stats Cards */}
        <div className="grid gap-4 md:grid-cols-3">
          <div className="rounded-lg border bg-card p-4">
            <div className="flex items-center gap-2">
              <Calendar className="h-5 w-5 text-muted-foreground" />
              <span className="text-sm font-medium text-muted-foreground">
                Schedules
              </span>
            </div>
            <p className="mt-2 text-2xl font-bold">Active backup campaigns</p>
          </div>
          <div className="rounded-lg border bg-card p-4">
            <div className="flex items-center gap-2">
              <Clock className="h-5 w-5 text-muted-foreground" />
              <span className="text-sm font-medium text-muted-foreground">
                Frequency
              </span>
            </div>
            <p className="mt-2 text-2xl font-bold">Daily, Weekly, Monthly</p>
          </div>
          <div className="rounded-lg border bg-card p-4">
            <div className="flex items-center gap-2">
              <Clock className="h-5 w-5 text-muted-foreground" />
              <span className="text-sm font-medium text-muted-foreground">
                Targets
              </span>
            </div>
            <p className="mt-2 text-2xl font-bold">Plans, Nodes, Customers</p>
          </div>
        </div>

        <AdminScheduleList
          onEditSchedule={handleEditSchedule}
          onCreateSchedule={handleCreateSchedule}
        />

        <CreateScheduleModal
          open={createModalOpen}
          onOpenChange={handleModalClose}
          editSchedule={editSchedule}
        />
      </div>
    </div>
  );
}