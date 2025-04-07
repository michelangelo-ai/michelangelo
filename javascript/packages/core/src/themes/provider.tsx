import { BaseProvider, createTheme } from 'baseui';
import { Client as Styletron } from 'styletron-engine-monolithic';
import { Provider as StyletronProvider } from 'styletron-react';

import { GRID_OVERRIDES } from './shared';

const engine = new Styletron();

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  return (
    <StyletronProvider value={engine}>
      <BaseProvider theme={createTheme({}, GRID_OVERRIDES)}>{children}</BaseProvider>
    </StyletronProvider>
  );
}
