import { Link } from 'react-router-dom'

export default function NotFound() {
  return (
    <main style={{ display: 'grid', placeItems: 'center', minHeight: '100vh', padding: 24, textAlign: 'center' }}>
      <div>
        <p style={{ fontSize: 13, fontWeight: 600, letterSpacing: '0.14em', textTransform: 'uppercase', color: 'var(--accent)' }}>
          404
        </p>
        <h1 style={{ fontSize: 44, margin: '8px 0 20px' }}>This page does not exist</h1>
        <Link className="btn" to="/">
          Back to home
        </Link>
      </div>
    </main>
  )
}
