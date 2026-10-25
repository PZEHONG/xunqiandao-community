import { Navigate, Route, Routes, useLocation } from 'react-router-dom';
import { Button } from '@mui/material';
import { WorkspaceLayout } from '../layout/WorkspaceLayout';
import { AccountConnection } from '../layout/AccountConnection';
import { useSession } from '../session/useSession';
import { AuthDialog } from '../features/auth/AuthDialog';
import { RecordsPage } from '../features/records/RecordsPage';
import { SignPage } from '../features/sign/SignPage';
import { ReportsPage } from '../features/reports/ReportsPage';
import { Feedback } from '../components/Feedback';
import { AccountPage } from '../features/account/AccountPage';
import { normalizeLegacyHash } from './legacyHash';
import { LandingPage } from '../features/landing/LandingPage';

function LegacyRedirect() { const location = useLocation(); const normalized = normalizeLegacyHash(`#${location.pathname}`); return <Navigate to={`${normalized.path}${normalized.signMode ? '?mode=daily' : ''}`} replace />; }
export function App() { return <WorkspaceApp />; }
function WorkspaceApp() {
  const session = useSession();
  const location = useLocation();
  if (location.pathname === '/') return <LandingPage />;
  return <WorkspaceLayout actions={<><AccountConnection /></>}>
    {session.error && <Feedback severity="error" sx={{ mb: 2 }} action={<Button disabled={session.authBusy} onClick={() => void session.restore()}>重试</Button>}>{session.error}</Feedback>}
    <div key={session.epoch}><Routes><Route path="/records" element={<RecordsPage />} /><Route path="/sign" element={<SignPage />} /><Route path="/reports/week" element={<ReportsPage key="week" kind="week" />} /><Route path="/reports/month" element={<ReportsPage key="month" kind="month" />} /><Route path="/account" element={<AccountPage />} /><Route path="*" element={<LegacyRedirect />} /></Routes></div><AuthDialog />
  </WorkspaceLayout>;
}
