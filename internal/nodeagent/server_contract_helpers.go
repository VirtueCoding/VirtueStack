package nodeagent

import (
	"context"
	stderrors "errors"

	"github.com/AbuGosok/VirtueStack/internal/nodeagent/storage/downloadutil"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func grpcStatusForNodeError(operation string, err error) error {
	switch {
	case stderrors.Is(err, sharederrors.ErrNotFound):
		return status.Errorf(codes.NotFound, "%s failed", operation)
	case stderrors.Is(err, context.Canceled):
		return status.Errorf(codes.Canceled, "%s failed", operation)
	case isTimeoutLikeError(err):
		return status.Errorf(codes.DeadlineExceeded, "%s failed", operation)
	default:
		return status.Errorf(codes.Internal, "%s failed", operation)
	}
}

func isTimeoutLikeError(err error) bool {
	if stderrors.Is(err, sharederrors.ErrTimeout) || stderrors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var timeoutErr interface{ Timeout() bool }
	return stderrors.As(err, &timeoutErr) && timeoutErr.Timeout()
}

func verifyTemplateIntegrity(path string, expectedSize int64, expectedChecksum string) error {
	return downloadutil.VerifyFileIntegrity(path, expectedSize, expectedChecksum)
}

func verifyCachedTemplateIntegrity(storageBackend, path string, expectedSize int64, expectedChecksum string) error {
	if storageBackend != "qcow" {
		return nil
	}
	return verifyTemplateIntegrity(path, expectedSize, expectedChecksum)
}
