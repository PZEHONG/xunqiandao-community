import { Chip } from '@mui/material';
export function StatusChip({ label, tone = 'default' }: { label: string; tone?: 'default' | 'success' | 'warning' | 'error' | 'info' }) {
  return <Chip role="status" label={label} color={tone} size="small" variant="outlined" sx={{ maxWidth: '100%', height: 'auto', minHeight: 28, '& .MuiChip-label': { whiteSpace: 'normal', overflowWrap: 'anywhere', py: .5 } }} />;
}
