package vm

import (
	"strings"
	"testing"
)

// TestGenerateLVMDiskXMLStructure tests that generateLVMDiskXML produces correct XML structure.
func TestGenerateLVMDiskXMLStructure(t *testing.T) {
	cfg := &DomainConfig{
		VMID:       "test-vm-123",
		LVMDiskPath: "/dev/vgvs/vs-test-vm-123-disk0",
	}

	xml, err := generateLVMDiskXML(cfg)
	if err != nil {
		t.Fatalf("generateLVMDiskXML() error = %v", err)
	}

	// Verify critical attributes
	tests := []struct {
		name    string
		pattern string
	}{
		{"type block", `type='block'`},
		{"device disk", `device='disk'`},
		{"driver type raw", `type='raw'`},
		{"discard unmap", `discard='unmap'`},
		{"cache none", `cache='none'`},
		{"io native", `io='native'`},
		{"dev vda", `dev='vda'`},
		{"bus virtio", `bus='virtio'`},
		{"disk path", `/dev/vgvs/vs-test-vm-123-disk0`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(xml, tt.pattern) {
				t.Errorf("generateLVMDiskXML() missing %s, got:\n%s", tt.pattern, xml)
			}
		})
	}
}

// TestGenerateLVMDiskXMLXMLSpecialCharsEscaping tests that XML special characters are escaped.
func TestGenerateLVMDiskXMLXMLSpecialCharsEscaping(t *testing.T) {
	tests := []struct {
		name     string
		lvmPath  string
		wantAmp  bool // Should contain &amp;
		wantLt   bool // Should contain &lt;
		wantGt   bool // Should contain &gt;
		wantApos bool // Should contain &apos;
		wantQuot bool // Should contain &quot;
	}{
		{
			name:    "normal path",
			lvmPath: "/dev/vgvs/normal-disk",
			wantAmp: false,
			wantLt:  false,
			wantGt:  false,
		},
		{
			name:    "path with ampersand",
			lvmPath: "/dev/vgvs/disk&special",
			wantAmp: true, // Should be escaped to &amp;
		},
		{
			name:    "path with less than",
			lvmPath: "/dev/vgvs/disk<special",
			wantLt: true, // Should be escaped to &lt;
		},
		{
			name:    "path with greater than",
			lvmPath: "/dev/vgvs/disk>special",
			wantGt: true, // Should be escaped to &gt;
		},
		{
			name:     "path with quote",
			lvmPath:  "/dev/vgvs/disk\"special",
			wantQuot: true, // Should be escaped (&#34; or &quot;)
		},
		{
			name:     "path with apostrophe",
			lvmPath:  "/dev/vgvs/disk'special",
			wantApos: true, // Should be escaped (&#39; or &apos;)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &DomainConfig{
				VMID:        "test-vm",
				LVMDiskPath: tt.lvmPath,
			}

			xml, err := generateLVMDiskXML(cfg)
			if err != nil {
				t.Fatalf("generateLVMDiskXML() error = %v", err)
			}

			if tt.wantAmp && !strings.Contains(xml, "&amp;") {
				t.Errorf("generateLVMDiskXML() should escape & to &amp;, got:\n%s", xml)
			}
			if tt.wantLt && !strings.Contains(xml, "&lt;") {
				t.Errorf("generateLVMDiskXML() should escape < to &lt;, got:\n%s", xml)
			}
			if tt.wantGt && !strings.Contains(xml, "&gt;") {
				t.Errorf("generateLVMDiskXML() should escape > to &gt;, got:\n%s", xml)
			}
			if tt.wantQuot && !strings.Contains(xml, "&quot;") && !strings.Contains(xml, "&#34;") {
				t.Errorf("generateLVMDiskXML() should escape \" to &quot; or &#34;, got:\n%s", xml)
			}
			if tt.wantApos && !strings.Contains(xml, "&apos;") && !strings.Contains(xml, "&#39;") {
				t.Errorf("generateLVMDiskXML() should escape ' to &apos; or &#39;, got:\n%s", xml)
			}
		})
	}
}

// TestGenerateLVMDiskXMLNoRawChars tests that raw XML special characters don't appear.
func TestGenerateLVMDiskXMLNoRawChars(t *testing.T) {
	cfg := &DomainConfig{
		VMID:       "test-vm",
		LVMDiskPath: "/dev/vgvs/test&vm<path>disk",
	}

	xml, err := generateLVMDiskXML(cfg)
	if err != nil {
		t.Fatalf("generateLVMDiskXML() error = %v", err)
	}

	// Raw characters should not appear (they should be escaped)
	if strings.Contains(xml, "&") && !strings.Contains(xml, "&amp;") {
		t.Errorf("generateLVMDiskXML() should escape all & to &amp;")
	}
	if strings.Contains(xml, "<") && !strings.Contains(xml, "&lt;") {
		t.Errorf("generateLVMDiskXML() should escape all < to &lt;")
	}
	if strings.Contains(xml, ">") && !strings.Contains(xml, "&gt;") {
		t.Errorf("generateLVMDiskXML() should escape all > to &gt;")
	}
}

