package metrics

import (
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
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

var (
	dbPoolMetricsMu         sync.Mutex
	dbPoolMetricsRegistered bool
	dbPoolStatsProvider     func() *pgxpool.Stat
)

var (
	DBPoolTotalConns = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "vs_db_pool_total_conns",
			Help: "Current total PostgreSQL connections in the pool",
		},
		func() float64 {
			if dbPoolStatsProvider == nil {
				return 0
			}
			return float64(dbPoolStatsProvider().TotalConns())
		},
	)
	DBPoolIdleConns = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "vs_db_pool_idle_conns",
			Help: "Current idle PostgreSQL connections in the pool",
		},
		func() float64 {
			if dbPoolStatsProvider == nil {
				return 0
			}
			return float64(dbPoolStatsProvider().IdleConns())
		},
	)
	DBPoolMaxConns = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "vs_db_pool_max_conns",
			Help: "Configured maximum PostgreSQL connections in the pool",
		},
		func() float64 {
			if dbPoolStatsProvider == nil {
				return 0
			}
			return float64(dbPoolStatsProvider().MaxConns())
		},
	)
	DBPoolAcquiredConns = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "vs_db_pool_acquired_conns",
			Help: "Current acquired PostgreSQL connections in the pool",
		},
		func() float64 {
			if dbPoolStatsProvider == nil {
				return 0
			}
			return float64(dbPoolStatsProvider().AcquiredConns())
		},
	)
	DBPoolAcquireWaitTime = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "vs_db_pool_acquire_wait_seconds_total",
			Help: "Total wait time spent acquiring PostgreSQL connections (seconds)",
		},
		func() float64 {
			if dbPoolStatsProvider == nil {
				return 0
			}
			return dbPoolStatsProvider().AcquireDuration().Seconds()
		},
	)
)

func RegisterDBPoolMetrics(pool *pgxpool.Pool) {
	dbPoolMetricsMu.Lock()
	defer dbPoolMetricsMu.Unlock()

	dbPoolStatsProvider = pool.Stat
	if dbPoolMetricsRegistered {
		return
	}

	prometheus.MustRegister(DBPoolTotalConns)
	prometheus.MustRegister(DBPoolIdleConns)
	prometheus.MustRegister(DBPoolMaxConns)
	prometheus.MustRegister(DBPoolAcquiredConns)
	prometheus.MustRegister(DBPoolAcquireWaitTime)
	dbPoolMetricsRegistered = true
}
