"use client"

import { cn } from "@/lib/utils"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@virtuestack/ui"
import { Button } from "@virtuestack/ui"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@virtuestack/ui"
import { useEffect, useState, useCallback, useRef } from "react"
import { vmApi } from "@/lib/api-client"
import { AlertCircle, RefreshCw } from "lucide-react"
import {
  Area,
  AreaChart,
  CartesianGrid,
  Legend,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts"

interface ChartDataPoint {
  timestamp: string
  cpu_percent?: number
  memory_percent?: number
  network_rx_mbps?: number
  network_tx_mbps?: number
  disk_read_mbps?: number
  disk_write_mbps?: number
}

type TimeRange = "1h" | "24h" | "7d"

interface ResourceChartsProps {
  className?: string
  vmId?: string
}
const colors = {
  cpu: "#3b82f6",      // blue-500
  memory: "#8b5cf6",   // violet-500
  networkRx: "#10b981", // emerald-500
  networkTx: "#f59e0b", // amber-500
  diskRead: "#06b6d4",  // cyan-500
  diskWrite: "#ec4899", // pink-500
}

interface BaseChartProps {
  data: ChartDataPoint[]
  title: string
  description: string
  dataKeys: Array<{ key: string; color: string; name: string }>
  yAxisUnit?: string
  yAxisDomain?: [number | string, number | string]
  yAxisTicks?: number[]
}

const BaseChart: React.FC<BaseChartProps> = ({
  data,
  title,
  description,
  dataKeys,
  yAxisUnit = "%",
  yAxisDomain = [0, 100],
  yAxisTicks = [0, 25, 50, 75, 100],
}) => {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg">{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="h-[300px] w-full">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={data} margin={{ top: 10, right: 10, left: 0, bottom: 0 }}>
              <defs>
                {dataKeys.map(({ key, color }) => (
                  <linearGradient key={key} id={`fill-${key}`} x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor={color} stopOpacity={0.3} />
                    <stop offset="95%" stopColor={color} stopOpacity={0} />
                  </linearGradient>
                ))}
              </defs>
              <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
              <XAxis
                dataKey="timestamp"
                tick={{ fontSize: 12, fill: "var(--foreground)" }}
                axisLine={false}
                tickLine={false}
              />
              <YAxis
                domain={yAxisDomain}
                ticks={yAxisTicks}
                tick={{ fontSize: 12, fill: "var(--foreground)" }}
                tickFormatter={(value) => `${value}${yAxisUnit}`}
                axisLine={false}
                tickLine={false}
              />
              <Tooltip
                contentStyle={{
                  backgroundColor: "var(--background)",
                  border: "1px solid var(--border)",
                  borderRadius: "var(--radius)",
                }}
                labelStyle={{ color: "var(--foreground)", marginBottom: "0.5rem" }}
                formatter={(value, name) => [
                  `${Number(value || 0).toFixed(1)}${yAxisUnit}`,
                  name,
                ]}
              />
              <Legend />
              {dataKeys.map(({ key, color, name }) => (
                <Area
                  key={key}
                  type="monotone"
                  dataKey={key}
                  name={name}
                  stroke={color}
                  fillOpacity={1}
                  fill={`url(#fill-${key})`}
                  strokeWidth={2}
                />
              ))}
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  )
}

const CPUChart: React.FC<{ data: ChartDataPoint[] }> = ({ data }) => (
  <BaseChart
    data={data}
    title="CPU Usage"
    description="Processor utilization over time"
    dataKeys={[{ key: "cpu_percent", color: colors.cpu, name: "CPU" }]}
  />
)

const MemoryChart: React.FC<{ data: ChartDataPoint[] }> = ({ data }) => (
  <BaseChart
    data={data}
    title="Memory Usage"
    description="RAM consumption over time"
    dataKeys={[{ key: "memory_percent", color: colors.memory, name: "Memory" }]}
  />
)

