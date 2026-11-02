import { useAuth } from '../auth/AuthContext'

// Temporary landing screen for staff. It is replaced by the lead list in a
// later phase.
export default function StaffHome() {
  const { user } = useAuth()
  const firstName = user?.name.split(' ')[0]

  return (
    <section>
      <h1 style={{ fontSize: 44, lineHeight: 1.05 }}>Welcome, {firstName}</h1>
      <p style={{ marginTop: 12, color: 'var(--muted)', maxWidth: 520 }}>
        You are signed in as {user?.role === 'head' ? 'the sales head' : 'a sales executive'}. The lead list, visit
        calendar and dashboard will appear here.
      </p>
    </section>
  )
}
