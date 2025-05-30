// This is required for some BaseWeb components. If missing, BaseWeb will console.warn

import { ThemeProvider } from '#core/themes/provider';
import { WrapperComponentProps } from './types';

// This is required for some BaseWeb components. If missing, BaseWeb will console.warn
// something like "`LayersManager` was not found."
export function getBaseProviderWrapper() {
  return function BaseProviderWrapper({ children }: WrapperComponentProps) {
    return <ThemeProvider>{children}</ThemeProvider>;
  };
}
