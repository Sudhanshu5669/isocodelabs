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
