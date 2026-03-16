package vm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cephConfig() *DomainConfig {
	return &DomainConfig{
		VMID:             "550e8400-e29b-41d4-a716-446655440000",
		Hostname:         "test-vm01",
		VCPU:             4,
		MemoryMB:         8192,
		StorageBackend:   StorageBackendCeph,
		CephPool:         "vs-vms",
		CephMonitors:     []string{"10.0.0.1", "10.0.0.2"},
		CephUser:         "libvirt",
		CephSecretUUID:   "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		MACAddress:       "52:54:00:aa:bb:cc",
		IPv4Address:      "192.168.1.100",
		IPv6Address:      "2001:db8::1",
		PortSpeedKbps:    1024000,
		BurstKB:          131072,
		CloudInitISOPath: "/var/lib/virtuestack/cloud-init/550e8400.iso",
	}
}

func qcowConfig() *DomainConfig {
	return &DomainConfig{
		VMID:             "660f9511-f30a-52e5-b827-557766551111",
		Hostname:         "test-vm02",
		VCPU:             2,
		MemoryMB:         4096,
		StorageBackend:   StorageBackendQcow,
		DiskPath:         "/var/lib/virtuestack/vms/660f9511-f30a-52e5-b827-557766551111-disk0.qcow2",
		MACAddress:       "52:54:00:dd:ee:ff",
		IPv4Address:      "192.168.1.200",
		IPv6Address:      "",
		PortSpeedKbps:    0,
		BurstKB:          0,
		CloudInitISOPath: "/var/lib/virtuestack/cloud-init/660f9511.iso",
	}
}

func TestGenerateDomainXML_CephBackend(t *testing.T) {
	cfg := cephConfig()
	xml, err := GenerateDomainXML(cfg)
	require.NoError(t, err)

	t.Run("contains domain type kvm", func(t *testing.T) {
		assert.Contains(t, xml, `<domain type='kvm'>`)
	})

	t.Run("contains VM UUID", func(t *testing.T) {
		assert.Contains(t, xml, `<uuid>550e8400-e29b-41d4-a716-446655440000</uuid>`)
	})

	t.Run("domain name is vs- prefix", func(t *testing.T) {
		assert.Contains(t, xml, `<name>vs-550e8400-e29b-41d4-a716-446655440000</name>`)
	})

	t.Run("memory and CPU", func(t *testing.T) {
		assert.Contains(t, xml, `<memory unit='MiB'>8192</memory>`)
		assert.Contains(t, xml, `<vcpu placement='static'>4</vcpu>`)
	})

	t.Run("RBD disk XML", func(t *testing.T) {
		assert.Contains(t, xml, `type='network'`)
		assert.Contains(t, xml, `protocol='rbd'`)
		assert.Contains(t, xml, `name='vs-vms/vs-550e8400-e29b-41d4-a716-446655440000-disk0'`)
		assert.Contains(t, xml, `username='libvirt'`)
		assert.Contains(t, xml, `uuid='a1b2c3d4-e5f6-7890-abcd-ef1234567890'`)
		assert.Contains(t, xml, `host name='10.0.0.1' port='6789'`)
		assert.Contains(t, xml, `host name='10.0.0.2' port='6789'`)
		assert.Contains(t, xml, `type='raw' cache='none' io='native' discard='unmap'`)
		assert.Contains(t, xml, `<target dev='vda' bus='virtio'/>`)
	})

	t.Run("Q35 chipset", func(t *testing.T) {
		assert.Contains(t, xml, `arch='x86_64' machine='q35'`)
	})
}

func TestGenerateDomainXML_QcowBackend(t *testing.T) {
	cfg := qcowConfig()
	xml, err := GenerateDomainXML(cfg)
	require.NoError(t, err)

	t.Run("file-based disk", func(t *testing.T) {
		assert.Contains(t, xml, `type='file' device='disk'`)
		assert.Contains(t, xml, `type='qcow2'`)
		assert.Contains(t, xml, `/var/lib/virtuestack/vms/660f9511-f30a-52e5-b827-557766551111-disk0.qcow2`)
		assert.Contains(t, xml, `<target dev='vda' bus='virtio'/>`)
	})
}

func TestGenerateDomainXML_BandwidthLimits(t *testing.T) {
	t.Run("with bandwidth limit", func(t *testing.T) {
		cfg := cephConfig()
		xml, err := GenerateDomainXML(cfg)
		require.NoError(t, err)

		assert.Contains(t, xml, `<bandwidth>`)
		assert.Contains(t, xml, `average='1024000'`)
		assert.Contains(t, xml, `burst='131072'`)
		assert.Contains(t, xml, `</bandwidth>`)
	})

	t.Run("without bandwidth limit", func(t *testing.T) {
		cfg := qcowConfig()
		xml, err := GenerateDomainXML(cfg)
		require.NoError(t, err)

		assert.NotContains(t, xml, `<bandwidth>`)
	})
}

func TestGenerateDomainXML_Nwfilter(t *testing.T) {
	t.Run("nwfilter with IPv4 and IPv6", func(t *testing.T) {
		cfg := cephConfig()
		xml, err := GenerateDomainXML(cfg)
		require.NoError(t, err)

		assert.Contains(t, xml, `<filterref filter='virtuestack-clean-traffic'>`)
		assert.Contains(t, xml, `<parameter name='IP' value='192.168.1.100'/>`)
		assert.Contains(t, xml, `<parameter name='IPV6' value='2001:db8::1'/>`)
		assert.Contains(t, xml, `<parameter name='MAC' value='52:54:00:aa:bb:cc'/>`)
	})

	t.Run("nwfilter with only IPv4", func(t *testing.T) {
		cfg := qcowConfig()
		xml, err := GenerateDomainXML(cfg)
		require.NoError(t, err)

		assert.Contains(t, xml, `<filterref filter='virtuestack-clean-traffic'>`)
		assert.Contains(t, xml, `<parameter name='IP' value='192.168.1.200'/>`)
		assert.Contains(t, xml, `<parameter name='IPV6' value=''/>`)
	})
}

