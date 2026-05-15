import { SnackbarProvider } from 'baseui/snackbar';

import type { WrapperComponentProps } from './types';

/**
 * Test wrapper that provides `useSnackbar()` access to children.
 * Use it whenever a component (or a hook the component calls) enqueues
 * snackbars — e.g. via `useSuccessOperations`.
 */
export function getSnackbarProviderWrapper() {
  return function SnackbarProviderWrapper({ children }: WrapperComponentProps) {
    return <SnackbarProvider>{children}</SnackbarProvider>;
  };
}
