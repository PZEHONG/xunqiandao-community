import { Button, Dialog, DialogActions, DialogContent, DialogTitle, Typography } from '@mui/material';
import { Link } from 'react-router-dom';
import type { SignResult } from './types';
export function SignSuccessDialog({ result, close }: { result: SignResult | null; close: () => void }) {
  return <Dialog open={result?.status === 'success'} onClose={close} aria-labelledby="sign-success-title"><DialogTitle id="sign-success-title">签到成功</DialogTitle><DialogContent><Typography sx={{ overflowWrap: 'anywhere' }}>{result && !['签到成功', '签到提交成功'].includes(result.message) ? result.message : '本次签到已完成，可在签到记录中查看。'}</Typography></DialogContent><DialogActions><Button component={Link} to="/records" onClick={close}>查看签到记录</Button><Button variant="contained" onClick={close}>完成</Button></DialogActions></Dialog>;
}
