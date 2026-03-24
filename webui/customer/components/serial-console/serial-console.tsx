"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { Terminal as TerminalIcon, Trash2, Power, Wifi, WifiOff } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

import "@xterm/xterm/css/xterm.css";

interface SerialConsoleProps {
  vmId: string;
  vmName: string;
  token?: string;
}

type ConnectionStatus = "connecting" | "connected" | "disconnected" | "error";

export function SerialConsole({
  vmId,
  vmName,
  token,
}: SerialConsoleProps) {
  const terminalRef = useRef<HTMLDivElement>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const terminal = useRef<Terminal | null>(null);
  const fitAddon = useRef<FitAddon | null>(null);
  const [status, setStatus] = useState<ConnectionStatus>("disconnected");
  const [error, setError] = useState<string>("");
  const [reconnectKey, setReconnectKey] = useState(0);
  const [isConnected, setIsConnected] = useState(false);
  const [isRebooting, setIsRebooting] = useState(false);
  const rebootTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Build WebSocket URL
  const getWsUrl = useCallback(() => {
    // Always use wss:// in production. ws:// is only permitted in non-production
    // environments (i.e. NODE_ENV !== 'production') to support local dev without TLS.
    const isDev = process.env.NODE_ENV !== 'production';
    const protocol = isDev && window.location.protocol !== 'https:'
      ? 'ws:'
      : 'wss:';
    const host = window.location.host;
    const tokenParam = token ? `?token=${encodeURIComponent(token)}` : "";
    return `${protocol}//${host}/api/v1/customer/ws/serial/${vmId}${tokenParam}`;
  }, [vmId, token]);

  useEffect(() => {
    if (!terminalRef.current) return;
    if (!isConnected) return;

    const term = new Terminal({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      theme: {
        background: "#0a0a0a",
        foreground: "#e5e5e5",
        cursor: "#22c55e",
        selectionBackground: "#22c55e",
        selectionForeground: "#0a0a0a",
        black: "#000000",
        red: "#ef4444",
        green: "#22c55e",
        yellow: "#eab308",
        blue: "#3b82f6",
        magenta: "#a855f7",
        cyan: "#06b6d4",
        white: "#e5e5e5",
        brightBlack: "#525252",
        brightRed: "#f87171",
        brightGreen: "#4ade80",
        brightYellow: "#facc15",
        brightBlue: "#60a5fa",
        brightMagenta: "#c084fc",
        brightCyan: "#22d3ee",
        brightWhite: "#ffffff",
      },
      scrollback: 10000,
      allowProposedApi: true,
    });

    fitAddon.current = new FitAddon();
    term.loadAddon(fitAddon.current);
    term.loadAddon(new WebLinksAddon());

    term.open(terminalRef.current);
    fitAddon.current.fit();

    term.writeln("\x1b[1;32mVirtueStack Serial Console\x1b[0m");
    term.writeln(`\x1b[90mVM: ${vmName} (${vmId})\x1b[0m`);
    term.writeln("\x1b[90mConnecting to serial port...\x1b[0m");
    term.writeln("");

    setStatus("connecting");
    setError("");

    const ws = new WebSocket(getWsUrl());
    wsRef.current = ws;

    ws.onopen = () => {
      setStatus("connected");
      term.writeln("\x1b[1;32mConnected to serial console.\x1b[0m");
      term.writeln("");
    };

    ws.onmessage = (event) => {
      if (event.data instanceof Blob) {
        event.data.text().then((text) => {
          term.write(text);
        });
      } else {
        term.write(event.data);
      }
    };

    ws.onerror = (event) => {
      setError("Connection error. Please check your network and try again.");
      setStatus("error");
      term.writeln("\x1b[1;31mConnection error.\x1b[0m");
    };

    ws.onclose = (event) => {
      setStatus("disconnected");
      term.writeln("");
      if (!event.wasClean) {
        term.writeln("\x1b[1;31mConnection closed unexpectedly.\x1b[0m");
      } else {
        term.writeln("\x1b[90mDisconnected from serial console.\x1b[0m");
      }
    };

    term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(data);
      }
    });

    terminal.current = term;

    const handleResize = () => {
      if (fitAddon.current) {
        fitAddon.current.fit();
        if (ws.readyState === WebSocket.OPEN && term) {
          const { cols, rows } = term;
          ws.send(JSON.stringify({ type: "resize", cols, rows }));
        }
      }
    };

    window.addEventListener("resize", handleResize);

    return () => {
      window.removeEventListener("resize", handleResize);
      if (rebootTimeoutRef.current) {
        clearTimeout(rebootTimeoutRef.current);
      }
      if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
        ws.close();
      }
      term.dispose();
    };
  }, [vmId, vmName, token, getWsUrl, reconnectKey, isConnected]);

  const handleConnect = () => {
    setIsConnected(true);
  };

  const handleDisconnect = () => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.close();
    }
    setIsConnected(false);
    setStatus("disconnected");
  };

  const handleReconnect = () => {
    if (!isConnected) {
      setIsConnected(true);
    } else {
      setReconnectKey((prev) => prev + 1);
    }
  };

  const handleClear = () => {
    if (terminal.current) {
      terminal.current.clear();
      terminal.current.writeln("\x1b[1;32mVirtueStack Serial Console\x1b[0m");
      terminal.current.writeln(`\x1b[90mVM: ${vmName} (${vmId})\x1b[0m`);
      terminal.current.writeln("");
    }
  };

  const handleReboot = async () => {
    if (terminal.current) {
      terminal.current.writeln("");
      terminal.current.writeln("\x1b[1;33m[ INFO ] System reboot initiated...\x1b[0m");
      terminal.current.writeln("");
    }
    setIsRebooting(true);
    try {
      const { vmApi } = await import("@/lib/api-client");
      await vmApi.restartVM(vmId);
    } catch {
      terminal.current?.writeln("\x1b[1;31m[ ERROR ] Failed to send reboot command via API\x1b[0m");
    }
    handleDisconnect();
    rebootTimeoutRef.current = setTimeout(() => {
      setIsRebooting(false);
      handleReconnect();
    }, 3000);
  };

  const getStatusBadge = () => {
    switch (status) {
      case "connected":
        return (
          <Badge variant="success" className="gap-1.5 font-mono text-xs">
            <Wifi className="h-3 w-3" />
            CONNECTED
          </Badge>
        );
      case "connecting":
        return (
          <Badge variant="warning" className="gap-1.5 font-mono text-xs">
            <Wifi className="h-3 w-3 animate-pulse" />
            CONNECTING
          </Badge>
        );
      case "error":
        return (
          <Badge variant="destructive" className="gap-1.5 font-mono text-xs">
            <WifiOff className="h-3 w-3" />
            ERROR
          </Badge>
        );
      default:
        return (
          <Badge variant="secondary" className="gap-1.5 font-mono text-xs">
            <WifiOff className="h-3 w-3" />
            DISCONNECTED
          </Badge>
        );
    }
  };

  return (
    <Card className="w-full border-border bg-card">
      <CardHeader className="pb-3 border-b border-border">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <TerminalIcon className="h-5 w-5 text-green-500" />
            <CardTitle className="text-lg font-mono">
              Serial Console - {vmName}
            </CardTitle>
          </div>
          <div className="flex items-center gap-2">
            {getStatusBadge()}
            <Button
              variant="outline"
              size="icon"
              onClick={status === "connected" ? handleDisconnect : handleConnect}
              title={status === "connected" ? "Disconnect" : "Connect"}
              className="h-8 w-8"
            >
              {status === "connected" ? (
                <WifiOff className="h-4 w-4" />
              ) : (
                <Wifi className="h-4 w-4" />
              )}
            </Button>
            <Button
              variant="outline"
              size="icon"
              onClick={handleReboot}
              title="Reboot VM"
              className="h-8 w-8"
            >
              <Power className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              size="icon"
              onClick={handleClear}
              title="Clear Terminal"
              className="h-8 w-8"
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </CardHeader>
      <CardContent className="p-0 relative">
        <div
          ref={terminalRef}
          className="font-mono text-sm min-h-[400px] max-h-[600px] overflow-hidden bg-black p-2"
        />
        {error && (
          <div className="absolute inset-0 flex items-center justify-center bg-black/80">
            <div className="text-center p-6">
              <p className="text-red-400 mb-4 font-mono">{error}</p>
              <Button onClick={handleReconnect} variant="outline">
                Retry Connection
              </Button>
            </div>
          </div>
        )}
        {status === "disconnected" && !error && !isConnected && (
            <div className="absolute bottom-4 left-1/2 -translate-x-1/2">
            <Button
              onClick={handleConnect}
              variant="outline"
              size="sm"
              className="gap-2"
            >
              <Wifi className="h-4 w-4" />
              Connect
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

export default SerialConsole;
