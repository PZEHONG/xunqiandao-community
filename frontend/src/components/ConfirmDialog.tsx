import type { ReactNode } from 'react';
import { useId } from 'react';
import { Button, Dialog, DialogActions, DialogContent, DialogTitle } from '@mui/material';
export function ConfirmDialog({ open, title, children, busy, confirmLabel, danger = false, cancel, confirm }: { open: boolean; title: string; children: ReactNode; busy: boolean; confirmLabel: string; danger?: boolean; cancel: () => void; confirm: () => void }) {
  const id = useId();
  return <Dialog open={open} onClose={() => { if (!busy) cancel(); }} disableEscapeKeyDown={busy} aria-labelledby={`${id}-title`} aria-describedby={`${id}-body`}><DialogTitle id={`${id}-title`}>{title}</DialogTitle><DialogContent dividers id={`${id}-body`} sx={{ overflowWrap: 'anywhere' }}>{children}</DialogContent><DialogActions sx={{ px: 3, py: 2, gap: 1, flexWrap: 'wrap' }}><Button disabled={busy} onClick={cancel}>取消</Button><Button disabled={busy} variant="contained" color={danger ? 'error' : 'primary'} onClick={confirm}>{busy ? '处理中…' : confirmLabel}</Button></DialogActions></Dialog>;
}
