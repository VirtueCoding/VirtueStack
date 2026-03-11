package vm

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// DomainConfig contains all parameters needed to generate a libvirt domain XML.
type DomainConfig struct {
	// VMID is the unique identifier for the virtual machine.
	VMID string
	// Hostname is the VM's hostname.
	Hostname string
	// VCPU is the number of virtual CPUs to allocate.
	VCPU int
	// MemoryMB is the amount of memory in megabytes.
	MemoryMB int
	// CephPool is the Ceph pool name for the VM disk (e.g., "vs-vms").
	CephPool string
	// CephMonitors is the list of Ceph monitor addresses.
	CephMonitors []string
	// CephUser is the Ceph user name for authentication.
	CephUser string
	// CephSecretUUID is the UUID of the Ceph secret for authentication.
	CephSecretUUID string
	// MACAddress is the MAC address for the primary network interface.
	MACAddress string
	// IPv4Address is the IPv4 address for the VM (empty for DHCP).
	IPv4Address string
	// IPv6Address is the IPv6 address for the VM (empty for SLAAC/DHCPv6).
	IPv6Address string
	// PortSpeedKbps is the port speed limit in kilobits per second.
	PortSpeedKbps int
	// BurstKB is the burst bandwidth in kilobytes.
	BurstKB int
	// CloudInitISOPath is the path to the cloud-init ISO file.
	CloudInitISOPath string
}

// CreateResult contains the result of a VM creation operation.
type CreateResult struct {
	// DomainName is the name assigned by libvirt to the domain.
	DomainName string
	// VNCPort is the VNC port assigned for console access.
	VNCPort int32
}

// domainXMLTemplate is the libvirt domain XML template for VirtueStack VMs.
const domainXMLTemplate = `<domain type='kvm'>
  <name>{{.DomainName}}</name>
  <uuid>{{.VMID}}</uuid>
  <memory unit='MiB'>{{.MemoryMB}}</memory>
  <currentMemory unit='MiB'>{{.MemoryMB}}</currentMemory>
  <vcpu placement='static'>{{.VCPU}}</vcpu>
  <os>
    <type arch='x86_64' machine='q35'>hvm</type>
    <boot dev='hd'/>
    <boot dev='cdrom'/>
  </os>
  <features>
    <acpi/>
    <apic/>
  </features>
  <cpu mode='host-model' check='partial'/>
  <clock offset='utc'>
    <timer name='rtc' tickpolicy='catchup'/>
    <timer name='pit' tickpolicy='delay'/>
    <timer name='hpet' present='no'/>
  </clock>
  <on_poweroff>destroy</on_poweroff>
  <on_reboot>restart</on_reboot>
  <on_crash>destroy</on_crash>
  <pm>
    <suspend-to-mem enabled='no'/>
    <suspend-to-disk enabled='no'/>
  </pm>
  <devices>
    <emulator>/usr/bin/qemu-system-x86_64</emulator>
    <disk type='network' device='disk'>
      <driver name='qemu' type='raw' cache='none' io='native' discard='unmap'/>
      <auth username='{{.CephUser}}'>
        <secret type='ceph' uuid='{{.CephSecretUUID}}'/>
      </auth>
      <source protocol='rbd' name='{{.CephPool}}/{{.DiskName}}'>
        {{range .CephMonitors}}
        <host name='{{.}}' port='6789'/>
        {{end}}
      </source>
      <target dev='vda' bus='virtio'/>
    </disk>
    <disk type='file' device='cdrom'>
      <driver name='qemu' type='raw'/>
      <source file='{{.CloudInitISOPath}}'/>
      <target dev='sda' bus='sata'/>
      <readonly/>
    </disk>
    <controller type='usb' index='0' model='qemu-xhci' ports='15'/>
    <controller type='sata' index='0'/>
    <controller type='pci' index='0' model='pcie-root'/>
    <controller type='pci' index='1' model='pcie-root-port'>
      <model name='pcie-root-port'/>
      <target chassis='1' port='0x10'/>
      <address type='pci' domain='0x0000' bus='0x00' slot='0x02' function='0x0'/>
    </controller>
    <controller type='pci' index='2' model='pcie-root-port'>
      <model name='pcie-root-port'/>
      <target chassis='2' port='0x11'/>
      <address type='pci' domain='0x0000' bus='0x00' slot='0x03' function='0x0'/>
    </controller>
    <controller type='pci' index='3' model='pcie-root-port'>
      <model name='pcie-root-port'/>
      <target chassis='3' port='0x12'/>
      <address type='pci' domain='0x0000' bus='0x00' slot='0x04' function='0x0'/>
    </controller>
    <controller type='pci' index='4' model='pcie-root-port'>
      <model name='pcie-root-port'/>
      <target chassis='4' port='0x13'/>
      <address type='pci' domain='0x0000' bus='0x00' slot='0x05' function='0x0'/>
    </controller>
    <controller type='pci' index='5' model='pcie-root-port'>
      <model name='pcie-root-port'/>
      <target chassis='5' port='0x14'/>
      <address type='pci' domain='0x0000' bus='0x00' slot='0x06' function='0x0'/>
    </controller>
    <controller type='pci' index='6' model='pcie-root-port'>
      <model name='pcie-root-port'/>
      <target chassis='6' port='0x15'/>
      <address type='pci' domain='0x0000' bus='0x00' slot='0x07' function='0x0'/>
    </controller>
    <interface type='bridge'>
      <mac address='{{.MACAddress}}'/>
      <source bridge='br0'/>
      <model type='virtio'/>
      <driver name='vhost'/>
      {{if .HasBandwidthLimit}}
      <bandwidth>
        <inbound average='{{.PortSpeedKbps}}' burst='{{.BurstKB}}'/>
        <outbound average='{{.PortSpeedKbps}}' burst='{{.BurstKB}}'/>
      </bandwidth>
      {{end}}
      <filterref filter='virtuestack-clean-traffic'>
        <parameter name='IP' value='{{.IPv4Address}}'/>
        <parameter name='IPV6' value='{{.IPv6Address}}'/>
        <parameter name='MAC' value='{{.MACAddress}}'/>
      </filterref>
      <address type='pci' domain='0x0000' bus='0x01' slot='0x00' function='0x0'/>
    </interface>
    <channel type='unix'>
      <target type='virtio' name='org.qemu.guest_agent.0'/>
      <address type='virtio-serial' controller='0' port='1'/>
    </channel>
    <input type='tablet' bus='usb'>
      <address type='usb' bus='0' port='1'/>
    </input>
    <input type='mouse' bus='ps2'/>
    <input type='keyboard' bus='ps2'/>
    <graphics type='vnc' port='-1' autoport='yes' listen='127.0.0.1'>
      <listen type='address' address='127.0.0.1'/>
    </graphics>
    <video>
      <model type='qxl' ram='65536' vram='65536' vgamem='16384' heads='1' primary='yes'/>
      <address type='pci' domain='0x0000' bus='0x00' slot='0x01' function='0x0'/>
    </video>
    <serial type='pty'>
      <target type='isa-serial' port='0'>
        <model name='isa-serial'/>
      </target>
    </serial>
    <console type='pty'>
      <target type='serial' port='0'/>
    </console>
    <rng model='virtio'>
      <backend model='random'>/dev/urandom</backend>
      <address type='pci' domain='0x0000' bus='0x02' slot='0x00' function='0x0'/>
    </rng>
    <memballoon model='virtio'>
      <address type='pci' domain='0x0000' bus='0x03' slot='0x00' function='0x0'/>
    </memballoon>
  </devices>
</domain>`

