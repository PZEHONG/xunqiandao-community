import { NavLink } from 'react-router-dom';
import { navigation } from '../app/navigation';
export function DesktopSidebar({ collapsed }: { collapsed: boolean }) {
  return <aside className="desktop-sidebar" aria-label="工作台侧栏"><nav aria-label="主导航">{navigation.map(({ path, label, icon: Icon }) => <NavLink key={path} to={path} title={collapsed ? label : undefined} aria-label={label}><Icon aria-hidden="true" /><span>{label}</span></NavLink>)}</nav></aside>;
}
