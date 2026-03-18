// Package libvirtutil provides shared utilities for libvirt operations.
package libvirtutil

import (
	"encoding/xml"

	"libvirt.org/go/libvirt"
)

// DomainInterfacesDef is an XML unmarshalling structure for extracting
// network interface names from a libvirt domain XML document.
// It is used by multiple packages to parse interface target device names.
type DomainInterfacesDef struct {
	Devices struct {
		Interfaces []struct {
			Target struct {
				Dev string `xml:"dev,attr"`
			} `xml:"target"`
		} `xml:"interface"`
	} `xml:"devices"`
}

// ParseDomainInterfaces extracts interface target device names from domain XML.
// Returns a slice of interface names (e.g., ["vnet0", "vnet1"]).
func ParseDomainInterfaces(xmlDesc string) (*DomainInterfacesDef, error) {
	var domainDef DomainInterfacesDef
	if err := xml.Unmarshal([]byte(xmlDesc), &domainDef); err != nil {
		return nil, err
	}
	return &domainDef, nil
}

// GetInterfaceNames extracts all interface target device names from domain XML.
// Returns a slice of interface names (e.g., ["vnet0", "vnet1"]).
func GetInterfaceNames(xmlDesc string) ([]string, error) {
	domainDef, err := ParseDomainInterfaces(xmlDesc)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, iface := range domainDef.Devices.Interfaces {
		if iface.Target.Dev != "" {
			names = append(names, iface.Target.Dev)
		}
	}
	return names, nil
}

// IsLibvirtError checks if an error is a specific libvirt error code.
// Use this to detect specific error conditions like ERR_NO_DOMAIN.
func IsLibvirtError(err error, code libvirt.ErrorNumber) bool {
	if err == nil {
		return false
	}
	if lerr, ok := err.(libvirt.Error); ok {
		return lerr.Code == code
	}
	return false
}