"use client";

import { useState, useEffect, useRef } from "react";
import { Terminal, Trash2, Power, Wifi, WifiOff } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

interface TerminalLine {
  id: string;
  text: string;
  type: "output" | "input" | "system" | "error";
}

interface SerialConsoleProps {
  vmId?: string;
  vmName?: string;
  isConnected?: boolean;
}

const mockBootSequence = [
  { text: "[ OK ] Started kernel", type: "system" as const },
  { text: "[ OK ] Mounted /dev/vda1", type: "system" as const },
  { text: "[ OK ] Started networking", type: "system" as const },
  { text: "vm login: root", type: "output" as const },
  { text: "Password: ", type: "output" as const },
  { text: "Last login: Wed Mar 11 10:30:00 2026 from 10.0.1.1", type: "output" as const },
  { text: "Welcome to VirtueStack VM Console", type: "output" as const },
  { text: "Type 'help' for available commands.", type: "output" as const },
  { text: "", type: "output" as const },
];

export function SerialConsole({
  vmId = "vm-001",
  vmName = "web-server-prod",
  isConnected = true,
}: SerialConsoleProps) {
  const [lines, setLines] = useState<TerminalLine[]>([]);
  const [currentInput, setCurrentInput] = useState("");
  const [isConnectedState, setIsConnectedState] = useState(isConnected);
  const scrollRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // Auto-scroll to bottom when new lines are added
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [lines]);

  // Initialize with boot sequence
  useEffect(() => {
    const initializeTerminal = async () => {
      // Simulate boot sequence with delays
      for (const line of mockBootSequence) {
        await new Promise((resolve) => setTimeout(resolve, 300));
        setLines((prev) => [
          ...prev,
          {
            id: Math.random().toString(36).substring(7),
            text: line.text,
            type: line.type,
          },
        ]);
      }
      // Add initial prompt
      setLines((prev) => [
        ...prev,
        {
          id: Math.random().toString(36).substring(7),
          text: `root@${vmName}:# `,
          type: "output",
        },
      ]);
    };

    initializeTerminal();
  }, [vmName]);

  const handleCommand = (e: React.FormEvent) => {
    e.preventDefault();
    if (!currentInput.trim()) return;

    // Add user input to terminal
    setLines((prev) => [
      ...prev,
      {
        id: Math.random().toString(36).substring(7),
        text: currentInput,
        type: "input",
      },
    ]);

    // Process command
    processCommand(currentInput);
    setCurrentInput("");

    // Focus input after command
    setTimeout(() => inputRef.current?.focus(), 10);
  };

  const processCommand = (command: string) => {
    const normalizedCommand = command.trim().toLowerCase();
    let response: string[] = [];

    switch (normalizedCommand) {
      case "help":
        response = [
          "Available commands:",
          "  help     - Show this help message",
          "  clear    - Clear terminal screen",
          "  status   - Show VM status",
          "  reboot   - Reboot the system",
          "  whoami   - Show current user",
          "  date     - Show current date/time",
        ];
        break;
      case "clear":
        setLines([]);
        return;
      case "status":
        response = [
          `VM: ${vmName}`,
          `ID: ${vmId}`,
          "Status: running",
          "Uptime: 2h 15m 32s",
        ];
        break;
      case "reboot":
        response = ["Rebooting system...", "[ OK ] System halted"];
        setTimeout(() => {
          setLines([]);
          // Reinitialize after "reboot"
          mockBootSequence.forEach((line, index) => {
            setTimeout(() => {
              setLines((prev) => [
                ...prev,
                {
                  id: Math.random().toString(36).substring(7),
                  text: line.text,
                  type: line.type,
                },
              ]);
            }, index * 300);
          });
        }, 1500);
        break;
      case "whoami":
        response = ["root"];
        break;
      case "date":
        response = [new Date().toString()];
        break;
      default:
        response = [`bash: ${command}: command not found`];
    }

    // Add response to terminal
    setTimeout(() => {
      response.forEach((text) => {
        setLines((prev) => [
          ...prev,
          {
            id: Math.random().toString(36).substring(7),
            text,
            type: text.includes("not found") ? "error" : "output",
          },
        ]);
      });
      // Add new prompt
      setLines((prev) => [
        ...prev,
        {
          id: Math.random().toString(36).substring(7),
          text: `root@${vmName}:# `,
          type: "output",
        },
      ]);
    }, 100);
  };

  const handleClear = () => {
    setLines([]);
    setCurrentInput("");
    inputRef.current?.focus();
  };

  const handleReboot = () => {
    setLines((prev) => [
      ...prev,
      {
        id: Math.random().toString(36).substring(7),
        text: "\n[ INFO ] System reboot initiated...",
        type: "system",
      },
    ]);
    setIsConnectedState(false);
    setTimeout(() => {
      setLines([]);
      setIsConnectedState(true);
      // Reinitialize boot sequence
      mockBootSequence.forEach((line, index) => {
        setTimeout(() => {
          setLines((prev) => [
            ...prev,
            {
              id: Math.random().toString(36).substring(7),
              text: line.text,
              type: line.type,
            },
          ]);
        }, index * 300);
      });
    }, 2000);
  };

  const handleDisconnect = () => {
    setIsConnectedState(!isConnectedState);
  };

  const getLineStyle = (type: TerminalLine["type"]) => {
    switch (type) {
      case "system":
        return "text-cyan-400 font-semibold";
      case "input":
        return "text-white";
      case "error":
        return "text-red-400";
      default:
        return "text-green-400";
    }
  };

  return (
    <Card className="w-full border-border bg-card">
      <CardHeader className="pb-3 border-b border-border">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Terminal className="h-5 w-5 text-green-500" />
            <CardTitle className="text-lg font-mono">
              Serial Console - {vmName}
            </CardTitle>
          </div>
          <div className="flex items-center gap-2">
            <Badge
              variant={isConnectedState ? "success" : "secondary"}
              className="gap-1.5 font-mono text-xs"
            >
              {isConnectedState ? (
                <Wifi className="h-3 w-3" />
              ) : (
                <WifiOff className="h-3 w-3" />
              )}
              {isConnectedState ? "CONNECTED" : "DISCONNECTED"}
            </Badge>
            <Button
              variant="outline"
              size="icon"
              onClick={handleDisconnect}
              title={isConnectedState ? "Disconnect" : "Connect"}
              className="h-8 w-8"
            >
              {isConnectedState ? (
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
      <CardContent className="p-0">
        <div
          className="font-mono text-sm min-h-[400px] max-h-[600px] overflow-y-auto bg-black p-4"
          ref={scrollRef}
          onClick={() => inputRef.current?.focus()}
        >
          {lines.map((line) => (
            <div
              key={line.id}
              className={`${getLineStyle(line.type)} whitespace-pre-wrap break-words leading-relaxed`}
            >
              {line.type === "input" && (
                <span className="text-green-400">{`root@${vmName}:# `}
                </span>
              )}
              {line.text}
            </div>
          ))}

          {/* Input line */}
          {isConnectedState && (
            <form onSubmit={handleCommand} className="flex items-center mt-2">
              <span className="text-green-400 whitespace-nowrap">{`root@${vmName}:# `}</span>
              <input
                ref={inputRef}
                type="text"
                value={currentInput}
                onChange={(e) => setCurrentInput(e.target.value)}
                className="flex-1 bg-transparent text-white outline-none border-none px-2 font-mono"
                autoFocus
                autoComplete="off"
                autoCorrect="off"
                autoCapitalize="off"
                spellCheck="false"
              />
              <span className="w-2 h-5 bg-green-400 animate-pulse" />
            </form>
          )}

          {!isConnectedState && (
            <div className="text-yellow-400 mt-4">
              <div>⚠ Disconnected from console</div>
              <div className="text-sm text-yellow-500 mt-1">
                Click the connect button to reconnect
              </div>
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

export default SerialConsole;
