import { BaseProvider, createTheme } from 'baseui';

import { GRID_OVERRIDES } from './shared';

import type { Theme } from 'baseui';

export function ThemeProvider({ children, theme }: { children: React.ReactNode; theme?: Theme }) {
  return <BaseProvider theme={theme ?? createTheme(GRID_OVERRIDES)}>{children}</BaseProvider>;
}
