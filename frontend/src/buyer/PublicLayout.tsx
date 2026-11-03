import { Link, Outlet } from 'react-router-dom'
import type { Project } from '../api/types'
import { useApi } from '../lib/useApi'
import './public.css'

// The frame around every buyer page: brand, the way to the assistant, and a
// footer that names each project's RERA registration number, as promotional
// material must.
export default function PublicLayout() {
  const projects = useApi<{ projects: Project[] }>('/projects')

  return (
    <div className="pub">
      <header className="pub-header">
        <Link to="/" className="pub-brand">
          <span className="pub-brand-name">COMPANY A</span>
          <span className="pub-brand-place">Ahmedabad</span>
        </Link>
        <nav className="pub-nav" aria-label="Main">
          <Link to="/">Homes</Link>
          <Link to="/chat" className="btn btn-small">
            Ask the assistant
          </Link>
        </nav>
      </header>
      <Outlet />
      <footer className="pub-footer">
        <p>
          {projects.data?.projects.map((p) => (
            <span key={p.id}>
              {p.name} RERA registration no. {p.reraNumber}.{' '}
            </span>
          ))}
          Photographs and 3D models are indicative; furniture and finishes may differ from the delivered home. Prices are
          indicative; the final price, any offer and availability are confirmed by the sales team.
        </p>
        <p>
          <Link to="/staff">Sales team sign in</Link>
        </p>
      </footer>
    </div>
  )
}
