// Package tasks provides async task handlers for VM operations.
// This file contains the handler registration function that maps task types
// to their respective handler functions.
package tasks

import (
	"context"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// RegisterAllHandlers registers all task handlers with the worker.
func RegisterAllHandlers(worker *Worker, deps *HandlerDeps) {
	worker.RegisterHandler(models.TaskTypeVMCreate, func(ctx context.Context, task *models.Task) error {
		return handleVMCreate(ctx, task, deps)
	})
	worker.RegisterHandler(models.TaskTypeVMReinstall, func(ctx context.Context, task *models.Task) error {
		return handleVMReinstall(ctx, task, deps)
	})
	worker.RegisterHandler(models.TaskTypeVMMigrate, func(ctx context.Context, task *models.Task) error {
		return handleVMMigrate(ctx, task, deps)
	})
	worker.RegisterHandler(models.TaskTypeBackupCreate, func(ctx context.Context, task *models.Task) error {
		return handleBackupCreate(ctx, task, deps)
	})
	worker.RegisterHandler(models.TaskTypeVMDelete, func(ctx context.Context, task *models.Task) error {
		return handleVMDelete(ctx, task, deps)
	})
	worker.RegisterHandler(models.TaskTypeBackupRestore, func(ctx context.Context, task *models.Task) error {
		return handleBackupRestore(ctx, task, deps)
	})
	worker.RegisterHandler(models.TaskTypeSnapshotCreate, func(ctx context.Context, task *models.Task) error {
		return handleSnapshotCreate(ctx, task, deps)
	})
	worker.RegisterHandler(models.TaskTypeSnapshotRevert, func(ctx context.Context, task *models.Task) error {
		return handleSnapshotRevert(ctx, task, deps)
	})
	worker.RegisterHandler(models.TaskTypeSnapshotDelete, func(ctx context.Context, task *models.Task) error {
		return handleSnapshotDelete(ctx, task, deps)
	})
	worker.RegisterHandler(models.TaskTypeVMResize, func(ctx context.Context, task *models.Task) error {
		return handleVMResize(ctx, task, deps)
	})
	worker.RegisterHandler(models.TaskTypeTemplateBuild, func(ctx context.Context, task *models.Task) error {
		return handleTemplateBuild(ctx, task, deps)
	})
	worker.RegisterHandler(models.TaskTypeTemplateDistribute, func(ctx context.Context, task *models.Task) error {
		return handleTemplateDistribute(ctx, task, deps)
	})

	deps.Logger.Info("all task handlers registered",
		"handlers", []string{
			models.TaskTypeVMCreate,
			models.TaskTypeVMReinstall,
			models.TaskTypeVMMigrate,
			models.TaskTypeVMResize,
			models.TaskTypeBackupCreate,
			models.TaskTypeVMDelete,
			models.TaskTypeBackupRestore,
			models.TaskTypeSnapshotCreate,
			models.TaskTypeSnapshotRevert,
			models.TaskTypeSnapshotDelete,
			models.TaskTypeTemplateBuild,
			models.TaskTypeTemplateDistribute,
		})
}