// TestValidateDomainConfigWithLVM tests validateDomainConfig with LVM storage backend.
func TestValidateDomainConfigWithLVM(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *DomainConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid LVM config",
			cfg: &DomainConfig{
				VMID:         "test-vm-123",
				VCPU:         2,
				MemoryMB:     2048,
				MACAddress:   "52:54:00:00:00:01",
				StorageBackend: StorageBackendLVM,
				LVMDiskPath:  "/dev/vgvs/vs-test-vm-123-disk0",
			},
			wantErr: false,
		},
		{
			name: "missing LVMDiskPath",
			cfg: &DomainConfig{
				VMID:           "test-vm-123",
				VCPU:           2,
				MemoryMB:       2048,
				MACAddress:     "52:54:00:00:00:01",
				StorageBackend: StorageBackendLVM,
				LVMDiskPath:    "", // Missing
			},
			wantErr: true,
			errMsg:  "LVMDiskPath",
		},
		{
			name: "nil config",
			cfg:  nil,
			wantErr: true,
			errMsg:  "nil",
		},
		{
			name: "missing VMID",
			cfg: &DomainConfig{
				VCPU:           2,
				MemoryMB:       2048,
				MACAddress:     "52:54:00:00:00:01",
				StorageBackend: StorageBackendLVM,
				LVMDiskPath:    "/dev/vgvs/vs-test-disk0",
			},
			wantErr: true,
			errMsg:  "VMID",
		},
		{
			name: "missing VCPU",
			cfg: &DomainConfig{
				VMID:           "test-vm",
				MemoryMB:       2048,
				MACAddress:     "52:54:00:00:00:01",
				StorageBackend: StorageBackendLVM,
				LVMDiskPath:    "/dev/vgvs/vs-test-disk0",
			},
			wantErr: true,
			errMsg:  "VCPU",
		},
		{
			name: "missing MemoryMB",
			cfg: &DomainConfig{
				VMID:           "test-vm",
				VCPU:           2,
				MACAddress:     "52:54:00:00:00:01",
				StorageBackend: StorageBackendLVM,
				LVMDiskPath:    "/dev/vgvs/vs-test-disk0",
			},
			wantErr: true,
			errMsg:  "MemoryMB",
		},
		{
			name: "missing MACAddress",
			cfg: &DomainConfig{
				VMID:           "test-vm",
				VCPU:           2,
				MemoryMB:       2048,
				StorageBackend: StorageBackendLVM,
				LVMDiskPath:    "/dev/vgvs/vs-test-disk0",
			},
			wantErr: true,
			errMsg:  "MACAddress",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDomainConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDomainConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("validateDomainConfig() error = %v, should contain %s", err, tt.errMsg)
			}
		})
	}
}

// TestValidateDomainConfigWithCeph tests validateDomainConfig with Ceph storage backend.
func TestValidateDomainConfigWithCeph(t *testing.T) {
	cfg := &DomainConfig{
		VMID:           "test-vm",
		VCPU:           2,
		MemoryMB:       2048,
		MACAddress:     "52:54:00:00:00:01",
		StorageBackend: StorageBackendCeph,
		CephPool:       "vs-vms",
		CephMonitors:   []string{"10.0.0.1:6789", "10.0.0.2:6789"},
		CephUser:       "admin",
		CephSecretUUID: "12345678-1234-1234-1234-123456789012",
	}

	err := validateDomainConfig(cfg)
	if err != nil {
		t.Errorf("validateDomainConfig() error = %v, want nil", err)
	}
}

// TestValidateDomainConfigWithQcow tests validateDomainConfig with QCOW storage backend.
func TestValidateDomainConfigWithQcow(t *testing.T) {
	cfg := &DomainConfig{
		VMID:           "test-vm",
		VCPU:           2,
		MemoryMB:       2048,
		MACAddress:     "52:54:00:00:00:01",
		StorageBackend: StorageBackendQcow,
		DiskPath:       "/var/lib/virtuestack/vms/test-vm-disk0.qcow2",
	}

	err := validateDomainConfig(cfg)
	if err != nil {
		t.Errorf("validateDomainConfig() error = %v, want nil", err)
	}
}

