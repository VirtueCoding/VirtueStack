package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	APIRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vs_api_requests_total",
			Help: "Total number of API requests",
		},
		[]string{"method", "path", "status"},
	)

	APIRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "vs_api_request_duration_seconds",
			Help:    "API request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	VMsTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vs_vms_total",
			Help: "Total number of VMs by status",
		},
		[]string{"status"},
	)

	NodesTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vs_nodes_total",
			Help: "Total number of nodes by status",
		},
		[]string{"status"},
	)

	NodeHeartbeatAge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vs_node_heartbeat_age_seconds",
			Help: "Age of last heartbeat per node",
		},
		[]string{"node_id"},
	)

	TasksTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vs_tasks_total",
			Help: "Total number of tasks processed",
		},
		[]string{"type", "status"},
	)

	TaskDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "vs_task_duration_seconds",
			Help:    "Task processing duration in seconds",
			Buckets: []float64{1, 5, 10, 30, 60, 300, 600, 1800, 3600},
		},
		[]string{"type"},
	)

	WSConnectionsActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "vs_ws_connections_active",
			Help: "Number of active WebSocket connections",
		},
	)

	BackupDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "vs_backup_duration_seconds",
			Help:    "Backup duration in seconds",
			Buckets: []float64{10, 30, 60, 120, 300, 600, 1800, 3600},
		},
		[]string{"type"},
	)

	BandwidthBytesTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vs_bandwidth_bytes_total",
			Help: "Total bandwidth in bytes",
		},
		[]string{"vm_id", "direction"},
	)
)
