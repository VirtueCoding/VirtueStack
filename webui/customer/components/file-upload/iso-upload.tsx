"use client"

import * as React from "react"
import { Upload, File, X, Check, AlertCircle } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Progress } from "@/components/ui/progress"
import { Card, CardContent } from "@/components/ui/card"
import { cn } from "@/lib/utils"
import { isoApi, ApiClientError } from "@/lib/api-client"

interface ISOUploadProps {
  vmId: string
  onUploadComplete?: (isoId: string, fileName: string) => void
}

type UploadState = "idle" | "dragOver" | "uploading" | "success" | "error"

interface UploadedFile {
  name: string
  size: number
}

export function ISOUpload({ vmId, onUploadComplete }: ISOUploadProps) {
  const [uploadState, setUploadState] = React.useState<UploadState>("idle")
  const [progress, setProgress] = React.useState(0)
  const [file, setFile] = React.useState<UploadedFile | null>(null)
  const [errorMessage, setErrorMessage] = React.useState<string>("")
  const fileInputRef = React.useRef<HTMLInputElement>(null)
  const abortControllerRef = React.useRef<AbortController | null>(null)

  const MAX_ISO_SIZE_BYTES = 10 * 1024 * 1024 * 1024

  const formatFileSize = (bytes: number): string => {
    if (bytes === 0) return "0 Bytes"
    const k = 1024
    const sizes = ["Bytes", "KB", "MB", "GB", "TB"]
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i]
  }

  const validateFile = (file: File): boolean => {
    if (!file.name.toLowerCase().endsWith(".iso")) {
      setErrorMessage("Only .iso files are allowed")
      setUploadState("error")
      return false
    }
    if (file.size > MAX_ISO_SIZE_BYTES) {
      setErrorMessage("File size exceeds the 10 GB limit")
      setUploadState("error")
      return false
    }
    return true
  }

  const startUpload = async (file: File) => {
    if (!validateFile(file)) return

    setFile({
      name: file.name,
      size: file.size,
    })
    setUploadState("uploading")
    setProgress(0)
    setErrorMessage("")

    const abortController = new AbortController()
    abortControllerRef.current = abortController

    try {
      const result = await isoApi.uploadISO(vmId, file, setProgress, abortController.signal)
      setProgress(100)
      setUploadState("success")
      onUploadComplete?.(result.id, result.file_name)
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") {
        setUploadState("idle")
        setFile(null)
        setProgress(0)
      } else if (err instanceof ApiClientError) {
        setErrorMessage(err.message)
        setUploadState("error")
      } else {
        setErrorMessage("Upload failed. Please try again.")
        setUploadState("error")
      }
    } finally {
      abortControllerRef.current = null
    }
  }

  const handleDragOver = (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    if (uploadState === "idle" || uploadState === "error") {
      setUploadState("dragOver")
    }
  }

  const handleDragLeave = (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    if (uploadState === "dragOver") {
      setUploadState("idle")
    }
  }

  const handleDrop = (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    setUploadState("idle")

    if (uploadState === "uploading" || uploadState === "success") return

    const droppedFiles = e.dataTransfer.files
    if (droppedFiles.length > 0) {
      startUpload(droppedFiles[0])
    }
  }

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const selectedFiles = e.target.files
    if (selectedFiles && selectedFiles.length > 0) {
      startUpload(selectedFiles[0])
    }
  }

  const handleCancel = () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort()
      abortControllerRef.current = null
    }
    setUploadState("idle")
    setProgress(0)
    setFile(null)
    setErrorMessage("")
    if (fileInputRef.current) {
      fileInputRef.current.value = ""
    }
  }

  const handleClick = () => {
    if (uploadState === "idle" || uploadState === "error") {
      fileInputRef.current?.click()
    }
  }

  const getStateStyles = () => {
    switch (uploadState) {
      case "dragOver":
        return "border-primary bg-primary/5 border-2 border-dashed"
      case "success":
        return "border-green-500 bg-green-500/10"
      case "error":
        return "border-destructive bg-destructive/10"
      default:
        return "border-input hover:border-primary/50"
    }
  }

  const renderContent = () => {
    if (uploadState === "uploading") {
      return (
        <div className="w-full space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <File className="h-8 w-8 text-primary" />
              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium text-foreground truncate">
                  {file?.name}
                </p>
                <p className="text-xs text-muted-foreground">
                  {file && formatFileSize(file.size)}
                </p>
              </div>
            </div>
            <Button
              variant="ghost"
              size="icon"
              onClick={handleCancel}
              className="h-8 w-8"
            >
              <X className="h-4 w-4" />
              <span className="sr-only">Cancel upload</span>
            </Button>
          </div>
          <div className="space-y-2">
            <Progress value={progress} className="h-2" />
            <div className="flex justify-between text-xs text-muted-foreground">
              <span>Uploading...</span>
              <span>Do not close this window</span>
            </div>
          </div>
        </div>
      )
    }

    if (uploadState === "success") {
      return (
        <div className="flex flex-col items-center justify-center space-y-3 text-center py-6">
          <div className="h-12 w-12 rounded-full bg-green-500/20 flex items-center justify-center">
            <Check className="h-6 w-6 text-green-500" />
          </div>
          <div className="space-y-1">
            <p className="text-sm font-medium text-foreground">Upload Complete!</p>
            <p className="text-xs text-muted-foreground">
              {file?.name} ({file && formatFileSize(file.size)})
            </p>
          </div>
          <Button variant="outline" size="sm" onClick={handleCancel}>
            Upload Another ISO
          </Button>
        </div>
      )
    }

    if (uploadState === "error") {
      return (
        <div className="flex flex-col items-center justify-center space-y-3 text-center py-6">
          <div className="h-12 w-12 rounded-full bg-destructive/20 flex items-center justify-center">
            <AlertCircle className="h-6 w-6 text-destructive" />
          </div>
          <div className="space-y-1">
            <p className="text-sm font-medium text-foreground">Upload Failed</p>
            <p className="text-xs text-muted-foreground">{errorMessage}</p>
          </div>
          <Button variant="outline" size="sm" onClick={handleCancel}>
            Try Again
          </Button>
        </div>
      )
    }

    // Idle or dragOver state
    return (
      <div
        className="flex flex-col items-center justify-center space-y-4 text-center py-12 cursor-pointer"
        onClick={handleClick}
      >
        <div
          className={cn(
            "h-16 w-16 rounded-full flex items-center justify-center transition-colors",
            uploadState === "dragOver"
              ? "bg-primary/20"
              : "bg-primary/10"
          )}
        >
          <Upload
            className={cn(
              "h-8 w-8 transition-colors",
              uploadState === "dragOver"
                ? "text-primary"
                : "text-primary/70"
            )}
          />
        </div>
        <div className="space-y-1">
          <p className="text-sm font-medium text-foreground">
            {uploadState === "dragOver"
              ? "Drop ISO file here"
              : "Drag & drop ISO file here"}
          </p>
          <p className="text-xs text-muted-foreground">
            or click to browse
          </p>
        </div>
        <p className="text-xs text-muted-foreground max-w-[200px]">
          Only .iso files are accepted (max 10 GB)
        </p>
      </div>
    )
  }

  return (
    <Card className="w-full">
      <CardContent className="p-6">
        <div
          className={cn(
            "relative rounded-lg border-2 transition-all duration-200",
            getStateStyles()
          )}
          onDragOver={handleDragOver}
          onDragLeave={handleDragLeave}
          onDrop={handleDrop}
        >
          <input
            ref={fileInputRef}
            type="file"
            accept=".iso"
            onChange={handleFileSelect}
            className="hidden"
            disabled={uploadState === "uploading" || uploadState === "success"}
          />
          {renderContent()}
        </div>
      </CardContent>
    </Card>
  )
}
