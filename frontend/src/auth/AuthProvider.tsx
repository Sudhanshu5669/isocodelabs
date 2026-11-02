import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { api, ApiError } from '../api/client'
import { AuthContext, type AuthState, type User } from './AuthContext'

// Keeps the signed-in user for the whole app. On first load it asks the server
// whether the browser already has a valid session cookie.
export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const controller = new AbortController()
    api<User>('/auth/me', { signal: controller.signal })
      .then(setUser)
      .catch((err: unknown) => {
        if (err instanceof ApiError && err.status === 401) {
          setUser(null) // simply not signed in
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [])

  const login = useCallback(async (email: string, password: string) => {
    setUser(await api<User>('/auth/login', { method: 'POST', body: { email, password } }))
  }, [])

  const logout = useCallback(async () => {
    await api<void>('/auth/logout', { method: 'POST' }).catch(() => undefined)
    setUser(null)
  }, [])

  const value = useMemo<AuthState>(() => ({ user, loading, login, logout }), [user, loading, login, logout])
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
