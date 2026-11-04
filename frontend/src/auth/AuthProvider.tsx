import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { api, ApiError, setUnauthorizedHandler } from '../api/client'
import { AuthContext, type AuthState, type User } from './AuthContext'

// Keeps the signed-in user for the whole app. On first load it asks the server
// whether the browser already has a valid session cookie.
export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)

  // Any call that finds the session gone signs the user out of the UI.
  useEffect(() => {
    setUnauthorizedHandler(() => setUser(null))
    return () => setUnauthorizedHandler(null)
  }, [])

  // A failed check (server busy, connection dropped) is not the same as
  // "signed out": it is retried twice, then reported with a retry button,
  // instead of sending a signed-in person back to the login page.
  const [checkError, setCheckError] = useState('')
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout> | undefined
    setLoading(true)
    setCheckError('')

    const check = (tries: number) => {
      api<User>('/auth/me', { signal: controller.signal })
        .then((u) => {
          setUser(u)
          setLoading(false)
        })
        .catch((err: unknown) => {
          if (controller.signal.aborted) return
          if (err instanceof ApiError && err.status === 401) {
            setUser(null) // simply not signed in
            setLoading(false)
          } else if (tries < 2) {
            timer = setTimeout(() => check(tries + 1), 1000 * (tries + 1))
          } else {
            setCheckError(err instanceof ApiError ? err.message : 'Could not reach the server.')
            setLoading(false)
          }
        })
    }
    check(0)
    return () => {
      controller.abort()
      clearTimeout(timer)
    }
  }, [attempt])

  const recheck = useCallback(() => setAttempt((a) => a + 1), [])

  const login = useCallback(async (email: string, password: string) => {
    setUser(await api<User>('/auth/login', { method: 'POST', body: { email, password } }))
  }, [])

  // Signing out must really end the session on the server. If the request
  // fails, the error is thrown and the user stays signed in, so they can try
  // again rather than walk away believing they are signed out. A 401 means the
  // session had already ended, which is a success.
  const logout = useCallback(async () => {
    try {
      await api<void>('/auth/logout', { method: 'POST' })
    } catch (err) {
      if (!(err instanceof ApiError && err.status === 401)) throw err
    }
    setUser(null)
  }, [])

  const value = useMemo<AuthState>(
    () => ({ user, loading, checkError, recheck, login, logout }),
    [user, loading, checkError, recheck, login, logout],
  )
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
