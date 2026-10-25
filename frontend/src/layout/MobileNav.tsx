import { NavLink } from 'react-router-dom';
import { navigation } from '../app/navigation';
export function MobileNav() { return <nav className="mobile-nav" aria-label="手机导航">{navigation.map(({ path, label, icon: Icon }) => <NavLink key={path} to={path} aria-label={label}><Icon aria-hidden="true" /><span>{label}</span></NavLink>)}</nav>; }
