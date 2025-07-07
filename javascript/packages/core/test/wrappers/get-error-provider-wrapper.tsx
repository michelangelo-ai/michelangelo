import { ErrorProvider } from '#core/providers/error-provider/error-provider';
import { ErrorContextValue } from '#core/providers/error-provider/types';
import { WrapperComponentProps } from './types';

/**
 * Creates a React wrapper for testing components that use error handling features.
 * This wrapper is essential for testing components that use error hooks
 * like useErrorSystem, useApplicationError, etc.
 *
 * @param errorContext - The error context configuration to use for the error provider
 * @returns A wrapper component that provides error context to its children
 *
 * @example
 * ```tsx
 * // Simple usage with a custom normalizer
 * const customNormalizer = (error: unknown) => {
 *   if (isMyCustomError(error)) {
 *     return { message: 'Custom error', code: 7, source: 'custom' };
 *   }
 *   return null;
 * };
 * const wrapper = getErrorProviderWrapper({ normalizeError: customNormalizer });
 * render(<MyComponent />, { wrapper });
 * ```
 */
export function getErrorProviderWrapper(errorContext: Partial<ErrorContextValue> = {}) {
  const defaultNormalizeError = (error: unknown) => {
    return {
      message: 'Test error',
      code: 2,
      source: 'test',
      meta: { originalError: error },
    };
  };

  const base: ErrorContextValue = {
    normalizeError: defaultNormalizeError,
  };

  return function ErrorProviderWrapper({ children }: WrapperComponentProps) {
    return (
      <ErrorProvider {...base} {...errorContext}>
        {children}
      </ErrorProvider>
    );
  };
}
