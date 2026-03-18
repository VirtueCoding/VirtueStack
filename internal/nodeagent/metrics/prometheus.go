package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// VMCPUUsagePercent tracks CPU usage percentage per VM.
	VMCPUUsagePercent = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vs_vm_cpu_usage_percent",
			Help: "VM CPU usage percentage",
		},
		[]string{"vm_id"},
	)

	// VMMemoryUsageBytes tracks memory usage in bytes per VM.
	VMMemoryUsageBytes = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vs_vm_memory_usage_bytes",
			Help: "VM memory usage in bytes",
		},
		[]string{"vm_id"},
	)

	// VMMemoryLimitBytes tracks the allocated memory limit in bytes per VM.
	VMMemoryLimitBytes = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vs_vm_memory_limit_bytes",
			Help: "VM allocated memory limit in bytes",
		},
		[]string{"vm_id"},
	)

	// VMDiskReadBytesTotal tracks total bytes read from disk per VM.
	VMDiskReadBytesTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vs_vm_disk_read_bytes_total",
			Help: "Total bytes read from disk per VM",
		},
		[]string{"vm_id"},
	)

	// VMDiskWriteBytesTotal tracks total bytes written to disk per VM.
	VMDiskWriteBytesTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vs_vm_disk_write_bytes_total",
			Help: "Total bytes written to disk per VM",
		},
		[]string{"vm_id"},
	)

	// VMDiskReadOpsTotal tracks total read operations per VM.
	VMDiskReadOpsTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vs_vm_disk_read_ops_total",
			Help: "Total read operations on disk per VM",
		},
		[]string{"vm_id"},
	)

	// VMDiskWriteOpsTotal tracks total write operations per VM.
	VMDiskWriteOpsTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vs_vm_disk_write_ops_total",
			Help: "Total write operations on disk per VM",
		},
		[]string{"vm_id"},
	)

	// VMNetworkRXBytesTotal tracks total bytes received on network per VM.
	VMNetworkRXBytesTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vs_vm_network_rx_bytes_total",
			Help: "Total bytes received on network per VM",
		},
		[]string{"vm_id"},
	)

	// VMNetworkTXBytesTotal tracks total bytes transmitted on network per VM.
	VMNetworkTXBytesTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vs_vm_network_tx_bytes_total",
			Help: "Total bytes transmitted on network per VM",
		},
		[]string{"vm_id"},
	)

	// VMNetworkRXPacketsTotal tracks total packets received per VM.
	VMNetworkRXPacketsTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vs_vm_network_rx_packets_total",
			Help: "Total packets received on network per VM",
		},
		[]string{"vm_id"},
	)

	// VMNetworkTXPacketsTotal tracks total packets transmitted per VM.
	VMNetworkTXPacketsTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vs_vm_network_tx_packets_total",
			Help: "Total packets transmitted on network per VM",
		},
		[]string{"vm_id"},
	)

	// VMNetworkRXErrorsTotal tracks total receive errors per VM.
	VMNetworkRXErrorsTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vs_vm_network_rx_errors_total",
			Help: "Total receive errors on network per VM",
		},
		[]string{"vm_id"},
	)

	// VMNetworkTXErrorsTotal tracks total transmit errors per VM.
	VMNetworkTXErrorsTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vs_vm_network_tx_errors_total",
			Help: "Total transmit errors on network per VM",
		},
		[]string{"vm_id"},
	)

	// VMNetworkRXDropsTotal tracks total receive drops per VM.
	VMNetworkRXDropsTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vs_vm_network_rx_drops_total",
			Help: "Total receive drops on network per VM",
		},
		[]string{"vm_id"},
	)

	// VMNetworkTXDropsTotal tracks total transmit drops per VM.
	VMNetworkTXDropsTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vs_vm_network_tx_drops_total",
			Help: "Total transmit drops on network per VM",
		},
		[]string{"vm_id"},
	)

	// VMStatus tracks the operational status of each VM (1=running, 0=stopped).
	VMStatus = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vs_vm_status",
			Help: "VM operational status (1=running, 0=stopped)",
		},
		[]string{"vm_id"},
	)

	// NodeCPUSecondsTotal tracks CPU seconds consumed by the node agent process.
	NodeCPUSecondsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "vs_node_cpu_seconds_total",
			Help: "Total CPU seconds consumed by the node agent process",
		},
	)

	// NodeMemoryAvailableBytes tracks available memory on the node in bytes.
	NodeMemoryAvailableBytes = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "vs_node_memory_available_bytes",
			Help: "Available memory on the node in bytes",
		},
	)

	// NodeVMCount tracks the number of VMs on the node.
	NodeVMCount = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "vs_node_vm_count",
			Help: "Number of VMs on the node",
		},
	)
)

// StatusToValue converts a VM status string to a numeric value for Prometheus metrics.
// Returns 1 for "running" status, 0 for all other states.
func StatusToValue(status string) float64 {
	if status == "running" {
		return 1
	}
	return 0
}
