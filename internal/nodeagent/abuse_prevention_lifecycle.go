package nodeagent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/nodeagent/transferutil"
)

var (
	abusePreventionTapLookupRetries  = 5
	abusePreventionTapLookupInterval = 200 * time.Millisecond
	abusePreventionCleanupTimeout    = 5 * time.Second
)

type vmRuleManager interface {
	ApplyVMRules(ctx context.Context, tapInterface string) error
	RemoveVMRules(ctx context.Context, tapInterface string) error
}

type tapInterfaceResolver func(ctx context.Context, vmID string) (string, error)
type vmDeleteFunc func(ctx context.Context, vmID string) error
type storageCleanupFunc func(ctx context.Context, vmID string) error

func ensureAbusePreventionAfterCreate(
	ctx context.Context,
	vmID string,
	logger *slog.Logger,
	resolveTap tapInterfaceResolver,
	ruleManager vmRuleManager,
	deleteVM vmDeleteFunc,
	cleanupStorage storageCleanupFunc,
) error {
	tapInterface, err := lookupTapInterfaceWithRetry(ctx, vmID, resolveTap)
	if err != nil {
		return cleanupCreatedVM(ctx, vmID, logger, "", deleteVM, cleanupStorage, fmt.Errorf("getting tap interface for abuse prevention: %w", err), ruleManager)
	}

	if err := ruleManager.ApplyVMRules(ctx, tapInterface); err != nil {
		return cleanupCreatedVM(ctx, vmID, logger, tapInterface, deleteVM, cleanupStorage, fmt.Errorf("applying abuse prevention rules: %w", err), ruleManager)
	}

	return nil
}

func deleteVMWithAbusePreventionCleanup(
	ctx context.Context,
	vmID string,
	logger *slog.Logger,
	resolveTap tapInterfaceResolver,
	ruleManager vmRuleManager,
	deleteVM vmDeleteFunc,
) error {
	if tapInterface, err := resolveTap(ctx, vmID); err != nil {
		logger.Warn("failed to get tap interface for abuse prevention cleanup", "error", err)
	} else if err := ruleManager.RemoveVMRules(ctx, tapInterface); err != nil {
		logger.Warn("failed to remove abuse prevention rules", "error", err, "tap", tapInterface)
	}

	return deleteVM(ctx, vmID)
}

func lookupTapInterfaceWithRetry(ctx context.Context, vmID string, resolveTap tapInterfaceResolver) (string, error) {
	var lastErr error

	for attempt := 0; attempt < abusePreventionTapLookupRetries; attempt++ {
		tapInterface, err := resolveTap(ctx, vmID)
		if err == nil {
			return tapInterface, nil
		}
		lastErr = err
		if attempt == abusePreventionTapLookupRetries-1 {
			break
		}

		timer := time.NewTimer(abusePreventionTapLookupInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}

	return "", lastErr
}

func cleanupCreatedVM(
	ctx context.Context,
	vmID string,
	logger *slog.Logger,
	tapInterface string,
	deleteVM vmDeleteFunc,
	cleanupStorage storageCleanupFunc,
	operationErr error,
	ruleManager vmRuleManager,
) error {
	cleanupCtx, cancel := transferutil.DetachedTimeoutContext(ctx, abusePreventionCleanupTimeout)
	defer cancel()

	if tapInterface != "" {
		if err := ruleManager.RemoveVMRules(cleanupCtx, tapInterface); err != nil {
			logger.Warn("failed to remove partial abuse prevention rules during cleanup", "error", err, "tap", tapInterface)
		}
	}

	if err := deleteVM(cleanupCtx, vmID); err != nil {
		operationErr = fmt.Errorf("%w: cleanup VM %s: %v", operationErr, vmID, err)
	}

	if cleanupStorage != nil {
		if err := cleanupStorage(cleanupCtx, vmID); err != nil {
			return fmt.Errorf("%w: cleanup storage for VM %s: %v", operationErr, vmID, err)
		}
	}

	return operationErr
}
