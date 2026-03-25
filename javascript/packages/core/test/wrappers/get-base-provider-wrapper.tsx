// This is required for some BaseWeb components. If missing, BaseWeb will console.warn

import { Client as Styletron } from 'styletron-engine-atomic';
import { Provider as StyletronProvider } from 'styletron-react';

import { ThemeProvider } from '#core/themes/provider';

import type { WrapperComponentProps } from './types';

// This is required for some BaseWeb components. If missing, BaseWeb will console.warn
// something like "`LayersManager` was not found."
export function getBaseProviderWrapper() {
  return function BaseProviderWrapper({ children }: WrapperComponentProps) {
    return (
      <StyletronProvider value={new Styletron()}>
        <ThemeProvider>{children}</ThemeProvider>
      </StyletronProvider>
    );
  };
}
