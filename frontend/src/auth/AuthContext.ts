import { createContext, useContext } from 'react'

export type Role = 'executive' | 'head'

export type User = {
  id: string
  email: string
  name: string
  role: Role
}

export type AuthState = {
  user: User | null
  /** True until we know whether a session already exists. */
  loading: boolean
  /** Set when the server could not say whether we are signed in (not a 401). */
  checkError: string
  /** Asks the server again after checkError. */
  recheck: () => void
  login: (email: string, password: string) => Promise<void>
  logout: () => Promise<void>
}

export const AuthContext = createContext<AuthState | null>(null)

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth must be used inside <AuthProvider>')
  }
  return ctx
}