func TestGenerateDomainXML_CloudInitISO(t *testing.T) {
	cfg := cephConfig()
	xml, err := GenerateDomainXML(cfg)
	require.NoError(t, err)

	assert.Contains(t, xml, `device='cdrom'`)
	assert.Contains(t, xml, `/var/lib/virtuestack/cloud-init/550e8400.iso`)
	assert.Contains(t, xml, `<readonly/>`)
	assert.Contains(t, xml, `<target dev='sda' bus='sata'/>`)
}

func TestGenerateDomainXML_VNCConsole(t *testing.T) {
	cfg := cephConfig()
	xml, err := GenerateDomainXML(cfg)
	require.NoError(t, err)

	assert.Contains(t, xml, `type='vnc' port='-1' autoport='yes' listen='127.0.0.1'`)
	assert.Contains(t, xml, `<listen type='address' address='127.0.0.1'/>`)
}

func TestGenerateDomainXML_SerialConsole(t *testing.T) {
	cfg := cephConfig()
	xml, err := GenerateDomainXML(cfg)
	require.NoError(t, err)

	assert.Contains(t, xml, `<serial type='pty'>`)
	assert.Contains(t, xml, `<console type='pty'>`)
}

func TestGenerateDomainXML_Validation(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*DomainConfig)
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			modify:  func(cfg *DomainConfig) { *cfg = DomainConfig{} },
			wantErr: true,
			errMsg:  "missing required fields",
		},
		{
			name:    "missing VMID",
			modify:  func(cfg *DomainConfig) { cfg.VMID = "" },
			wantErr: true,
			errMsg:  "VMID",
		},
		{
			name:    "missing VCPU",
			modify:  func(cfg *DomainConfig) { cfg.VCPU = 0 },
			wantErr: true,
			errMsg:  "VCPU",
		},
		{
			name:    "missing MemoryMB",
			modify:  func(cfg *DomainConfig) { cfg.MemoryMB = 0 },
			wantErr: true,
			errMsg:  "MemoryMB",
		},
		{
			name:    "missing MACAddress",
			modify:  func(cfg *DomainConfig) { cfg.MACAddress = "" },
			wantErr: true,
			errMsg:  "MACAddress",
		},
		{
			name:    "missing CephPool",
			modify:  func(cfg *DomainConfig) { cfg.CephPool = "" },
			wantErr: true,
			errMsg:  "CephPool",
		},
		{
			name:    "missing CephMonitors",
			modify:  func(cfg *DomainConfig) { cfg.CephMonitors = nil },
			wantErr: true,
			errMsg:  "CephMonitors",
		},
		{
			name:    "missing CephUser",
			modify:  func(cfg *DomainConfig) { cfg.CephUser = "" },
			wantErr: true,
			errMsg:  "CephUser",
		},
		{
			name:    "missing CephSecretUUID",
			modify:  func(cfg *DomainConfig) { cfg.CephSecretUUID = "" },
			wantErr: true,
			errMsg:  "CephSecretUUID",
		},
		{
			name: "missing DiskPath for qcow",
			modify: func(cfg *DomainConfig) {
				cfg.StorageBackend = StorageBackendQcow
				cfg.DiskPath = ""
			},
			wantErr: true,
			errMsg:  "DiskPath",
		},
		{
			name:    "unsupported backend",
			modify:  func(cfg *DomainConfig) { cfg.StorageBackend = "zfs" },
			wantErr: true,
			errMsg:  "unsupported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := cephConfig()
			tt.modify(cfg)
			_, err := GenerateDomainXML(cfg)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDomainNameFromID(t *testing.T) {
	result := DomainNameFromID("test-vm-id")
	assert.Equal(t, "vs-test-vm-id", result)
}

func TestGenerateDomainXML_ValidXML(t *testing.T) {
	cfg := cephConfig()
	xml, err := GenerateDomainXML(cfg)
	require.NoError(t, err)

	assert.True(t, strings.Contains(xml, `<domain`), "should contain domain element")
	assert.True(t, strings.Contains(xml, `</domain>`))
	assert.True(t, strings.HasSuffix(strings.TrimSpace(xml), `</domain>`))
}

func TestGenerateDomainXML_GuestAgent(t *testing.T) {
	cfg := cephConfig()
	xml, err := GenerateDomainXML(cfg)
	require.NoError(t, err)

	assert.Contains(t, xml, `org.qemu.guest_agent.0`)
}

func TestGenerateDomainXML_VirtioDevices(t *testing.T) {
	cfg := cephConfig()
	xml, err := GenerateDomainXML(cfg)
	require.NoError(t, err)

	assert.Contains(t, xml, `<model type='virtio'/>`)
	assert.Contains(t, xml, `<rng model='virtio'>`)
	assert.Contains(t, xml, `<memballoon model='virtio'>`)
}

func TestGenerateDomainXML_DefaultCephBackend(t *testing.T) {
	cfg := cephConfig()
	cfg.StorageBackend = ""
	xml, err := GenerateDomainXML(cfg)
	require.NoError(t, err)

	assert.Contains(t, xml, `protocol='rbd'`)
}
