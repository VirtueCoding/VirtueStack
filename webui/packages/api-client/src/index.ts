export interface AuthTokens {
  token_type: string
  expires_in: number
  requires_2fa?: boolean
  temp_token?: string
  session_id?: string
  session_cleanup_token?: string
}

export interface LoginRequest {
  email: string
  password: string
}

export interface Verify2FARequest {
  temp_token: string
  totp_code: string
}

export class ApiClientError extends Error {
  public readonly code: string
  public readonly status: number
  public readonly correlationId?: string

  constructor(message: string, code: string, status: number, correlationId?: string) {
    super(message)
    this.name = "ApiClientError"
    this.code = code
    this.status = status
    this.correlationId = correlationId
  }
}

const requestTimeoutMs = 10_000

interface LogoutWithRefreshRecoveryOptions {
  invalidateSession: () => Promise<void>
  refreshSession: () => Promise<unknown>
}

function isUnauthorizedError(error: unknown): error is ApiClientError {
  return error instanceof ApiClientError && error.status === 401
}

function isMissingRefreshSessionError(error: unknown): error is ApiClientError {
  return error instanceof ApiClientError && (error.status === 400 || error.status === 401)
}

export async function logoutWithRefreshRecovery(
  options: LogoutWithRefreshRecoveryOptions
): Promise<void> {
  try {
    await options.invalidateSession()
    return
  } catch (error) {
    if (!isUnauthorizedError(error)) {
      throw error
    }
  }

  try {
    await options.refreshSession()
  } catch (error) {
    if (isMissingRefreshSessionError(error)) {
      return
    }
    throw error
  }

  await options.invalidateSession()
}

export function getCsrfToken(): string | null {
  if (typeof document === "undefined") return null
  const match = document.cookie.match(/csrf_token=([^;]+)/)
  return match ? decodeURIComponent(match[1]) : null
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null
}

export function parseUnreadCountEventData(raw: string): number | null {
  try {
    const parsed: unknown = JSON.parse(raw)
    if (!isRecord(parsed)) {
      return null
    }

    const count = parsed["count"]
    if (typeof count !== "number" || !Number.isSafeInteger(count) || count < 0) {
      return null
    }

    return count
  } catch {
    return null
  }
}

export async function fetchCsrfToken(
  apiBaseURL: string,
  csrfPath: string,
  signal?: AbortSignal,
): Promise<void> {
  await fetchWithTimeout(`${apiBaseURL}${csrfPath}`, {
    method: "GET",
    credentials: "include",
    signal,
  })
}

function csrfPathForEndpoint(apiBaseURL: string, endpoint: string): string | null {
  const normalizedEndpoint = endpoint.startsWith(apiBaseURL)
    ? endpoint.slice(apiBaseURL.length)
    : endpoint

  if (normalizedEndpoint.startsWith("/customer/")) {
    return "/customer/auth/csrf"
  }
  if (normalizedEndpoint.startsWith("/admin/")) {
    return "/admin/auth/csrf"
  }

  return null
}

async function ensureCsrfForStateChangingRequest(
  apiBaseURL: string,
  endpoint: string,
  isStateChanging: boolean,
  signal?: AbortSignal,
): Promise<void> {
  if (!isStateChanging || typeof document === "undefined" || getCsrfToken()) {
    return
  }

  const csrfPath = csrfPathForEndpoint(apiBaseURL, endpoint)
  if (!csrfPath) {
    return
  }

  await fetchCsrfToken(apiBaseURL, csrfPath, signal)
}

async function fetchWithTimeout(url: string, config: RequestInit): Promise<Response> {
  const controller = new AbortController()
  let abortSource: "caller" | "timeout" | null = null
  const markAbortSource = (source: "caller" | "timeout") => {
    if (abortSource === null) {
      abortSource = source
    }
  }

  const callerAbortListener = () => markAbortSource("caller")
  const timeoutAbortListener = () => markAbortSource("timeout")

  if (config.signal?.aborted) {
    markAbortSource("caller")
  } else {
    config.signal?.addEventListener("abort", callerAbortListener, { once: true })
  }

  controller.signal.addEventListener("abort", timeoutAbortListener, { once: true })

  const timeoutID = setTimeout(() => {
    markAbortSource("timeout")
    controller.abort()
  }, requestTimeoutMs)
  const signal = config.signal
    ? AbortSignal.any([config.signal, controller.signal])
    : controller.signal

  try {
    return await fetch(url, {
      ...config,
      signal,
    })
  } catch (networkErr) {
    const isAbort = networkErr instanceof DOMException && networkErr.name === "AbortError"
    const isCallerAbort = isAbort && abortSource === "caller"

    if (isCallerAbort) {
      throw networkErr
    }

    throw new ApiClientError(
      isAbort ? "Request timed out" : "Network error: unable to reach the server",
      isAbort ? "REQUEST_TIMEOUT" : "NETWORK_ERROR",
      0
    )
  } finally {
    clearTimeout(timeoutID)
    config.signal?.removeEventListener("abort", callerAbortListener)
    controller.signal.removeEventListener("abort", timeoutAbortListener)
  }
}

export function buildHeaders(includeCsrf = false): HeadersInit {
  const headers: HeadersInit = {
    "Content-Type": "application/json",
    Accept: "application/json",
  }

  if (includeCsrf) {
    const csrfToken = getCsrfToken()
    if (csrfToken) {
      headers["X-CSRF-Token"] = csrfToken
    }
  }

  return headers
}

export async function parseError(response: Response): Promise<ApiClientError> {
  let code = "UNKNOWN_ERROR"
  let message = "An unexpected error occurred"
  let correlationId: string | undefined

  try {
    const data = await response.json()
    if (data.error) {
      code = data.error.code || code
      message = data.error.message || message
      correlationId = data.error.correlation_id
    }
  } catch {
    message = response.statusText || message
  }

  return new ApiClientError(message, code, response.status, correlationId)
}

export async function apiRequest<T>(
  apiBaseURL: string,
  endpoint: string,
  options: RequestInit = {}
): Promise<T> {
  const url = `${apiBaseURL}${endpoint}`
  const isStateChanging = ["POST", "PUT", "PATCH", "DELETE"].includes(
    (options.method || "GET").toUpperCase()
  )
  const requestSignal = options.signal ?? undefined

  await ensureCsrfForStateChangingRequest(
    apiBaseURL,
    endpoint,
    isStateChanging,
    requestSignal,
  )

  const config: RequestInit = {
    ...options,
    credentials: "include",
    headers: {
      ...buildHeaders(isStateChanging),
      ...options.headers,
    },
  }

  const response = await fetchWithTimeout(url, config)

  if (!response.ok) {
    throw await parseError(response)
  }

  if (response.status === 204) {
    return undefined as unknown as T
  }

  const data = await response.json()
  return data.data as T
}
