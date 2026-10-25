import { Alert, type AlertProps } from '@mui/material';
export function Feedback({ severity = 'info', busy = false, uncertain = false, children, ...props }: AlertProps & { busy?: boolean; uncertain?: boolean }) {
  const tone = uncertain ? 'warning' : severity;
  return <Alert {...props} severity={tone} role={busy ? 'status' : props.role ?? 'alert'} aria-busy={busy || undefined}>{children}</Alert>;
}
