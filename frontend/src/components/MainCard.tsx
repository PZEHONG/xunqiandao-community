// Adapted from Berry Free MainCard.jsx (MIT); see THIRD_PARTY.md.
import type { ReactNode } from 'react';
import { Card, CardContent, CardHeader, Divider } from '@mui/material';
export function MainCard({ title, actions, children }: { title?: string; actions?: ReactNode; children: ReactNode }) {
  return <Card sx={{ minWidth: 0 }}>{title && <><CardHeader title={title} action={actions} sx={{ '& .MuiCardHeader-action': { mr: 0 } }} /><Divider /></>}<CardContent sx={{ p: { xs: 2, sm: 3 }, '&:last-child': { pb: { xs: 2, sm: 3 } } }}>{children}</CardContent></Card>;
}
