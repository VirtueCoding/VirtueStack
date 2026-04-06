package nodeagent

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type codedError interface {
	error
	ErrorCode() string
}

func mapSnapshotStorageError(operation string, err error) error {
	var coded codedError
	if ok := asCodedError(err, &coded); ok {
		switch coded.ErrorCode() {
		case "NOT_FOUND":
			return status.Error(codes.NotFound, "snapshot not found")
		case "INVALID_ARGUMENT":
			return status.Error(codes.InvalidArgument, "snapshot_id is invalid")
		}
	}

	return status.Errorf(codes.Internal, "%s: %v", operation, err)
}

func asCodedError(err error, target *codedError) bool {
	if err == nil {
		return false
	}

	coded, ok := err.(codedError)
	if ok {
		*target = coded
		return true
	}

	type unwrapper interface {
		Unwrap() error
	}
	uw, ok := err.(unwrapper)
	if !ok {
		return false
	}

	return asCodedError(uw.Unwrap(), target)
}
