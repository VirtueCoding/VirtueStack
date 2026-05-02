"use client"

import { ReactNode, useEffect } from "react"

export interface RequireAuthGateProps {
  children: ReactNode
  isAuthenticated: boolean
  isLoading: boolean
  onUnauthenticated: () => void
}

export function RequireAuthGate({
  children,
  isAuthenticated,
  isLoading,
  onUnauthenticated,
}: RequireAuthGateProps) {
  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      onUnauthenticated()
    }
  }, [isLoading, isAuthenticated, onUnauthenticated])

  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-b-2 border-foreground" />
      </div>
    )
  }

  if (!isAuthenticated) {
    return null
  }

  return <>{children}</>
}
