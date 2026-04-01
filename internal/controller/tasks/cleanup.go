package tasks

import (
	"context"
	"log/slog"
)

// CompensationStep describes a rollback action for a previously completed step.
type CompensationStep struct {
	Name    string
	Cleanup func(ctx context.Context) error
}

// CompensationStack stores rollback actions in execution order.
type CompensationStack struct {
	steps  []CompensationStep
	logger *slog.Logger
}

// NewCompensationStack creates a new compensation stack.
func NewCompensationStack(logger *slog.Logger) *CompensationStack {
	return &CompensationStack{
		steps:  make([]CompensationStep, 0),
		logger: logger,
	}
}

// Push adds a rollback action to the stack.
func (cs *CompensationStack) Push(name string, cleanup func(ctx context.Context) error) {
	cs.steps = append(cs.steps, CompensationStep{
		Name:    name,
		Cleanup: cleanup,
	})
}

// Rollback executes all rollback actions in reverse order.
func (cs *CompensationStack) Rollback(ctx context.Context) {
	for i := len(cs.steps) - 1; i >= 0; i-- {
		step := cs.steps[i]
		if step.Cleanup == nil {
			continue
		}
		if err := step.Cleanup(ctx); err != nil {
			cs.logger.Error("compensation step failed", "step", step.Name, "error", err)
		}
	}
}
