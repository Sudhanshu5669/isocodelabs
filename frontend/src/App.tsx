import { BrowserRouter, Link, Route, Routes } from 'react-router-dom'
import { AuthProvider } from './auth/AuthProvider'
import { RequireAuth } from './auth/RequireAuth'
import NotFound from './pages/NotFound'
import StaffHome from './pages/StaffHome'
import StaffLogin from './pages/StaffLogin'
import StaffShell from './pages/StaffShell'

// Placeholder for the public chat assistant, built in a later phase.
function Home() {
  return (
    <main style={{ display: 'grid', placeItems: 'center', minHeight: '100vh', padding: 24, textAlign: 'center' }}>
      <div>
        <p style={{ fontSize: 14, fontWeight: 700, letterSpacing: '0.2em' }}>COMPANY A</p>
        <h1 style={{ fontSize: 48, margin: '12px 0 20px', lineHeight: 1.05 }}>Your new home, one conversation away</h1>
        <Link className="btn btn-quiet" to="/staff">
          Sales team sign in
        </Link>
      </div>
    </main>
  )
}

export default function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/staff/login" element={<StaffLogin />} />
          <Route element={<RequireAuth />}>
            <Route path="/staff" element={<StaffShell />}>
              <Route index element={<StaffHome />} />
            </Route>
          </Route>
          <Route path="*" element={<NotFound />} />
        </Routes>
      </AuthProvider>
    </BrowserRouter>
  )
}