const NetworkChart: React.FC<{ data: ChartDataPoint[] }> = ({ data }) => (
  <BaseChart
    data={data}
    title="Network Traffic"
    description="Network throughput (receive/transmit)"
    dataKeys={[
      { key: "network_rx_mbps", color: colors.networkRx, name: "Receive" },
      { key: "network_tx_mbps", color: colors.networkTx, name: "Transmit" },
    ]}
    yAxisUnit=" MB"
    yAxisDomain={[0, "auto"]}
    yAxisTicks={[0, 20, 40, 60, 80, 100]}
  />
)

const DiskChart: React.FC<{ data: ChartDataPoint[] }> = ({ data }) => (
  <BaseChart
    data={data}
    title="Disk I/O"
    description="Disk read/write operations"
    dataKeys={[
      { key: "disk_read_mbps", color: colors.diskRead, name: "Read" },
      { key: "disk_write_mbps", color: colors.diskWrite, name: "Write" },
    ]}
    yAxisUnit=" MB"
    yAxisDomain={[0, "auto"]}
    yAxisTicks={[0, 10, 20, 30, 40, 50]}
  />
)

export function ResourceCharts({ className, vmId }: ResourceChartsProps) {
  const [timeRange, setTimeRange] = useState<TimeRange>("1h")
  const [chartData, setChartData] = useState<ChartDataPoint[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const historyRef = useRef<ChartDataPoint[]>([])

  const fetchMetrics = useCallback(async () => {
    if (!vmId) return
    setLoading(true)
    setError(null)
    try {
      const metrics = await vmApi.getMetrics(vmId)
      const point: ChartDataPoint = {
        timestamp: new Date().toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit" }),
        cpu_percent: metrics.cpu_usage_percent,
        memory_percent: metrics.memory_total_bytes > 0
          ? (metrics.memory_usage_bytes / metrics.memory_total_bytes) * 100
          : 0,
        network_rx_mbps: metrics.network_rx_bytes / (1024 * 1024),
        network_tx_mbps: metrics.network_tx_bytes / (1024 * 1024),
        disk_read_mbps: metrics.disk_read_bytes / (1024 * 1024),
        disk_write_mbps: metrics.disk_write_bytes / (1024 * 1024),
      }
      const maxPoints = timeRange === "1h" ? 13 : timeRange === "24h" ? 25 : 7
      historyRef.current = [...historyRef.current, point].slice(-maxPoints)
      setChartData([...historyRef.current])
    } catch (err) {
      setError('Failed to load metrics. Please try again.')
    } finally {
      setLoading(false)
    }
  }, [vmId, timeRange])

  useEffect(() => {
    historyRef.current = []
    if (vmId) {
      fetchMetrics()
      const interval = setInterval(fetchMetrics, 30000)
      return () => clearInterval(interval)
    }
  }, [timeRange, vmId, fetchMetrics])

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center p-8 text-center">
        <AlertCircle className="h-12 w-12 text-red-500 mb-4" />
        <p className="text-red-600 mb-4">{error}</p>
        <Button onClick={fetchMetrics} variant="outline">
          <RefreshCw className="mr-2 h-4 w-4" />
          Retry
        </Button>
      </div>
    )
  }

  return (
    <div className={cn("space-y-6", className)}>
      {/* Time Range Selector */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight">Resource Monitoring</h2>
          <p className="text-muted-foreground">
            Track CPU, memory, network, and disk usage
          </p>
        </div>
        <Select value={timeRange} onValueChange={(value: TimeRange) => setTimeRange(value)}>
          <SelectTrigger className="w-[120px]">
            <SelectValue placeholder="Select range" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="1h">Last 1h</SelectItem>
            <SelectItem value="24h">Last 24h</SelectItem>
            <SelectItem value="7d">Last 7d</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {/* Charts Grid */}
      <div className="grid gap-6 md:grid-cols-2">
        <CPUChart data={chartData} />
        <MemoryChart data={chartData} />
        <NetworkChart data={chartData} />
        <DiskChart data={chartData} />
      </div>
    </div>
  )
}
