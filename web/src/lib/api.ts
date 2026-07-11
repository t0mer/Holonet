// Typed fetch wrapper. Same-origin, cookie-session auth (credentials:include),
// JSON in/out, throws ApiError on non-2xx. A 401 on a protected call signals
// the session lapsed — listeners redirect to the login screen.

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
    this.name = 'ApiError'
  }
}

type Unauthorized = () => void
let onUnauthorized: Unauthorized | null = null

/** Register a handler invoked whenever a protected request returns 401. */
export function setUnauthorizedHandler(fn: Unauthorized) {
  onUnauthorized = fn
}

const BASE = '/api/v1'

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(BASE + path, {
    method,
    credentials: 'include',
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })

  // 401 on anything but the auth probes means the session lapsed.
  if (res.status === 401 && !path.startsWith('/auth/')) {
    onUnauthorized?.()
  }

  if (res.status === 204) {
    return undefined as T
  }

  const text = await res.text()
  const data = text ? JSON.parse(text) : undefined

  if (!res.ok) {
    const message = (data && (data.error as string)) || `Request failed (${res.status})`
    throw new ApiError(res.status, message)
  }
  return data as T
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body),
  del: <T>(path: string) => request<T>('DELETE', path),
}
