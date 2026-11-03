// One small wrapper around fetch, so every API call handles JSON, cookies and
// errors the same way.

export class ApiError extends Error {
  status: number

  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

// Called whenever the server says the session is gone (401) on a call that
// needed one, for example after the 12-hour session expires. AuthProvider
// registers a handler that clears the signed-in user, so the app shows the
// login screen instead of a page full of errors.
let onUnauthorized: (() => void) | null = null

export function setUnauthorizedHandler(handler: (() => void) | null) {
  onUnauthorized = handler
}

type Options = {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  body?: unknown
  signal?: AbortSignal
}

export async function api<T>(path: string, { method = 'GET', body, signal }: Options = {}): Promise<T> {
  let res: Response
  try {
    res = await fetch(`/api${path}`, {
      method,
      signal,
      credentials: 'same-origin', // sends the session cookie
      headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  } catch {
    // The request never reached the server (offline, server down).
    throw new ApiError('Could not reach the server. Check your connection and try again.', 0)
  }

  if (res.status === 204) {
    return undefined as T
  }

  const data: unknown = await res.json().catch(() => null)
  if (!res.ok) {
    // A 401 from the login form just means a wrong password, not an expired session.
    if (res.status === 401 && path !== '/auth/login') {
      onUnauthorized?.()
    }
    const message =
      data && typeof data === 'object' && 'error' in data && typeof data.error === 'string'
        ? data.error
        : 'Something went wrong. Please try again.'
    throw new ApiError(message, res.status)
  }
  return data as T
}
