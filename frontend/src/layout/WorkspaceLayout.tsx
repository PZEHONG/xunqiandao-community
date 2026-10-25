// Berry Free MainLayout structure adapted to existing Chinese workflows (MIT).
import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useLocation } from 'react-router-dom';
import { TopBar } from './TopBar';
import { DesktopSidebar } from './DesktopSidebar';
import { MobileNav } from './MobileNav';
import { navigation } from '../app/navigation';
import './workspace.css';
export function WorkspaceLayout({ children, actions }: { children: ReactNode; actions?: ReactNode }) {
  const route = useLocation(); const main = useRef<HTMLElement>(null); const [collapsed, setCollapsed] = useState(false);
  const title = navigation.find(item => item.path === route.pathname)?.label ?? '签到记录';
  useEffect(() => { document.title = `${title} · 讯签到`; main.current?.focus(); }, [route.pathname, title]);
  return <div className="workspace" data-collapsed={collapsed}><a className="skip-link" href="#workspace-main" onClick={event => { event.preventDefault(); main.current?.focus(); }}>跳到主要内容</a><TopBar title={title} collapsed={collapsed} toggle={() => setCollapsed(!collapsed)} actions={actions} /><DesktopSidebar collapsed={collapsed} /><main ref={main} id="workspace-main" tabIndex={-1}><h1 className="sr-only">{title}</h1>{children}</main><MobileNav /></div>;
}
