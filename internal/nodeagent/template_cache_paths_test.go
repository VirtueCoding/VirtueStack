package nodeagent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveTemplateLocalPath(t *testing.T) {
	tests := []struct {
		name           string
		storageBackend string
		configured     string
		storagePath    string
		lvmVG          string
		ref            string
		wantPath       string
		wantErr        bool
	}{
		{
			name:           "qcow uses configured storage path",
			storageBackend: "qcow",
			configured:     "ceph",
			storagePath:    "/srv/virtuestack",
			ref:            "ubuntu-2404",
			wantPath:       "/srv/virtuestack/templates/ubuntu-2404.qcow2",
		},
		{
			name:           "lvm uses canonical lv device path",
			storageBackend: "lvm",
			configured:     "ceph",
			lvmVG:          "vgvs",
			ref:            "ubuntu-2404",
			wantPath:       "/dev/vgvs/ubuntu-2404-base",
		},
		{
			name:           "empty request falls back to configured backend",
			storageBackend: "",
			configured:     "qcow",
			storagePath:    "/srv/virtuestack",
			ref:            "ubuntu-2404",
			wantPath:       "/srv/virtuestack/templates/ubuntu-2404.qcow2",
		},
		{
			name:           "unknown backend errors",
			storageBackend: "mystery",
			configured:     "ceph",
			ref:            "ubuntu-2404",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := resolveTemplateLocalPath(effectiveTemplateStorageBackend(tt.storageBackend, tt.configured), tt.storagePath, tt.lvmVG, tt.ref)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantPath, path)
		})
	}
}

func TestEffectiveTemplateStorageBackend(t *testing.T) {
	assert.Equal(t, "qcow", effectiveTemplateStorageBackend("", "qcow"))
	assert.Equal(t, "lvm", effectiveTemplateStorageBackend("lvm", "qcow"))
}

func TestValidateTemplateRequestBackend(t *testing.T) {
	tests := []struct {
		name            string
		requested       string
		configured      string
		wantErr         bool
		wantErrContains string
	}{
		{
			name:       "empty requested backend is accepted",
			requested:  "",
			configured: "qcow",
		},
		{
			name:       "matching backend is accepted",
			requested:  "lvm",
			configured: "lvm",
		},
		{
			name:            "mismatched backend is rejected",
			requested:       "ceph",
			configured:      "qcow",
			wantErr:         true,
			wantErrContains: "does not match configured backend",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTemplateRequestBackend(tt.requested, tt.configured)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)
				return
			}

			require.NoError(t, err)
		})
	}
}
