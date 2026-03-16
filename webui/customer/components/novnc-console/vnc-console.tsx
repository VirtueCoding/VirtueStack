"use client"

import { useState, useEffect, useRef, useCallback } from "react"
import { cn } from "@/lib/utils"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Monitor,
  Maximize,
  Minimize,
  Power,
  Loader2,
  RefreshCw,
  Wifi,
  WifiOff,
  Clipboard,
  Keyboard,
  Copy,
} from "lucide-react"

let RFB: typeof import("@novnc/novnc/lib/rfb").default | null = null

type ConnectionStatus = "disconnected" | "connecting" | "connected" | "error"

interface VNCConsoleProps {
  className?: string
  wsUrl?: string
  vmId?: string
  token?: string
}

export function VNCConsole({
  className,
  wsUrl,
  vmId,
  token,
}: VNCConsoleProps) {
  const [status, setStatus] = useState<ConnectionStatus>("disconnected")
  const [isFullScreen, setIsFullScreen] = useState(false)
  const [errorMessage, setErrorMessage] = useState<string | null>(null)
  const [clipboardText, setClipboardText] = useState("")
  const [isClipboardOpen, setIsClipboardOpen] = useState(false)
  const [passwordPromptOpen, setPasswordPromptOpen] = useState(false)
  const [vncPassword, setVncPassword] = useState("")
  const containerRef = useRef<HTMLDivElement>(null)
  const screenRef = useRef<HTMLDivElement>(null)
  const rfbRef = useRef<InstanceType<typeof import("@novnc/novnc/lib/rfb").default> | null>(null)

  // Build WebSocket URL
  const getWsUrl = useCallback(() => {
    if (wsUrl) return wsUrl
    if (vmId && token) {
      const protocol = window.location.protocol === "https:" ? "wss:" : "ws:"
      return `${protocol}//${window.location.host}/api/v1/customer/ws/vnc/${vmId}?token=${token}`
    }
    return null
  }, [wsUrl, vmId, token])

  // Connect to VNC
  const connect = useCallback(async () => {
    const url = getWsUrl()
    if (!url) {
      setErrorMessage("Console URL not available")
      setStatus("error")
      return
    }

    if (rfbRef.current) {
      return
    }

    if (!RFB) {
      const mod = await import("@novnc/novnc/lib/rfb")
      RFB = mod.default
    }

    if (!screenRef.current) {
      setErrorMessage("Screen container not available")
      setStatus("error")
      return
    }

    setStatus("connecting")
    setErrorMessage(null)

    try {
      // Clear any existing content
      while (screenRef.current.firstChild) {
        screenRef.current.removeChild(screenRef.current.firstChild)
      }

      const rfb = new RFB(screenRef.current, url, {
        shared: true,
      })

      rfb.scaleViewport = true
      rfb.resizeSession = true
      rfb.clipViewport = true

      rfb.addEventListener("connect", () => {
        setStatus("connected")
      })

      rfb.addEventListener("disconnect", (e: { detail: { clean: boolean } }) => {
        setStatus("disconnected")
        if (!e.detail.clean) {
          setErrorMessage("Connection closed unexpectedly")
          setStatus("error")
        }
        rfbRef.current = null
      })

      rfb.addEventListener("credentialsrequired", () => {
        setPasswordPromptOpen(true)
      })

      rfb.addEventListener("securityfailure", (e: { detail: { reason: string } }) => {
        setErrorMessage(`Security failure: ${e.detail.reason}`)
        setStatus("error")
      })

      rfbRef.current = rfb
    } catch (error) {
      setErrorMessage("Failed to initialize VNC connection")
      setStatus("error")
    }
  }, [getWsUrl])

  // Disconnect from VNC
  const disconnect = useCallback(() => {
    if (rfbRef.current) {
      rfbRef.current.disconnect()
      rfbRef.current = null
    }
    setStatus("disconnected")
    setIsFullScreen(false)

    // Clear screen
    if (screenRef.current) {
      while (screenRef.current.firstChild) {
        screenRef.current.removeChild(screenRef.current.firstChild)
      }
    }
  }, [])

  const handlePasswordSubmit = useCallback(() => {
    if (rfbRef.current && vncPassword) {
      rfbRef.current.sendCredentials({ username: "", password: vncPassword, target: "" })
      setPasswordPromptOpen(false)
      setVncPassword("")
    }
  }, [vncPassword])

  // Toggle fullscreen
  const toggleFullScreen = useCallback(() => {
    if (!containerRef.current) return

    if (!document.fullscreenElement) {
      containerRef.current.requestFullscreen().catch(() => {})
      setIsFullScreen(true)
    } else {
      document.exitFullscreen().catch(() => {})
      setIsFullScreen(false)
    }
  }, [])

  // Send Ctrl+Alt+Del
  const sendCtrlAltDel = useCallback(() => {
    if (rfbRef.current) {
      rfbRef.current.sendCtrlAltDel()
    }
  }, [])

  // Send clipboard text to VNC
  const sendClipboard = useCallback(() => {
    if (rfbRef.current && clipboardText) {
      rfbRef.current.clipboardPasteFrom(clipboardText)
      setIsClipboardOpen(false)
      setClipboardText("")
    }
  }, [clipboardText])

  // Handle fullscreen change events
  useEffect(() => {
    const handleFullscreenChange = () => {
      setIsFullScreen(!!document.fullscreenElement)
    }

    document.addEventListener("fullscreenchange", handleFullscreenChange)
    return () =>
      document.removeEventListener("fullscreenchange", handleFullscreenChange)
  }, [])

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (rfbRef.current) {
        rfbRef.current.disconnect()
        rfbRef.current = null
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
        {/* Clipboard Dialog */}
        <Dialog open={isClipboardOpen} onOpenChange={setIsClipboardOpen}>
          <DialogTrigger asChild>
            <Button variant="outline" size="icon" title="Clipboard">
              <Clipboard className="h-4 w-4" />
            </Button>
          </DialogTrigger>
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle>Clipboard</DialogTitle>
              <DialogDescription>
                Send text to the remote clipboard
              </DialogDescription>
            </DialogHeader>
            <div className="grid gap-4 py-4">
              <div className="grid gap-2">
                <Label htmlFor="clipboard-text">Text to send</Label>
                <Input
                  id="clipboard-text"
                  value={clipboardText}
                  onChange={(e) => setClipboardText(e.target.value)}
                  placeholder="Enter text to send to remote clipboard..."
                />
              </div>
            </div>
            <div className="flex justify-end gap-2">
              <Button variant="outline" onClick={() => setIsClipboardOpen(false)}>
                Cancel
              </Button>
              <Button onClick={sendClipboard} disabled={!clipboardText}>
                <Copy className="h-4 w-4 mr-2" />
                Send to Remote
              </Button>
            </div>
          </DialogContent>
        </Dialog>

        {/* Ctrl+Alt+Del Button */}
        <Button
          variant="outline"
          size="sm"
          onClick={sendCtrlAltDel}
          title="Send Ctrl+Alt+Del"
        >
          <Keyboard className="h-4 w-4 mr-1" />
          Ctrl+Alt+Del
        </Button>

        {/* Fullscreen Button */}
        <Button
          onClick={toggleFullScreen}
          variant="outline"
          size="icon"
          title="Toggle Fullscreen"
        >
          {isFullScreen ? (
            <Minimize className="h-4 w-4" />
          ) : (
            <Maximize className="h-4 w-4" />
          )}
        </Button>

        {/* Disconnect Button */}
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
                {vmId ? `VM: ${vmId}` : "Remote Desktop Access"}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-3">{getStatusBadge()}</div>
        </div>
      </CardHeader>

      <CardContent className="space-y-4">
        {/* Screen Container - 16:9 Aspect Ratio */}
        <div
          className="relative w-full bg-black rounded-lg overflow-hidden"
          style={{ aspectRatio: "16/9" }}
        >
          {status === "disconnected" && (
            <div className="absolute inset-0 flex flex-col items-center justify-center border-2 border-dashed border-muted bg-muted/20">
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
            <div className="absolute inset-0 flex flex-col items-center justify-center bg-black">
              <Loader2 className="h-10 w-10 animate-spin text-primary mb-3" />
              <p className="text-muted-foreground text-sm">
                Establishing VNC connection...
              </p>
            </div>
          )}

          {status === "error" && (
            <div className="absolute inset-0 flex flex-col items-center justify-center bg-destructive/5">
              <WifiOff className="h-10 w-10 text-destructive mb-3" />
              <p className="text-destructive font-medium">
                {errorMessage || "Connection failed"}
              </p>
              <p className="text-muted-foreground/70 text-xs mt-1">
                Check your connection settings and try again
              </p>
            </div>
          )}

          <div
            ref={screenRef}
            className="w-full h-full"
            style={{
              display: status === "connected" ? "flex" : "none",
            }}
          />
        </div>

        {/* Control Bar */}
        <div className="flex items-center justify-between border-t pt-4">
          <div className="flex items-center gap-2">
            {status === "connected" && (
              <Badge variant="outline" className="text-xs">
                noVNC Client
              </Badge>
            )}
            {status === "connected" && (
              <Badge variant="outline" className="text-xs">
                Interactive
              </Badge>
            )}
          </div>
          {getControls()}
        </div>
      </CardContent>

      <Dialog open={passwordPromptOpen} onOpenChange={setPasswordPromptOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>VNC Password Required</DialogTitle>
            <DialogDescription>
              Enter the VNC password to connect to this remote desktop.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="vnc-password">Password</Label>
              <Input
                id="vnc-password"
                type="password"
                value={vncPassword}
                onChange={(e) => setVncPassword(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    handlePasswordSubmit()
                  }
                }}
                placeholder="Enter VNC password"
              />
            </div>
          </div>
          <Button onClick={handlePasswordSubmit} disabled={!vncPassword}>
            Connect
          </Button>
        </DialogContent>
      </Dialog>
    </Card>
  )
}

export default VNCConsole
