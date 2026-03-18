package libvirtutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"libvirt.org/go/libvirt"
)

func TestParseDomainInterfaces(t *testing.T) {
	tests := []struct {
		name        string
		xmlDesc     string
		wantNames   []string
		wantErr     bool
		errContains string
	}{
		{
			name: "single interface",
			xmlDesc: `<domain>
				<devices>
					<interface>
						<target dev="vnet0"/>
					</interface>
				</devices>
			</domain>`,
			wantNames: []string{"vnet0"},
			wantErr:   false,
		},
		{
			name: "multiple interfaces",
			xmlDesc: `<domain>
				<devices>
					<interface>
						<target dev="vnet0"/>
					</interface>
					<interface>
						<target dev="vnet1"/>
					</interface>
				</devices>
			</domain>`,
			wantNames: []string{"vnet0", "vnet1"},
			wantErr:   false,
		},
		{
			name: "no interfaces",
			xmlDesc: `<domain>
				<devices>
				</devices>
			</domain>`,
			wantNames: nil,
			wantErr:   false,
		},
		{
			name:        "invalid XML",
			xmlDesc:     `<domain><invalid`,
			wantErr:     true,
			errContains: "XML",
		},
		{
			name: "interface with empty dev",
			xmlDesc: `<domain>
				<devices>
					<interface>
						<target dev=""/>
					</interface>
					<interface>
						<target dev="vnet0"/>
					</interface>
				</devices>
			</domain>`,
			wantNames: []string{"vnet0"},
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			domainDef, err := ParseDomainInterfaces(tt.xmlDesc)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
			require.NotNil(t, domainDef)

			// Collect names from parsed result
			var gotNames []string
			for _, iface := range domainDef.Devices.Interfaces {
				if iface.Target.Dev != "" {
					gotNames = append(gotNames, iface.Target.Dev)
				}
			}
			if tt.wantNames == nil {
				assert.Nil(t, gotNames)
			} else {
				assert.Equal(t, tt.wantNames, gotNames)
			}
		})
	}
}

func TestGetInterfaceNames(t *testing.T) {
	tests := []struct {
		name      string
		xmlDesc   string
		wantNames []string
		wantErr   bool
	}{
		{
			name: "single interface",
			xmlDesc: `<domain>
				<devices>
					<interface>
						<target dev="vnet0"/>
					</interface>
				</devices>
			</domain>`,
			wantNames: []string{"vnet0"},
			wantErr:   false,
		},
		{
			name: "multiple interfaces",
			xmlDesc: `<domain>
				<devices>
					<interface>
						<target dev="vnet0"/>
					</interface>
					<interface>
						<target dev="vnet1"/>
					</interface>
					<interface>
						<target dev="vnet2"/>
					</interface>
				</devices>
			</domain>`,
			wantNames: []string{"vnet0", "vnet1", "vnet2"},
			wantErr:   false,
		},
		{
			name: "no interfaces",
			xmlDesc: `<domain>
				<devices>
				</devices>
			</domain>`,
			wantNames: nil,
			wantErr:   false,
		},
		{
			name:      "invalid XML",
			xmlDesc:   `<invalid`,
			wantNames: nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			names, err := GetInterfaceNames(tt.xmlDesc)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantNames, names)
		})
	}
}

func TestIsLibvirtError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		code     libvirt.ErrorNumber
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			code:     libvirt.ERR_NO_DOMAIN,
			expected: false,
		},
		{
			name:     "matching error code",
			err:      libvirt.Error{Code: libvirt.ERR_NO_DOMAIN},
			code:     libvirt.ERR_NO_DOMAIN,
			expected: true,
		},
		{
			name:     "non-matching error code",
			err:      libvirt.Error{Code: libvirt.ERR_NO_DOMAIN},
			code:     libvirt.ERR_NO_NWFILTER,
			expected: false,
		},
		{
			name:     "non-libvirt error",
			err:      assert.AnError,
			code:     libvirt.ERR_NO_DOMAIN,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsLibvirtError(tt.err, tt.code)
			assert.Equal(t, tt.expected, result)
		})
	}
}