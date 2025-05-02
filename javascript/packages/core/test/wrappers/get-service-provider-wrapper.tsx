import { vi } from 'vitest';

import { ServiceProvider } from '#core/providers/service-provider/service-provider';
import { ServiceContextType } from '#core/providers/service-provider/types';
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
export function getServiceProviderWrapper(serviceProvider: Partial<ServiceContextType>) {
  const mockUseQuery = vi.fn();

  const base = {
    useQuery: mockUseQuery,
  };

  return function QueryProviderWrapper({ children }: WrapperComponentProps) {
    return (
      <ServiceProvider {...base} {...serviceProvider}>
        {children}
      </ServiceProvider>
    );
  };
}
