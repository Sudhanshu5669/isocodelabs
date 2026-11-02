import { Outlet, useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'
import './shell.css'

// The frame around every staff screen: brand, who is signed in, sign out.
export default function StaffShell() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()

  async function onLogout() {
    await logout()
    navigate('/staff/login', { replace: true })
  }

  return (
    <div className="shell">
      <header className="shell-header">
        <span className="shell-brand">COMPANY A</span>
        <div className="shell-user">
          <span className="shell-name">{user?.name}</span>
          <span className="shell-role">{user?.role === 'head' ? 'Sales head' : 'Executive'}</span>
          <button className="btn btn-quiet" onClick={onLogout}>
            Sign out
          </button>
        </div>
      </header>
      <main className="shell-main">
        <Outlet />
      </main>
    </div>
  )
}
