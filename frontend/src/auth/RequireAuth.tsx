import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { useAuth, type Role } from './AuthContext'

// Wraps staff routes. Signed-out visitors go to the login page and come back
// to where they were headed afterwards. `roles` limits a route to certain roles.
//
// This only controls what the screen shows. The server checks the role again on
// every API call, so hiding a link is never the real protection.
export function RequireAuth({ roles }: { roles?: Role[] }) {
  const { user, loading } = useAuth()
  const location = useLocation()

  if (loading) {
    return (
      <div role="status" style={{ display: 'grid', placeItems: 'center', minHeight: '60vh', color: 'var(--muted)' }}>
        <span className="spinner" aria-hidden="true" />
        <span className="visually-hidden">Loading</span>
      </div>
    )
  }
  if (!user) {
    return <Navigate to="/staff/login" replace state={{ from: location.pathname }} />
  }
  if (roles && !roles.includes(user.role)) {
    return <Navigate to="/staff" replace />
  }
  return <Outlet />
}
