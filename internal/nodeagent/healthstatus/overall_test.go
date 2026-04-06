package healthstatus

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOverallNodeHealthy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		libvirtConnected bool
		storageConnected bool
		want             bool
	}{
		{
			name:             "healthy when libvirt and storage are connected",
			libvirtConnected: true,
			storageConnected: true,
			want:             true,
		},
		{
			name:             "unhealthy when libvirt is disconnected",
			libvirtConnected: false,
			storageConnected: true,
			want:             false,
		},
		{
			name:             "unhealthy when storage is disconnected",
			libvirtConnected: true,
			storageConnected: false,
			want:             false,
		},
		{
			name:             "unhealthy when both dependencies are disconnected",
			libvirtConnected: false,
			storageConnected: false,
			want:             false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, OverallNodeHealthy(tt.libvirtConnected, tt.storageConnected))
		})
	}
}
