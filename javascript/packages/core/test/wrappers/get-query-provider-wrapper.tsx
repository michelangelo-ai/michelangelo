import { vi } from 'vitest';

import { QueryProvider } from '#core/providers/query-provider/query-provider';
import { QueryContextType } from '#core/providers/query-provider/types';
import { WrapperComponentProps } from './types';

/**
 * Creates a React wrapper for testing components that use query features.
 * This wrapper is essential for testing components that use query hooks
 * like useQuery, useMutation, etc.
 *
 * @param queryProvider - The hooks to use for the query provider
 * @returns A wrapper component that provides query context to its children
 *
 * @example
 * ```tsx
 * // Simple usage with a specific route
 * const mockUseQuery = jest.fn();
 * const wrapper = getQueryProviderWrapper({ useQuery: mockUseQuery });
 * render(<MyComponent />, { wrapper });
 *
 * expect(mockUseQuery).toHaveBeenCalledWith('queryId', { queryKey: ['queryId'] });
 * ```
 */
export function getQueryProviderWrapper(queryProvider: QueryContextType) {
  const mockUseQuery = vi.fn();

  const base = {
    useQuery: mockUseQuery,
  };

  return function QueryProviderWrapper({ children }: WrapperComponentProps) {
    return (
      <QueryProvider {...base} {...queryProvider}>
        {children}
      </QueryProvider>
    );
  };
}
