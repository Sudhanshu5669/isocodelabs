import { useState, type FormEvent } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import { ApiError } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import './login.css'

// Quick-fill buttons help a reviewer try each role. They fill the email only;
// the demo password is in deployment.md. Shown in development, or when the
// build sets VITE_SHOW_DEMO_LOGINS=true.
const showDemo = import.meta.env.DEV || import.meta.env.VITE_SHOW_DEMO_LOGINS === 'true'
const demoAccounts = [
  { label: 'Sales head', email: 'head@companya.test' },
  { label: 'Executive', email: 'rohan@companya.test' },
]

export default function StaffLogin() {
  const { user, login } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const from = (location.state as { from?: string } | null)?.from ?? '/staff'

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  if (user) {
    return <Navigate to={from} replace />
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setSubmitting(true)
    try {
      await login(email.trim(), password)
      navigate(from, { replace: true })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Something went wrong. Please try again.')
      setSubmitting(false)
    }
  }

  return (
    <main className="login">
      <aside className="login-art" aria-hidden="true">
        <div className="login-art-text">
          <p className="login-brand">COMPANY A</p>
          <p className="login-tagline">Every enquiry answered. Every visit booked.</p>
        </div>
      </aside>

      <section className="login-panel">
        <form className="login-form" onSubmit={onSubmit} noValidate>
          <p className="login-eyebrow">Sales team</p>
          <h1>Sign in</h1>

          <div className="field">
            <label htmlFor="email">Email</label>
            <input
              id="email"
              className="input"
              type="email"
              autoComplete="username"
              inputMode="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
            />
          </div>

          <div className="field">
            <label htmlFor="password">Password</label>
            <input
              id="password"
              className="input"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
            />
          </div>

          <div role="alert" aria-live="polite">
            {error && <p className="alert alert-error">{error}</p>}
          </div>

          <button className="btn" type="submit" disabled={submitting || !email || !password}>
            {submitting ? (
              <>
                <span className="spinner" aria-hidden="true" /> Signing in
              </>
            ) : (
              'Sign in'
            )}
          </button>

          {showDemo && (
            <div className="login-demo">
              <p>Try a demo account</p>
              <div>
                {demoAccounts.map((a) => (
                  <button key={a.email} type="button" className="btn btn-quiet" onClick={() => setEmail(a.email)}>
                    {a.label}
                  </button>
                ))}
              </div>
            </div>
          )}
        </form>
      </section>
    </main>
  )
}
