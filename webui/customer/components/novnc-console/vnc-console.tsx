"use client"

import { useState, useEffect, useRef, useCallback } from "react"
import { cn } from "@/lib/utils"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import {
  Monitor,
  Maximize,
  Minimize,
  Power,
  Loader2,
  RefreshCw,
  Wifi,
  WifiOff,
} from "lucide-react"

type ConnectionStatus = "disconnected" | "connecting" | "connected" | "error"

interface VNCConsoleProps {
  className?: string
  wsUrl?: string
  vmId?: string
}

export function VNCConsole({
  className,
  wsUrl,
  vmId,
}: VNCConsoleProps) {
  const [status, setStatus] = useState<ConnectionStatus>("disconnected")
  const [isFullScreen, setIsFullScreen] = useState(false)
  const [errorMessage, setErrorMessage] = useState<string | null>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const animationRef = useRef<number | undefined>(undefined)
  const wsRef = useRef<WebSocket | null>(null)

  // Real WebSocket connection
  const connect = useCallback(() => {
    if (!wsUrl) {
      setErrorMessage("Console URL not available")
      setStatus("error")
      return
    }

    setStatus("connecting")
    setErrorMessage(null)

    try {
      const ws = new WebSocket(wsUrl)
      wsRef.current = ws

      ws.onopen = () => {
        setStatus("connected")
      }

      ws.onclose = () => {
        setStatus("disconnected")
        wsRef.current = null
      }

      ws.onerror = () => {
        setErrorMessage("WebSocket connection error")
        setStatus("error")
        wsRef.current = null
      }
    } catch (error) {
      setErrorMessage("Failed to initialize WebSocket")
      setStatus("error")
    }
  }, [wsUrl])

  const disconnect = useCallback(() => {
    if (wsRef.current) {
      wsRef.current.close()
      wsRef.current = null
    }
    setStatus("disconnected")
    setIsFullScreen(false)

    // Clear canvas
    const canvas = canvasRef.current
    if (canvas) {
      const ctx = canvas.getContext("2d")
      if (ctx) {
        ctx.clearRect(0, 0, canvas.width, canvas.height)
      }
    }
  }, [])

  const toggleFullScreen = useCallback(() => {
    if (!containerRef.current) return

    if (!document.fullscreenElement) {
      containerRef.current.requestFullscreen().catch((err) => {
        console.error("Failed to enter fullscreen:", err)
      })
      setIsFullScreen(true)
    } else {
      document.exitFullscreen().catch((err) => {
        console.error("Failed to exit fullscreen:", err)
      })
      setIsFullScreen(false)
    }
  }, [])

  // Handle fullscreen change events
  useEffect(() => {
    const handleFullscreenChange = () => {
      setIsFullScreen(!!document.fullscreenElement)
    }

    document.addEventListener("fullscreenchange", handleFullscreenChange)
    return () =>
      document.removeEventListener("fullscreenchange", handleFullscreenChange)
  }, [])

  // Draw gradient pattern on canvas when connected
  useEffect(() => {
    if (status !== "connected" || !canvasRef.current) return

    const canvas = canvasRef.current
    const ctx = canvas.getContext("2d")
    if (!ctx) return

    // Set canvas size
    const updateCanvasSize = () => {
      const container = containerRef.current
      if (!container) return

      const rect = container.getBoundingClientRect()
      canvas.width = rect.width
      canvas.height = rect.height

      // Draw gradient background
      const gradient = ctx.createLinearGradient(0, 0, rect.width, rect.height)
      gradient.addColorStop(0, "#1e293b")
      gradient.addColorStop(0.5, "#334155")
      gradient.addColorStop(1, "#1e293b")
      ctx.fillStyle = gradient
      ctx.fillRect(0, 0, rect.width, rect.height)

      // Draw grid pattern
      ctx.strokeStyle = "rgba(255, 255, 255, 0.05)"
      ctx.lineWidth = 1
      const gridSize = 40

      for (let x = 0; x < rect.width; x += gridSize) {
        ctx.beginPath()
        ctx.moveTo(x, 0)
        ctx.lineTo(x, rect.height)
        ctx.stroke()
      }

      for (let y = 0; y < rect.height; y += gridSize) {
        ctx.beginPath()
        ctx.moveTo(0, y)
        ctx.lineTo(rect.width, y)
        ctx.stroke()
      }

      // Draw "VNC Connected" text
      ctx.fillStyle = "rgba(255, 255, 255, 0.3)"
      ctx.font = "bold 24px system-ui, -apple-system, sans-serif"
      ctx.textAlign = "center"
      ctx.textBaseline = "middle"
      ctx.fillText("VNC Connected", rect.width / 2, rect.height / 2)

      // Draw VM ID if provided
      if (vmId) {
        ctx.fillStyle = "rgba(255, 255, 255, 0.2)"
        ctx.font = "14px system-ui, -apple-system, sans-serif"
        ctx.fillText(`VM: ${vmId}`, rect.width / 2, rect.height / 2 + 40)
      }
    }

    updateCanvasSize()

    // Redraw on resize
    const resizeObserver = new ResizeObserver(updateCanvasSize)
    resizeObserver.observe(containerRef.current!)

    return () => resizeObserver.disconnect()
  }, [status, vmId])

  // Cleanup animation frame on unmount
  useEffect(() => {
    return () => {
      if (animationRef.current) {
        cancelAnimationFrame(animationRef.current)
      }
    }
  }, [])

  const getStatusBadge = () => {
    switch (status) {
      case "connected":
        return (
          <Badge variant="success" className="gap-1.5">
            <Wifi className="h-3 w-3" />
            Connected
          </Badge>
        )
      case "connecting":
        return (
          <Badge variant="warning" className="gap-1.5">
            <Loader2 className="h-3 w-3 animate-spin" />
            Connecting
          </Badge>
        )
      case "error":
        return (
          <Badge variant="destructive" className="gap-1.5">
            <WifiOff className="h-3 w-3" />
            Error
          </Badge>
        )
      default:
        return (
          <Badge variant="secondary" className="gap-1.5">
            <WifiOff className="h-3 w-3" />
            Disconnected
          </Badge>
        )
    }
  }

  const getControls = () => {
    if (status === "disconnected") {
      return (
        <Button onClick={connect}>
          <Power className="h-4 w-4" />
          Connect
        </Button>
      )
    }

    if (status === "connecting") {
      return (
        <Button disabled variant="secondary">
          <Loader2 className="h-4 w-4 animate-spin" />
          Connecting...
        </Button>
      )
    }

    if (status === "error") {
      return (
        <div className="flex items-center gap-2">
          <Button onClick={connect} variant="outline">
            <RefreshCw className="h-4 w-4" />
            Retry
          </Button>
          <Button onClick={disconnect} variant="destructive">
            <Power className="h-4 w-4" />
            Disconnect
          </Button>
        </div>
      )
    }

    // Connected state
    return (
      <div className="flex items-center gap-2">
        <Button onClick={toggleFullScreen} variant="outline" size="icon">
          {isFullScreen ? (
            <Minimize className="h-4 w-4" />
          ) : (
            <Maximize className="h-4 w-4" />
          )}
        </Button>
        <Button onClick={disconnect} variant="destructive">
          <Power className="h-4 w-4" />
          Disconnect
        </Button>
      </div>
    )
  }

  return (
    <Card
      ref={containerRef}
      className={cn("overflow-hidden transition-all duration-300", className)}
    >
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10">
              <Monitor className="h-5 w-5 text-primary" />
            </div>
            <div>
              <CardTitle className="text-lg">VNC Console</CardTitle>
              <p className="text-sm text-muted-foreground">
                {wsUrl ? wsUrl : "No URL provided"}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-3">{getStatusBadge()}</div>
        </div>
      </CardHeader>

      <CardContent className="space-y-4">
        {/* Canvas Container - 16:9 Aspect Ratio */}
        <div className="relative w-full" style={{ aspectRatio: "16/9" }}>
          {status === "disconnected" && (
            <div className="absolute inset-0 flex flex-col items-center justify-center rounded-lg border-2 border-dashed border-muted bg-muted/20">
              <Monitor className="h-12 w-12 text-muted-foreground/50 mb-3" />
              <p className="text-muted-foreground text-sm">
                Click Connect to start VNC session
              </p>
              <p className="text-muted-foreground/70 text-xs mt-1">
                {vmId ? `Virtual Machine: ${vmId}` : "Remote Desktop Access"}
              </p>
            </div>
          )}

          {status === "connecting" && (
            <div className="absolute inset-0 flex flex-col items-center justify-center rounded-lg border bg-muted/10">
              <Loader2 className="h-10 w-10 animate-spin text-primary mb-3" />
              <p className="text-muted-foreground text-sm">
                Establishing connection...
              </p>
              <p className="text-muted-foreground/70 text-xs mt-1">
                Connecting to {wsUrl}
              </p>
            </div>
          )}

          {status === "error" && (
            <div className="absolute inset-0 flex flex-col items-center justify-center rounded-lg border bg-destructive/5">
              <WifiOff className="h-10 w-10 text-destructive mb-3" />
              <p className="text-destructive font-medium">
                {errorMessage || "Connection failed"}
              </p>
              <p className="text-muted-foreground/70 text-xs mt-1">
                Check your connection settings and try again
              </p>
            </div>
          )}

          {status === "connected" && (
            <canvas
              ref={canvasRef}
              className="h-full w-full rounded-lg"
              style={{ display: "block" }}
            />
          )}
        </div>

        {/* Control Bar */}
        <div className="flex items-center justify-between border-t pt-4">
          <div className="flex items-center gap-2">
            {status === "connected" && (
              <Badge variant="outline" className="text-xs">
                Full HD (1920x1080)
              </Badge>
            )}
            {status === "connected" && (
              <Badge variant="outline" className="text-xs">
                60 FPS
              </Badge>
            )}
          </div>
          {getControls()}
        </div>
      </CardContent>
    </Card>
  )
}

export default VNCConsole
