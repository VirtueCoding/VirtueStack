package nodeagent

import (
	"testing"

	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var templateBuildValidationError = sharederrors.NewValidationError("iso_source", "exactly one of iso_path or iso_url must be set")

func TestMapTemplateBuildErrorMapsValidationToInvalidArgument(t *testing.T) {
	t.Parallel()

	err := mapTemplateBuildError(templateBuildValidationError)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Equal(t, "building template from ISO failed", st.Message())
}