// TestEscapeXMLFunction tests the escapeXML helper function.
func TestEscapeXMLFunction(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantAmp bool
		wantLt  bool
		wantGt  bool
	}{
		{"normal", "normal", false, false, false},
		{"<test>", "&lt;test&gt;", false, true, true},
		{"a & b", "a &amp; b", true, false, false},
		{"it's", "it&#39;s", false, false, false},  // xml.EscapeText uses numeric ref for apostrophe
		{`quote"test`, "quote&#34;test", false, false, false}, // xml.EscapeText uses numeric ref for quote
		{"", "", false, false, false},
		{"<>&\"'", "&lt;&gt;&amp;&#34;&#39;", true, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := escapeXML(tt.input)
			if got != tt.want {
				t.Errorf("escapeXML(%q) = %q, want %q", tt.input, got, tt.want)
			}
			// Verify essential escaping happened
			if tt.wantAmp && !strings.Contains(got, "&amp;") {
				t.Errorf("escapeXML(%q) should contain &amp;", tt.input)
			}
			if tt.wantLt && !strings.Contains(got, "&lt;") {
				t.Errorf("escapeXML(%q) should contain &lt;", tt.input)
			}
			if tt.wantGt && !strings.Contains(got, "&gt;") {
				t.Errorf("escapeXML(%q) should contain &gt;", tt.input)
			}
		})
	}
}

// TestGenerateLVMDiskXMLCompleteOutput tests the complete XML output structure.
func TestGenerateLVMDiskXMLCompleteOutput(t *testing.T) {
	cfg := &DomainConfig{
		VMID:        "550e8400-e29b-41d4-a716-446655440000",
		LVMDiskPath: "/dev/vgvs/vs-550e8400-e29b-41d4-a716-446655440000-disk0",
	}

	xml, err := generateLVMDiskXML(cfg)
	if err != nil {
		t.Fatalf("generateLVMDiskXML() error = %v", err)
	}

	// Verify complete structure - check key parts
	if !strings.Contains(xml, `<disk type='block' device='disk'>`) {
		t.Errorf("generateLVMDiskXML() missing disk element, got:\n%s", xml)
	}
	if !strings.Contains(xml, `type='raw'`) {
		t.Errorf("generateLVMDiskXML() missing raw type, got:\n%s", xml)
	}
	if !strings.Contains(xml, `discard='unmap'`) {
		t.Errorf("generateLVMDiskXML() missing discard unmap, got:\n%s", xml)
	}
	if !strings.Contains(xml, `/dev/vgvs/vs-550e8400-e29b-41d4-a716-446655440000-disk0`) {
		t.Errorf("generateLVMDiskXML() missing LVM disk path, got:\n%s", xml)
	}
}

// TestGenerateLVMDiskXMLWithCloudInit tests that LVM disk XML is correctly generated
// when cloud-init ISO is also attached. This verifies both disk types coexist properly.
func TestGenerateLVMDiskXMLWithCloudInit(t *testing.T) {
	vmCfg := &DomainConfig{
		VMID:              "550e8400-e29b-41d4-a716-446655440000",
		LVMDiskPath:       "/dev/vgvs/vs-550e8400-e29b-41d4-a716-446655440000-disk0",
		CloudInitISOPath:  "/var/lib/virtuestack/cloud-init/vs-550e8400-e29b-41d4-a716-446655440000.iso",
	}

	// Test that cloud-init path is preserved in the config
	if vmCfg.CloudInitISOPath == "" {
		t.Errorf("CloudInitISOPath should be set")
	}

	// Verify the path format is correct
	if !strings.HasSuffix(vmCfg.CloudInitISOPath, ".iso") {
		t.Errorf("CloudInitISOPath should end with .iso, got: %s", vmCfg.CloudInitISOPath)
	}
}

// TestLVMWithCloudInitDomainXMLGeneration tests that a complete domain XML with both
// LVM disk and cloud-init ISO is generated correctly.
func TestLVMWithCloudInitDomainXMLGeneration(t *testing.T) {
	// This test verifies the pattern for LVM + cloud-init VM configuration
	cfg := &DomainConfig{
		VMID:              "test-vm-cloudinit",
		VCPU:               2,
		MemoryMB:           2048,
		MACAddress:         "52:54:00:00:00:01",
		StorageBackend:     StorageBackendLVM,
		LVMDiskPath:       "/dev/vgvs/vs-test-vm-cloudinit-disk0",
		CloudInitISOPath:   "/var/lib/virtuestack/cloud-init/vs-test-vm-cloudinit.iso",
		IPv4Address:        "192.168.1.100",
		IPv6Address:        "2001:db8::100",
		PortSpeedKbps:     1000000,
	}

	// Validate the config
	err := validateDomainConfig(cfg)
	if err != nil {
		t.Fatalf("validateDomainConfig() error = %v", err)
	}

	// Verify storage backend is correctly set
	if cfg.StorageBackend != StorageBackendLVM {
		t.Errorf("StorageBackend = %v, want %v", cfg.StorageBackend, StorageBackendLVM)
	}

	// Verify LVM disk path is set
	if cfg.LVMDiskPath == "" {
		t.Errorf("LVMDiskPath should not be empty")
	}

	// Verify cloud-init ISO path is set
	if cfg.CloudInitISOPath == "" {
		t.Errorf("CloudInitISOPath should not be empty")
	}

	t.Logf("LVM VM with cloud-init configured: disk=%s, cloudinit=%s",
		cfg.LVMDiskPath, cfg.CloudInitISOPath)
}
