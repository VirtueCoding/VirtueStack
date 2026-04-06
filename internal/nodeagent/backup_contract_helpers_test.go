package nodeagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestBackupArtifactPathForWriteUsesStagingFile(t *testing.T) {
	t.Parallel()

	finalPath := filepath.Join("var", "lib", "virtuestack", "backups", "vm.qcow2")
	stagingPath := backupArtifactPathForWrite(finalPath)

	assert.NotEqual(t, finalPath, stagingPath)
	assert.Equal(t, finalPath+".partial", stagingPath)
}

func TestFinalizeBackupArtifactRenamesStagingIntoPlace(t *testing.T) {
	t.Parallel()

	dir := backupTestDir(t)
	finalPath := filepath.Join(dir, "backup.img")
	stagingPath := backupArtifactPathForWrite(finalPath)
	require.NoError(t, os.WriteFile(stagingPath, []byte("backup-data"), 0o600))

	require.NoError(t, finalizeBackupArtifact(stagingPath, finalPath))

	data, err := os.ReadFile(finalPath)
	require.NoError(t, err)
	assert.Equal(t, []byte("backup-data"), data)
	_, err = os.Stat(stagingPath)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestCleanupBackupArtifactRemovesPartialFileOnFailure(t *testing.T) {
	t.Parallel()

	dir := backupTestDir(t)
	finalPath := filepath.Join(dir, "backup.img")
	stagingPath := backupArtifactPathForWrite(finalPath)
	require.NoError(t, os.WriteFile(stagingPath, []byte("partial"), 0o600))

	cleanupBackupArtifact(stagingPath)

	_, err := os.Stat(stagingPath)
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(finalPath)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestMapBackupOperationErrorRedactsInternalDetails(t *testing.T) {
	t.Parallel()

	err := mapBackupOperationError("creating backup artifact", errors.New("dd: failed writing /var/lib/virtuestack/backups/vm.qcow2"))
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Equal(t, "creating backup artifact failed", st.Message())
}

func TestMapTemplateBuildErrorRedactsInternalDetails(t *testing.T) {
	t.Parallel()

	err := mapTemplateBuildError(context.Canceled)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Canceled, st.Code())
	assert.Equal(t, "building template from ISO failed", st.Message())
}

func TestTemplateBuildClientMessageUsesSafeValidationMessage(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "invalid template build request", templateBuildClientMessage(templateBuildValidationError))
}

func TestTemplateImportClientMessageUsesSafeGenericMessage(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "importing built template failed", templateImportClientMessage())
}

func TestTemplateCacheRequestClientMessageUsesSafeGenericMessage(t *testing.T) {
	t.Parallel()

	err := errors.New(`validating template ref: expected volume group "vg0" in "/dev/vg1/template-base"`)

	assert.Equal(t, "invalid template cache request", templateCacheRequestClientMessage(err))
}

func TestTemplateCacheIntegrityClientMessageRedactsInternalDetails(t *testing.T) {
	t.Parallel()

	err := errors.New("stat /var/lib/virtuestack/templates/template.qcow2: no such file or directory")

	assert.Equal(t, "verifying cached template failed", templateCacheIntegrityClientMessage(err))
}

func TestTemplateCacheDownloadClientMessageRedactsInternalDetails(t *testing.T) {
	t.Parallel()

	err := errors.New("downloading template from https://controller.internal/templates/1: permission denied")

	assert.Equal(t, "downloading template failed", templateCacheDownloadClientMessage(err))
}

func backupTestDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp(".", "backup-contract-test-")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(dir))
	})

	return dir
}
