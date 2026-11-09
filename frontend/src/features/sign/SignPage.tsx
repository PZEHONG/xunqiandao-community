import { Feedback } from '../../components/Feedback';
import { Box, Button, Stack } from '@mui/material';
import { useSearchParams } from 'react-router-dom';
import { MainCard } from '../../components/MainCard';
import { LocationPicker } from '../locations/LocationPicker';
import { useLocation } from '../locations/useLocation';
import { useSign } from './useSign';
import { SignForm } from './SignForm';
import { SignConfirmDialog } from './SignConfirmDialog';
import { SignSuccessDialog } from './SignSuccessDialog';
import { useSession } from '../../session/useSession';
import { useEffect, useState } from 'react';
import { AutoSignPanel } from '../autosign/AutoSignPanel';
export function SignPage() {
  const [params, setParams] = useSearchParams(); const daily = params.get('mode') === 'daily'; const [busy, setBusy] = useState(false); const session = useSession();
  return <Stack gap={2}>
    <Stack direction="row" gap={1}><Button disabled={busy} aria-pressed={!daily} variant={daily ? 'outlined' : 'contained'} onClick={() => setParams({})}>立即签到</Button><Button disabled={busy} aria-pressed={daily} variant={daily ? 'contained' : 'outlined'} onClick={() => setParams({ mode: 'daily' })}>每日自动签到</Button></Stack>
    {session.status !== 'authenticated' || !session.bindings.length ? <MainCard><Stack gap={2}><Feedback severity="info">请先登录并连接习讯云账号，再选择地点签到。</Feedback><Button onClick={() => session.user ? session.reconnect() : session.openLogin()} sx={{ alignSelf: 'flex-start' }}>{session.user ? '连接习讯云' : '登录'}</Button></Stack></MainCard> : daily ? <AutoSignPanel onBusy={setBusy} /> : <ImmediateSign onBusy={setBusy} />}
  </Stack>;
}
function ImmediateSign({ onBusy }: { onBusy: (value: boolean) => void }) {
  const location = useLocation(); const sign = useSign(location);
  useEffect(() => { onBusy(sign.busy); return () => onBusy(false); }, [sign.busy, onBusy]);
  return <>
      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'minmax(0, 1fr)', lg: 'minmax(0, 1.35fr) minmax(0, 1fr)' }, gap: 2, alignItems: 'start' }}>
        <MainCard title="签到地点"><Box component="fieldset" disabled={sign.busy} sx={{ border: 0, m: 0, p: 0, minWidth: 0 }}><LocationPicker location={location} disabled={sign.busy} /></Box></MainCard>
        <MainCard title="签到信息"><SignForm sign={sign} locationReady={!!location.confirmed && !location.pending} /></MainCard>
      </Box>
    <SignConfirmDialog snapshot={sign.confirmation} busy={sign.busy} progress={sign.progress} cancel={sign.cancelConfirmation} submit={sign.submitConfirmed} />
    <SignSuccessDialog result={sign.result} close={sign.dismissResult} />
  </>;
}
