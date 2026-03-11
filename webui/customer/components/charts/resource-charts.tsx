"use client"

import { cn } from "@/lib/utils"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { useEffect, useState } from "react"
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
}

// Mock data generators
const generateTimeRange = (range: TimeRange): string[] => {
  const now = new Date()
  const points: string[] = []
  
  switch (range) {
    case "1h":
      for (let i = 60; i >= 0; i -= 5) {
        const time = new Date(now.getTime() - i * 60 * 1000)
        points.push(time.toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit" }))
      }
      break
    case "24h":
      for (let i = 24; i >= 0; i--) {
        const time = new Date(now.getTime() - i * 60 * 60 * 1000)
        points.push(time.toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit" }))
      }
      break
    case "7d":
      for (let i = 6; i >= 0; i--) {
        const time = new Date(now.getTime() - i * 24 * 60 * 60 * 1000)
        points.push(time.toLocaleDateString("en-US", { weekday: "short", month: "short", day: "numeric" }))
      }
      break
  }
  
  return points
}

const generateMockData = (range: TimeRange): ChartDataPoint[] => {
  const timestamps = generateTimeRange(range)
  
  return timestamps.map((timestamp, index) => {
    const variation = range === "1h" ? 15 : range === "24h" ? 25 : 30
    const baseCpu = 35 + Math.random() * 20
    const baseMemory = 55 + Math.random() * 15
    const baseNetworkRx = 45 + Math.random() * 30
    const baseNetworkTx = 25 + Math.random() * 20
    const baseDiskRead = 20 + Math.random() * 15
    const baseDiskWrite = 15 + Math.random() * 10
    
    // Add some peaks and valleys
    const peakFactor = Math.sin(index * 0.5) * variation
    
    return {
      timestamp,
      cpu_percent: Math.min(100, Math.max(0, baseCpu + peakFactor)),
      memory_percent: Math.min(100, Math.max(0, baseMemory + peakFactor * 0.5)),
      network_rx_mbps: Math.max(0, baseNetworkRx + peakFactor * 0.8),
      network_tx_mbps: Math.max(0, baseNetworkTx + peakFactor * 0.6),
      disk_read_mbps: Math.max(0, baseDiskRead + peakFactor * 0.4),
      disk_write_mbps: Math.max(0, baseDiskWrite + peakFactor * 0.3),
    }
  })
}

// Chart color palette (using CSS variable references for theme support)
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
  yAxisDomain?: [number, number]
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
    yAxisUnit=" Mbps"
    yAxisDomain={[0, "auto" as unknown as number]}
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
    yAxisUnit=" Mbps"
    yAxisDomain={[0, "auto" as unknown as number]}
    yAxisTicks={[0, 10, 20, 30, 40, 50]}
  />
)

export function ResourceCharts({ className }: ResourceChartsProps) {
  const [timeRange, setTimeRange] = useState<TimeRange>("1h")
  const [chartData, setChartData] = useState<ChartDataPoint[]>(() => generateMockData("1h"))

  // Update chart data when time range changes
  useEffect(() => {
    setChartData(generateMockData(timeRange))
  }, [timeRange])

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
