import { useState } from 'react'
import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { ApiError } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import './shell.css'

// The frame around every staff screen: brand, the screens this role may use,
// who is signed in, sign out.

type NavItem = { to: string; label: string; end?: boolean }

const everyone: NavItem[] = [
  { to: '/staff', label: 'Leads', end: true },
  { to: '/staff/visits', label: 'Visits' },
  { to: '/staff/reminders', label: 'Reminders' },
]
const headOnly: NavItem[] = [
  { to: '/staff/dashboard', label: 'Dashboard' },
  { to: '/staff/review', label: 'Review' },
  { to: '/staff/settings', label: 'Settings' },
]

export default function StaffShell() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()

  const [logoutError, setLogoutError] = useState('')

  async function onLogout() {
    setLogoutError('')
    try {
      await logout()
      navigate('/staff/login', { replace: true })
    } catch (err) {
      setLogoutError(
        err instanceof ApiError && err.status !== 0
          ? 'Sign-out did not complete on the server. Please try again.'
          : 'Could not reach the server, so you are still signed in. Check your connection and try again.',
      )
    }
  }

  // Hiding a link is only for tidiness: the server checks the role on every call.
  const links = user?.role === 'head' ? [...everyone, ...headOnly] : everyone

  return (
    <div className="shell">
      <header className="shell-side">
        <div className="shell-brand">
          <span className="shell-brand-name">Company A</span>
          <span className="shell-brand-sub">Sales desk</span>
        </div>
        <nav className="shell-nav" aria-label="Staff screens">
          {links.map((l) => (
            <NavLink key={l.to} to={l.to} end={l.end}>
              {l.label}
            </NavLink>
          ))}
        </nav>
        <div className="shell-user">
          <span className="shell-signed">Signed in as</span>
          <span className="shell-name">{user?.name}</span>
          <span className="shell-role">{user?.role === 'head' ? 'Sales head' : 'Executive'}</span>
          <button className="btn btn-quiet btn-small" onClick={onLogout}>
            Sign out
          </button>
        </div>
      </header>
      <div className="shell-body">
        {logoutError && (
          <p className="alert alert-error shell-alert" role="alert">
            {logoutError}
          </p>
        )}
        <main className="shell-main">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
