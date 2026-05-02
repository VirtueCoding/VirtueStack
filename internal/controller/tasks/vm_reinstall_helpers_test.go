package tasks

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeReinstallIPAM struct {
	ipv4       *models.IPAddress
	ipv4Err    error
	ipv6       []models.VMIPv6Subnet
	ipv6Err    error
	releaseErr error
}

func (i fakeReinstallIPAM) AllocateIPv4(context.Context, string, string, string) (*models.IPAddress, error) {
	return nil, nil
}

func (i fakeReinstallIPAM) AllocateIPv6(context.Context, string, string, string) (*models.VMIPv6Subnet, error) {
	return nil, nil
}

func (i fakeReinstallIPAM) ReleaseIPsByVM(context.Context, string) error {
	return i.releaseErr
}

func (i fakeReinstallIPAM) GetPrimaryIPv4(context.Context, string) (*models.IPAddress, error) {
	return i.ipv4, i.ipv4Err
}

func (i fakeReinstallIPAM) GetIPv6SubnetsByVM(context.Context, string) ([]models.VMIPv6Subnet, error) {
	return i.ipv6, i.ipv6Err
}

func TestBuildReinstallRequest(t *testing.T) {
	cephPool := "vs-vms"
	info := &vmReinstallInfo{
		vm: &models.VM{
			ID:            "vm-1",
			Hostname:      "vm-1.example.test",
			VCPU:          2,
			MemoryMB:      4096,
			DiskGB:        40,
			MACAddress:    "52:54:00:aa:bb:cc",
			PortSpeedMbps: 1000,
			CephPool:      &cephPool,
		},
		template: &models.Template{
			ID:          "template-1",
			RBDImage:    "ubuntu-24.04",
			RBDSnapshot: "base",
		},
		nodeID: "node-1",
	}

	tests := []struct {
		name          string
		ipam          IPAMService
		wantIPv4      string
		wantIPv6      string
		wantIPv6GW    string
		wantNoNetwork bool
	}{
		{
			name: "includes preserved VM resources and network config",
			ipam: fakeReinstallIPAM{
				ipv4: &models.IPAddress{Address: "192.0.2.10"},
				ipv6: []models.VMIPv6Subnet{{Subnet: "2001:db8::/64", Gateway: "2001:db8::1"}},
			},
			wantIPv4:   "192.0.2.10",
			wantIPv6:   "2001:db8::/64",
			wantIPv6GW: "2001:db8::1",
		},
		{
			name: "continues without network config when lookup fails",
			ipam: fakeReinstallIPAM{
				ipv4Err: errors.New("ipv4 unavailable"),
				ipv6Err: errors.New("ipv6 unavailable"),
			},
			wantNoNetwork: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := &HandlerDeps{
				IPAMService:    tt.ipam,
				DNSNameservers: []string{"1.1.1.1", "9.9.9.9"},
				CephMonitors:   []string{"10.0.0.1:6789"},
				CephUser:       "virtue",
				CephSecretUUID: "secret-uuid",
			}
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))

			req, err := buildReinstallRequest(
				context.Background(),
				deps,
				&VMReinstallPayload{VMID: "vm-1", TemplateID: "template-1", SSHKeys: []string{"ssh-ed25519 AAAA test"}},
				info,
				"$6$salt$hash",
				logger,
			)

			require.NoError(t, err)
			assert.Equal(t, "vm-1", req.VMID)
			assert.Equal(t, "vm-1.example.test", req.Hostname)
			assert.Equal(t, 2, req.VCPU)
			assert.Equal(t, 4096, req.MemoryMB)
			assert.Equal(t, 40, req.DiskGB)
			assert.Equal(t, "52:54:00:aa:bb:cc", req.MACAddress)
			assert.Equal(t, 1000, req.PortSpeedMbps)
			assert.Equal(t, "ubuntu-24.04", req.TemplateRBDImage)
			assert.Equal(t, "base", req.TemplateRBDSnapshot)
			assert.Equal(t, "vs-vms", req.CephPool)
			assert.Equal(t, []string{"1.1.1.1", "9.9.9.9"}, req.Nameservers)
			assert.Equal(t, []string{"10.0.0.1:6789"}, req.CephMonitors)
			assert.Equal(t, "virtue", req.CephUser)
			assert.Equal(t, "secret-uuid", req.CephSecretUUID)
			if tt.wantNoNetwork {
				assert.Empty(t, req.IPv4Address)
				assert.Empty(t, req.IPv6Address)
				return
			}
			assert.Equal(t, tt.wantIPv4, req.IPv4Address)
			assert.Equal(t, tt.wantIPv6, req.IPv6Address)
			assert.Equal(t, tt.wantIPv6GW, req.IPv6Gateway)
		})
	}
}
