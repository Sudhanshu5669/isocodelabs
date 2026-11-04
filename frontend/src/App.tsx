import { BrowserRouter, Route, Routes } from 'react-router-dom'
import { AuthProvider } from './auth/AuthProvider'
import { RequireAuth } from './auth/RequireAuth'
import CataloguePage from './buyer/CataloguePage'
import ChatPage from './buyer/ChatPage'
import PublicLayout from './buyer/PublicLayout'
import UnitPage from './buyer/UnitPage'
import VisitPage from './buyer/VisitPage'
import NotFound from './pages/NotFound'
import StaffLogin from './pages/StaffLogin'
import StaffShell from './pages/StaffShell'
import DashboardPage from './staff/DashboardPage'
import LeadDetailPage from './staff/LeadDetailPage'
import LeadsPage from './staff/LeadsPage'
import RemindersPage from './staff/RemindersPage'
import ReviewPage from './staff/ReviewPage'
import SettingsPage from './staff/SettingsPage'
import VisitsPage from './staff/VisitsPage'

export default function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <Routes>
          {/* Buyers: no sign-in. */}
          <Route element={<PublicLayout />}>
            <Route path="/" element={<CataloguePage />} />
            <Route path="/homes/:code" element={<UnitPage />} />
            <Route path="/chat" element={<ChatPage />} />
            <Route path="/visit/:code" element={<VisitPage />} />
          </Route>

          {/* Sales team. */}
          <Route path="/staff/login" element={<StaffLogin />} />
          <Route element={<RequireAuth />}>
            <Route path="/staff" element={<StaffShell />}>
              <Route index element={<LeadsPage />} />
              <Route path="leads/:id" element={<LeadDetailPage />} />
              <Route path="visits" element={<VisitsPage />} />
              <Route path="reminders" element={<RemindersPage />} />
              {/* Sales head only. The server checks the role on every call too. */}
              <Route element={<RequireAuth roles={['head']} />}>
                <Route path="dashboard" element={<DashboardPage />} />
                <Route path="review" element={<ReviewPage />} />
                <Route path="settings" element={<SettingsPage />} />
              </Route>
            </Route>
          </Route>
          <Route path="*" element={<NotFound />} />
        </Routes>
      </AuthProvider>
    </BrowserRouter>
  )
}
