import type { ReactNode } from 'react';
import { AppBar, Toolbar, IconButton, Typography } from '@mui/material';
import MenuOutlined from '@mui/icons-material/MenuOutlined';
import { Link } from 'react-router-dom';
export function TopBar({ title, toggle, collapsed, actions }: { title: string; toggle: () => void; collapsed: boolean; actions?: ReactNode }) {
  return <AppBar position="fixed" color="inherit" elevation={0} sx={{ borderBottom: '1px solid', borderColor: 'divider' }}><Toolbar sx={{ minHeight: '64px !important', gap: { xs: 1, sm: 2 }, px: { xs: 1.5, sm: 2.5 } }}>
    <Link className="brand" to="/" aria-label="讯签到首页"><img width="40" height="40" src="/next/xunqiandao-mascot-transparent.png" alt="" /><span>讯签到</span></Link>
    <IconButton className="sidebar-toggle" aria-label={collapsed ? '展开侧栏' : '收起侧栏'} onClick={toggle}><MenuOutlined /></IconButton>
    <Typography className="current-page" sx={{ fontSize: 14, fontWeight: 600, flex: 1 }}>{title}</Typography><div className="account-actions">{actions}</div>
  </Toolbar></AppBar>;
}
