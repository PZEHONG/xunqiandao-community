import { createContext, useContext } from 'react';
import type { Binding, PlatformUser, AuthStatus } from '../api/contracts';
export type LoginPane = 'login' | 'official';
export interface SessionState {
  status: 'loading' | 'anonymous' | 'authenticated'; user: PlatformUser | null; bindings: Binding[]; epoch: number; error: string;
  authConfig: AuthStatus | null; loginOpen: boolean; loginPane: LoginPane; reconnecting: boolean;
  authBusy: boolean; loginKey: number; configCleared: boolean; message: string; clearConfig: () => void;
  restore: () => Promise<void>; logout: () => Promise<void>; openLogin: (pane?: LoginPane) => void; closeLogin: () => void; reconnect: () => void;
  authenticate: (pane: LoginPane, data: Record<string, unknown>) => Promise<void>;
}
export const SessionContext = createContext<SessionState | null>(null);
export function useSession() { const value = useContext(SessionContext); if (!value) throw new Error('需要 SessionProvider'); return value; }
