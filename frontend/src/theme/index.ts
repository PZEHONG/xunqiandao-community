// Adapted from Berry Free vite/src/themes/index.jsx (MIT); see THIRD_PARTY.md.
import { createTheme } from '@mui/material/styles';
import { tokens as t } from './tokens';
export const theme = createTheme({
  palette: { mode: 'light', primary: { main: t.primary, dark: t.primaryDark, light: t.primarySoft }, background: { default: t.background, paper: t.paper }, text: { primary: t.text, secondary: t.secondary }, divider: t.divider, error: { main: t.error }, success: { main: t.success } },
  shape: { borderRadius: 10 },
  typography: { fontFamily: 'system-ui,"PingFang SC","Microsoft YaHei",sans-serif', fontSize: 14, h1: { fontSize: '1.75rem', fontWeight: 650 }, h2: { fontSize: '1.25rem', fontWeight: 650 }, h3: { fontSize: '1.0625rem', fontWeight: 650 }, button: { textTransform: 'none', fontWeight: 600 } },
  components: {
    MuiButton: { defaultProps: { disableElevation: true }, styleOverrides: { root: { minHeight: 44, borderRadius: 8 } } },
    MuiIconButton: { styleOverrides: { root: { minHeight: 44, minWidth: 44 } } },
    MuiTextField: { defaultProps: { size: 'small', fullWidth: true } },
    MuiOutlinedInput: { styleOverrides: { root: { minHeight: 44 }, notchedOutline: { borderColor: t.inputBorder }, input: { '@media(max-width:760px)': { fontSize: 16 } } } },
    MuiCard: { defaultProps: { elevation: 0 }, styleOverrides: { root: { border: `1px solid ${t.divider}`, borderRadius: 10 } } },
    MuiCardHeader: { styleOverrides: { title: { fontSize: 17, fontWeight: 650 } } },
    MuiDialog: { defaultProps: { fullWidth: true, maxWidth: 'sm' }, styleOverrides: { paper: { margin: 16, width: 'calc(100% - 32px)', maxHeight: 'calc(100% - 32px)' } } },
    MuiCssBaseline: { styleOverrides: { '*': { boxSizing: 'border-box' }, body: { margin: 0 }, ':focus-visible': { outline: `3px solid ${t.primary}`, outlineOffset: 3 }, '@media(prefers-reduced-motion:reduce)': { '*,*::before,*::after': { animationDuration: '0.01ms !important', transitionDuration: '0.01ms !important', scrollBehavior: 'auto !important' } } } },
  },
});
