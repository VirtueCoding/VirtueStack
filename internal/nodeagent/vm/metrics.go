// Package vm provides VM lifecycle management for the VirtueStack Node Agent.
// It handles VM creation, start, stop, deletion, and metrics collection via libvirt.
package vm

import "libvirt.org/go/libvirt"

// VMStatus represents the current operational state of a virtual machine.
type VMStatus struct {
	// VMID is the unique identifier of the virtual machine.
	VMID string
	// Status is the current operational state as a string.
	// Values: "running", "stopped", "paused", "shutting_down", "crashed", "unknown".
	Status string
	// VCPU is the number of allocated virtual CPUs.
	VCPU int32
	// MemoryMB is the allocated memory in megabytes.
	MemoryMB int32
	// UptimeSeconds is the VM uptime in seconds (0 if stopped).
	UptimeSeconds int64
}

// VMMetrics contains real-time resource utilization metrics for a VM.
type VMMetrics struct {
	// VMID is the unique identifier of the virtual machine.
	VMID string
	// CPUUsagePercent is the current CPU utilization as a percentage (0-100).
	CPUUsagePercent float64
	// MemoryUsageBytes is the current memory usage in bytes.
	MemoryUsageBytes int64
	// MemoryTotalBytes is the total allocated memory in bytes.
	MemoryTotalBytes int64
	// DiskReadBytes is the total bytes read from disk since VM start.
	DiskReadBytes int64
	// DiskWriteBytes is the total bytes written to disk since VM start.
	DiskWriteBytes int64
	// NetworkRXBytes is the total bytes received on network interfaces since VM start.
	NetworkRXBytes int64
	// NetworkTXBytes is the total bytes transmitted on network interfaces since VM start.
	NetworkTXBytes int64
	DiskReadOps    int64
	DiskWriteOps   int64
	NetworkRXPkts  int64
	NetworkTXPkts  int64
	NetworkRXErrs  int64
	NetworkTXErrs  int64
	NetworkRXDrop  int64
	NetworkTXDrop  int64
}

// NodeResources contains aggregate resource information for a compute node.
type NodeResources struct {
	// TotalVCPU is the total vCPUs available on the node.
	TotalVCPU int32
	// UsedVCPU is the vCPUs currently allocated to VMs.
	UsedVCPU int32
	// TotalMemoryMB is the total memory available in megabytes.
	TotalMemoryMB int64
	// UsedMemoryMB is the memory currently allocated to VMs in megabytes.
	UsedMemoryMB int64
	// VMCount is the number of VMs on this node.
	VMCount int32
	// LoadAverage is the load average over 1, 5, and 15 minutes.
	LoadAverage [3]float64
	// UptimeSeconds is the node uptime in seconds.
	UptimeSeconds int64
}

// mapLibvirtState converts a libvirt domain state to a human-readable string.
func mapLibvirtState(state libvirt.DomainState) string {
	switch state {
	case libvirt.DOMAIN_RUNNING:
		return "running"
	case libvirt.DOMAIN_BLOCKED:
		return "blocked"
	case libvirt.DOMAIN_PAUSED:
		return "paused"
	case libvirt.DOMAIN_SHUTDOWN:
		return "shutting_down"
	case libvirt.DOMAIN_SHUTOFF:
		return "stopped"
	case libvirt.DOMAIN_CRASHED:
		return "crashed"
	case libvirt.DOMAIN_PMSUSPENDED:
		return "suspended"
	default:
		return "unknown"
	}
}
