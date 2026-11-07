import { Link, Outlet, useLocation } from 'react-router-dom'
import type { Project } from '../api/types'
import { useApi } from '../lib/useApi'
import './public.css'

// The frame around every buyer page: brand, the way to the assistant, and a
// footer that names each project's RERA registration number, as promotional
// material must. On the home page the header floats over the photo hero.
export default function PublicLayout() {
  const projects = useApi<{ projects: Project[] }>('/projects')
  const onHome = useLocation().pathname === '/'

  return (
    <div className="pub">
      <header className={onHome ? 'pub-header is-over' : 'pub-header'}>
        <Link to="/" className="pub-brand">
          Company A
        </Link>
        <nav className="pub-nav" aria-label="Main">
          <Link to="/">Homes</Link>
          <Link to="/chat">Ask a question</Link>
          <Link to="/chat" className="btn btn-small">
            Book a visit <span aria-hidden="true">→</span>
          </Link>
        </nav>
      </header>
      <Outlet />
      <footer className="pub-footer">
        <div className="pub-footer-inner">
          <span className="pub-footer-brand">Company A</span>
          <div>
            <p className="pub-rera">
              {projects.data?.projects.map((p) => `${p.name} RERA registration no. ${p.reraNumber}`).join(' · ')}
            </p>
            <p>
              Photographs and 3D models are indicative; furniture and finishes may differ from the delivered home. Prices
              are indicative; the final price, any offer and availability are confirmed by the sales team.
            </p>
          </div>
          <p>
            <Link to="/staff">Sales team sign in</Link>
          </p>
        </div>
      </footer>
    </div>
  )
}
