package nodeagent

import (
	stderrors "errors"
	"fmt"
	"os"

	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const templateBuildOperation = "building template from ISO"
const templateImportOperation = "importing built template"

func backupArtifactPathForWrite(finalPath string) string {
	return finalPath + ".partial"
}

func finalizeBackupArtifact(stagingPath, finalPath string) error {
	if err := os.Rename(stagingPath, finalPath); err != nil {
		return fmt.Errorf("renaming staged backup artifact: %w", err)
	}

	return nil
}

func cleanupBackupArtifact(path string) {
	if path == "" {
		return
	}

	if err := os.Remove(path); err != nil && !stderrors.Is(err, os.ErrNotExist) {
		return
	}
}

func mapBackupOperationError(operation string, err error) error {
	return grpcStatusForNodeError(operation, err)
}

func mapTemplateBuildError(err error) error {
	if stderrors.Is(err, sharederrors.ErrValidation) {
		return status.Error(codes.InvalidArgument, templateBuildOperation+" failed")
	}

	return grpcStatusForNodeError(templateBuildOperation, err)
}

func templateBuildClientMessage(err error) string {
	if stderrors.Is(err, sharederrors.ErrValidation) {
		return "invalid template build request"
	}

	return templateBuildOperation + " failed"
}

func templateImportClientMessage() string {
	return templateImportOperation + " failed"
}

func templateCacheRequestClientMessage(err error) string {
	if stderrors.Is(err, sharederrors.ErrValidation) {
		return "invalid template cache request"
	}

	return "invalid template cache request"
}

func templateCacheIntegrityClientMessage(err error) string {
	return "verifying cached template failed"
}

func templateCacheDownloadClientMessage(err error) string {
	return "downloading template failed"
}
