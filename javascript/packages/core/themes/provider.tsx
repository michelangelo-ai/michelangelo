import { BaseProvider, createTheme } from 'baseui';
import { Client as Styletron } from 'styletron-engine-atomic';
import { Provider as StyletronProvider } from 'styletron-react';

import { GRID_OVERRIDES } from './shared';

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const engine = new Styletron();
  return (
    <StyletronProvider value={engine}>
      <BaseProvider theme={createTheme({}, GRID_OVERRIDES)}>{children}</BaseProvider>
    </StyletronProvider>
  );
}