// templateData holds the data for domain XML template execution.
type templateData struct {
	DomainName       string
	VMID             string
	MemoryMB         int
	VCPU             int
	CephUser         string
	CephSecretUUID   string
	CephPool         string
	DiskName         string
	CephMonitors     []string
	MACAddress       string
	IPv4Address      string
	IPv6Address      string
	PortSpeedKbps    int
	BurstKB          int
	HasBandwidthLimit bool
	CloudInitISOPath string
}

// GenerateDomainXML generates a libvirt domain XML from the given configuration.
// It validates required fields before template execution.
func GenerateDomainXML(cfg *DomainConfig) (string, error) {
	if err := validateDomainConfig(cfg); err != nil {
		return "", fmt.Errorf("validating domain config: %w", err)
	}

	tmpl, err := template.New("domain").Parse(domainXMLTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing domain template: %w", err)
	}

	data := templateData{
		DomainName:       domainName(cfg.VMID),
		VMID:             cfg.VMID,
		MemoryMB:         cfg.MemoryMB,
		VCPU:             cfg.VCPU,
		CephUser:         cfg.CephUser,
		CephSecretUUID:   cfg.CephSecretUUID,
		CephPool:         cfg.CephPool,
		DiskName:         diskName(cfg.VMID),
		CephMonitors:     cfg.CephMonitors,
		MACAddress:       cfg.MACAddress,
		IPv4Address:      cfg.IPv4Address,
		IPv6Address:      cfg.IPv6Address,
		PortSpeedKbps:    cfg.PortSpeedKbps,
		BurstKB:          cfg.BurstKB,
		HasBandwidthLimit: cfg.PortSpeedKbps > 0,
		CloudInitISOPath: cfg.CloudInitISOPath,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing domain template: %w", err)
	}

	return buf.String(), nil
}

// validateDomainConfig validates that all required fields are set.
func validateDomainConfig(cfg *DomainConfig) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	var missing []string

	if cfg.VMID == "" {
		missing = append(missing, "VMID")
	}
	if cfg.VCPU <= 0 {
		missing = append(missing, "VCPU")
	}
	if cfg.MemoryMB <= 0 {
		missing = append(missing, "MemoryMB")
	}
	if cfg.CephPool == "" {
		missing = append(missing, "CephPool")
	}
	if len(cfg.CephMonitors) == 0 {
		missing = append(missing, "CephMonitors")
	}
	if cfg.CephUser == "" {
		missing = append(missing, "CephUser")
	}
	if cfg.CephSecretUUID == "" {
		missing = append(missing, "CephSecretUUID")
	}
	if cfg.MACAddress == "" {
		missing = append(missing, "MACAddress")
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
	}

	return nil
}

// domainName returns the libvirt domain name for a VM.
func domainName(vmID string) string {
	return "vs-" + vmID
}

// diskName returns the RBD disk name for a VM.
func diskName(vmID string) string {
	return "vs-" + vmID + "-disk0"
}

// DomainNameFromID converts a VM ID to its libvirt domain name.
func DomainNameFromID(vmID string) string {
	return domainName(vmID)
}