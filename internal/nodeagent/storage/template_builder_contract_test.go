package storage

import (
	"testing"

	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateBuildISOSourceRejectsMultipleSources(t *testing.T) {
	t.Parallel()

	err := ValidateBuildISOSource(BuildConfig{
		ISOPath: "/var/lib/virtuestack/a.iso",
		ISOURL:  "https://example.com/a.iso",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, sharederrors.ErrValidation)
	assert.Contains(t, err.Error(), "exactly one of iso_path or iso_url must be set")
}
