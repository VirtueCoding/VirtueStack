// Package models provides data model types for VirtueStack Controller.
package models

import "time"

// IPSet represents a pool of IP addresses associated with a network location.
type IPSet struct {
	ID         string    `json:"id" db:"id"`
	Name       string    `json:"name" db:"name"`
	LocationID *string   `json:"location_id,omitempty" db:"location_id"`
	Network    string    `json:"network" db:"network"`
	Gateway    string    `json:"gateway" db:"gateway"`
	VLANID     *int      `json:"vlan_id,omitempty" db:"vlan_id"`
	IPVersion  int16     `json:"ip_version" db:"ip_version"`
	NodeIDs    []string  `json:"node_ids,omitempty" db:"node_ids"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}

// IPAddress status constants define the allocation lifecycle of an IP address.
const (
	IPStatusAvailable = "available"
	IPStatusAssigned  = "assigned"
	IPStatusReserved  = "reserved"
	IPStatusCooldown  = "cooldown"
)

// IPAddress represents an individual IP address within an IP set.
type IPAddress struct {
	ID            string     `json:"id" db:"id"`
	IPSetID       string     `json:"ip_set_id" db:"ip_set_id"`
	Address       string     `json:"address" db:"address"`
	IPVersion     int16      `json:"ip_version" db:"ip_version"`
	VMID          *string    `json:"vm_id,omitempty" db:"vm_id"`
	CustomerID    *string    `json:"customer_id,omitempty" db:"customer_id"`
	IsPrimary     bool       `json:"is_primary" db:"is_primary"`
	RDNSHostname  *string    `json:"rdns_hostname,omitempty" db:"rdns_hostname"`
	Status        string     `json:"status" db:"status"`
	AssignedAt    *time.Time `json:"assigned_at,omitempty" db:"assigned_at"`
	ReleasedAt    *time.Time `json:"released_at,omitempty" db:"released_at"`
	CooldownUntil *time.Time `json:"cooldown_until,omitempty" db:"cooldown_until"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
}

// IPv6Prefix represents a /48 or larger IPv6 prefix allocated to a node.
type IPv6Prefix struct {
	ID        string    `json:"id" db:"id"`
	NodeID    string    `json:"node_id" db:"node_id"`
	Prefix    string    `json:"prefix" db:"prefix"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// VMIPv6Subnet represents a /64 IPv6 subnet assigned to a virtual machine.
type VMIPv6Subnet struct {
	ID           string    `json:"id" db:"id"`
	VMID         string    `json:"vm_id" db:"vm_id"`
	IPv6PrefixID string    `json:"ipv6_prefix_id" db:"ipv6_prefix_id"`
	Subnet       string    `json:"subnet" db:"subnet"`
	SubnetIndex  int       `json:"subnet_index" db:"subnet_index"`
	Gateway      string    `json:"gateway" db:"gateway"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// IPSetCreateRequest holds the fields required to create a new IP set.
type IPSetCreateRequest struct {
	Name       string   `json:"name" validate:"required,max=100"`
	LocationID *string  `json:"location_id,omitempty" validate:"omitempty,uuid"`
	Network    string   `json:"network" validate:"required,cidr"`
	Gateway    string   `json:"gateway" validate:"required,ip"`
	VlanID     *int     `json:"vlan_id,omitempty" validate:"omitempty,min=1,max=4094"`
	IPVersion  int      `json:"ip_version" validate:"required,oneof=4 6"`
	NodeIDs    []string `json:"node_ids,omitempty" validate:"dive,uuid"`
}

// IPImportRequest holds the fields required to bulk-import IP addresses into an existing IP set.
type IPImportRequest struct {
	IPSetID   string   `json:"ip_set_id" validate:"required,uuid"`
	Addresses []string `json:"addresses" validate:"required,min=1,dive,ip"`
}
